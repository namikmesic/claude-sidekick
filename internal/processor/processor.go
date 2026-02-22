package processor

import (
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/namikmesic/claude-sidekick/internal/storage"
	"github.com/namikmesic/claude-sidekick/internal/stream"
	"github.com/rs/zerolog/log"
)

// Processor handles background analytics for proxied requests.
type Processor struct {
	writer *storage.BatchWriter
}

func New(writer *storage.BatchWriter) *Processor {
	return &Processor{writer: writer}
}

// ProcessStream reads SSE events from the analytics pipe, stores each event,
// and extracts token usage from message_start/message_delta.
func (p *Processor) ProcessStream(requestID uuid.UUID, ts time.Time, reader io.Reader) {
	parser := stream.NewParser()
	buf := make([]byte, 32*1024)

	var allEvents []stream.SSEEvent
	var model string
	var inputTokens, outputTokens, cacheRead, cacheCreation int

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			events := parser.ParseChunk(buf[:n])
			allEvents = append(allEvents, events...)

			for _, ev := range events {
				p.extractUsage(ev, &model, &inputTokens, &outputTokens, &cacheRead, &cacheCreation)
			}
		}
		if err != nil {
			break
		}
	}

	if len(allEvents) > 0 {
		p.writer.Enqueue(storage.InsertSSEEventsJob(requestID, ts, allEvents))
	}

	totalTokens := inputTokens + outputTokens + cacheRead + cacheCreation
	if model != "" || totalTokens > 0 {
		p.writer.Enqueue(storage.UpdateRequestUsageJob(
			requestID, ts, model,
			inputTokens, outputTokens, cacheRead, cacheCreation, totalTokens,
			0, 0, // cost and tokens/sec calculated later
		))
	}

	log.Debug().
		Str("request_id", requestID.String()).
		Int("sse_events", len(allEvents)).
		Str("model", model).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Msg("stream processing complete")
}

// ProcessNonStream handles a non-streaming response body.
func (p *Processor) ProcessNonStream(requestID uuid.UUID, ts time.Time, body []byte) {
	var parsed struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}

	if parsed.Model == "" {
		return
	}

	totalTokens := parsed.Usage.InputTokens + parsed.Usage.OutputTokens +
		parsed.Usage.CacheReadInputTokens + parsed.Usage.CacheCreationInputTokens

	p.writer.Enqueue(storage.UpdateRequestUsageJob(
		requestID, ts, parsed.Model,
		parsed.Usage.InputTokens, parsed.Usage.OutputTokens,
		parsed.Usage.CacheReadInputTokens, parsed.Usage.CacheCreationInputTokens,
		totalTokens, 0, 0,
	))
}

func (p *Processor) extractUsage(ev stream.SSEEvent, model *string, input, output, cacheRead, cacheCreation *int) {
	switch ev.EventType {
	case "message_start":
		var msg stream.MessageStart
		if err := json.Unmarshal([]byte(ev.RawData), &msg); err == nil {
			*model = msg.Message.Model
			*input = msg.Message.Usage.InputTokens
			*output = msg.Message.Usage.OutputTokens
			*cacheRead = msg.Message.Usage.CacheReadInputTokens
			*cacheCreation = msg.Message.Usage.CacheCreationInputTokens
		}
	case "message_delta":
		var msg stream.MessageDelta
		if err := json.Unmarshal([]byte(ev.RawData), &msg); err == nil {
			if msg.Usage.OutputTokens > 0 {
				*output = msg.Usage.OutputTokens
			}
		}
	}
}
