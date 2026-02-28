package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/namikmesic/claude-sidekick/internal/storage"
	"github.com/namikmesic/claude-sidekick/internal/stream"
	nats "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// Processor handles background analytics for proxied requests.
type Processor struct {
	writer *storage.BatchWriter
}

func New(writer *storage.BatchWriter) *Processor {
	return &Processor{writer: writer}
}

type streamBlock struct {
	blockType string // "text" | "tool_use" | "thinking"
	id        string
	name      string
	text      string
	inputJSON string // accumulated partial_json fragments for tool_use
}

type reqAccumulator struct {
	ts  time.Time
	buf bytes.Buffer
}

func (p *Processor) StartConsumer(ctx context.Context, js nats.JetStreamContext) {
	accumulators := make(map[uuid.UUID]*reqAccumulator)

	sub, err := js.Subscribe("sidekick.req.>", func(msg *nats.Msg) {
		subject := msg.Subject
		msg.Ack()

		if strings.HasSuffix(subject, ".done") {
			requestID := extractRequestID(subject, true)
			if requestID == uuid.Nil {
				return
			}
			acc, ok := accumulators[requestID]
			if !ok {
				return
			}
			delete(accumulators, requestID)

			var meta struct {
				TS int64 `json:"ts"`
			}
			if err := json.Unmarshal(msg.Data, &meta); err == nil && meta.TS != 0 {
				acc.ts = time.Unix(0, meta.TS)
			}

			p.processReader(requestID, acc.ts, bytes.NewReader(acc.buf.Bytes()))
		} else {
			requestID := extractRequestID(subject, false)
			if requestID == uuid.Nil {
				return
			}
			if _, ok := accumulators[requestID]; !ok {
				accumulators[requestID] = &reqAccumulator{ts: time.Now()}
			}
			accumulators[requestID].buf.Write(msg.Data)
		}
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to JetStream")
	}

	<-ctx.Done()
	sub.Drain()
}

func extractRequestID(subject string, done bool) uuid.UUID {
	const prefix = "sidekick.req."
	s := strings.TrimPrefix(subject, prefix)
	if done {
		s = strings.TrimSuffix(s, ".done")
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func (p *Processor) processReader(requestID uuid.UUID, ts time.Time, reader io.Reader) {
	parser := stream.NewParser()
	buf := make([]byte, 32*1024)

	var allEvents []stream.SSEEvent
	var model, messageID, stopReason string
	var stopSequence *string
	var inputTokens, outputTokens, cacheRead, cacheCreation int
	blocks := make(map[int]*streamBlock)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			events := parser.ParseChunk(buf[:n])
			allEvents = append(allEvents, events...)

			for _, ev := range events {
				p.extractStreamFields(ev, &model, &messageID, &stopReason, &stopSequence, &inputTokens, &outputTokens, &cacheRead, &cacheCreation)
				p.accumulateBlock(ev, blocks)
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
			0, 0,
			stopReason, messageID,
		))
	}

	if respBody := reconstructResponse(messageID, model, stopReason, stopSequence, blocks, inputTokens, outputTokens, cacheRead, cacheCreation); respBody != nil {
		p.writer.Enqueue(storage.UpdatePayloadResponseJob(requestID, ts, respBody, stopSequence))
	}

	log.Debug().
		Str("request_id", requestID.String()).
		Int("sse_events", len(allEvents)).
		Str("model", model).
		Str("stop_reason", stopReason).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Msg("stream processing complete")
}

// ProcessNonStream handles a non-streaming response body.
func (p *Processor) ProcessNonStream(requestID uuid.UUID, ts time.Time, body []byte) {
	var parsed AnthropicResponse
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
		parsed.StopReason, parsed.ID,
	))
}

func (p *Processor) extractStreamFields(ev stream.SSEEvent, model, messageID, stopReason *string, stopSequence **string, input, output, cacheRead, cacheCreation *int) {
	switch ev.EventType {
	case "message_start":
		var msg stream.MessageStart
		if err := json.Unmarshal([]byte(ev.RawData), &msg); err == nil {
			*model = msg.Message.Model
			*messageID = msg.Message.ID
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
			if msg.Delta.StopReason != "" {
				*stopReason = msg.Delta.StopReason
			}
			if msg.Delta.StopSequence != nil {
				*stopSequence = msg.Delta.StopSequence
			}
		}
	}
}

func (p *Processor) accumulateBlock(ev stream.SSEEvent, blocks map[int]*streamBlock) {
	switch ev.EventType {
	case "content_block_start":
		var msg stream.ContentBlockStart
		if err := json.Unmarshal([]byte(ev.RawData), &msg); err == nil {
			blocks[msg.Index] = &streamBlock{
				blockType: msg.ContentBlock.Type,
				id:        msg.ContentBlock.ID,
				name:      msg.ContentBlock.Name,
			}
		}
	case "content_block_delta":
		var msg stream.ContentBlockDelta
		if err := json.Unmarshal([]byte(ev.RawData), &msg); err == nil {
			b := blocks[msg.Index]
			if b == nil {
				return
			}
			switch msg.Delta.Type {
			case "text_delta":
				b.text += msg.Delta.Text
			case "thinking_delta":
				b.text += msg.Delta.Thinking
			case "input_json_delta":
				b.inputJSON += msg.Delta.PartialJSON
			}
		}
	}
}

func reconstructResponse(messageID, model, stopReason string, stopSequence *string, blocks map[int]*streamBlock, inputTokens, outputTokens, cacheRead, cacheCreation int) []byte {
	if messageID == "" && model == "" {
		return nil
	}

	indices := make([]int, 0, len(blocks))
	for i := range blocks {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	content := make([]RespBlock, 0, len(indices))
	for _, i := range indices {
		b := blocks[i]
		switch b.blockType {
		case "text":
			content = append(content, RespBlock{Type: "text", Text: b.text})
		case "tool_use":
			content = append(content, RespBlock{Type: "tool_use", ID: b.id, Name: b.name, Input: validJSON(b.inputJSON)})
		case "thinking":
			content = append(content, RespBlock{Type: "thinking", Thinking: b.text})
		}
	}

	resp := AnthropicResponse{
		ID:           messageID,
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        model,
		StopReason:   stopReason,
		StopSequence: stopSequence,
		Usage: UsageInfo{
			InputTokens:              inputTokens,
			OutputTokens:             outputTokens,
			CacheCreationInputTokens: cacheCreation,
			CacheReadInputTokens:     cacheRead,
		},
	}

	out, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	return out
}

func validJSON(s string) json.RawMessage {
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	return json.RawMessage("null")
}
