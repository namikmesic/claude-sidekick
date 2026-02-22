package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/namikmesic/claude-sidekick/internal/config"
	"github.com/namikmesic/claude-sidekick/internal/jetstream"
	"github.com/namikmesic/claude-sidekick/internal/processor"
	"github.com/namikmesic/claude-sidekick/internal/storage"
	nats "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// Handler is the core reverse proxy.
type Handler struct {
	cfg       *config.Config
	client    *http.Client
	writer    *storage.BatchWriter
	processor *processor.Processor
	js        nats.JetStreamContext
}

func NewHandler(cfg *config.Config, writer *storage.BatchWriter, proc *processor.Processor, js nats.JetStreamContext) *Handler {
	return &Handler{
		cfg: cfg,
		client: &http.Client{
			// No timeout — streaming responses can be long-lived
			Timeout: 0,
			// Don't follow redirects
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		writer:    writer,
		processor: proc,
		js:        js,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New()
	ts := time.Now()
	start := ts

	var reqBody []byte
	if r.Body != nil {
		var err error
		reqBody, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			log.Error().Err(err).Msg("failed to read request body")
			http.Error(w, "failed to read request body", http.StatusBadGateway)
			return
		}
	}

	reqParsed := processor.ParseRequest(reqBody)

	targetURL := buildTargetURL(h.cfg.AnthropicBaseURL, r.URL.Path, r.URL.RawQuery)
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		log.Error().Err(err).Msg("failed to create upstream request")
		http.Error(w, "failed to create upstream request", http.StatusBadGateway)
		return
	}

	upstreamReq.Header = prepareUpstreamHeaders(r.Header, h.cfg.AnthropicAPIKey)

	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		log.Error().Err(err).Str("url", targetURL).Msg("upstream request failed")
		http.Error(w, "upstream request failed", http.StatusBadGateway)

		h.writer.Enqueue(storage.InsertRequestJob(&storage.RequestRecord{
			ID:             requestID,
			Timestamp:      ts,
			Method:         r.Method,
			Path:           r.URL.Path,
			StatusCode:     502,
			Success:        false,
			ErrorMessage:   err.Error(),
			ResponseTimeMs: int(time.Since(start).Milliseconds()),
		}))
		return
	}
	defer resp.Body.Close()

	isStreaming := isStreamingResponse(resp)

	h.writer.Enqueue(storage.InsertRequestJob(&storage.RequestRecord{
		ID:                   requestID,
		Timestamp:            ts,
		Method:               r.Method,
		Path:                 r.URL.Path,
		StatusCode:           resp.StatusCode,
		Success:              resp.StatusCode >= 200 && resp.StatusCode < 400,
		ResponseTimeMs:       int(time.Since(start).Milliseconds()),
		IsStream:             isStreaming,
		ToolCount:            reqParsed.ToolCount,
		ThinkingBudgetTokens: reqParsed.ThinkingBudgetTokens,
	}))

	clientHeaders := prepareClientHeaders(resp.Header)
	for k, vv := range clientHeaders {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	if isStreaming {
		h.handleStreaming(w, resp, requestID, ts, r, reqBody, reqParsed)
	} else {
		h.handleNonStreaming(w, resp, requestID, ts, r, reqBody, reqParsed)
	}

	log.Info().
		Str("request_id", requestID.String()).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Int("status", resp.StatusCode).
		Bool("stream", isStreaming).
		Dur("duration", time.Since(start)).
		Msg("proxied request")
}

func (h *Handler) handleStreaming(w http.ResponseWriter, resp *http.Response, requestID uuid.UUID, ts time.Time, origReq *http.Request, reqBody []byte, reqParsed processor.ParsedRequest) {
	h.storePayload(requestID, ts, origReq, reqBody, resp, nil, reqParsed, nil)

	w.WriteHeader(resp.StatusCode)
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	subject := jetstream.ChunkSubject(requestID.String())

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			h.js.Publish(subject, buf[:n])
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}

	done, _ := json.Marshal(map[string]int64{"ts": ts.UnixNano()})
	h.js.Publish(jetstream.DoneSubject(requestID.String()), done)
}

func (h *Handler) handleNonStreaming(w http.ResponseWriter, resp *http.Response, requestID uuid.UUID, ts time.Time, origReq *http.Request, reqBody []byte, reqParsed processor.ParsedRequest) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read response body")
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	var stopSequence *string
	var respParsed processor.AnthropicResponse
	if jsonErr := json.Unmarshal(respBody, &respParsed); jsonErr == nil {
		stopSequence = respParsed.StopSequence
	}

	go h.processor.ProcessNonStream(requestID, ts, respBody)
	h.storePayload(requestID, ts, origReq, reqBody, resp, respBody, reqParsed, stopSequence)
}

func (h *Handler) storePayload(requestID uuid.UUID, ts time.Time, req *http.Request, reqBody []byte, resp *http.Response, respBody []byte, reqParsed processor.ParsedRequest, stopSequence *string) {
	reqHeaders := headerMap(req.Header)
	respHeaders := headerMap(resp.Header)
	extras := storage.PayloadExtras{
		SystemPrompt: reqParsed.SystemPrompt,
		MaxTokens:    reqParsed.MaxTokens,
		Temperature:  reqParsed.Temperature,
		TopP:         reqParsed.TopP,
		MessageCount: reqParsed.MessageCount,
		StopSequence: stopSequence,
	}
	h.writer.Enqueue(storage.InsertPayloadJob(requestID, ts, reqHeaders, respHeaders, reqBody, respBody, extras))
}

func isStreamingResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/event-stream")
}

func headerMap(h http.Header) map[string][]string {
	// Filter out sensitive headers before storing
	m := make(map[string][]string, len(h))
	for k, v := range h {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "x-api-key" {
			m[k] = []string{"[REDACTED]"}
		} else {
			m[k] = v
		}
	}
	return m
}
