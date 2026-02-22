package stream

import (
	"bytes"
	"strings"
)

// Parser maintains state across chunks to handle partial SSE lines.
type Parser struct {
	buffer     []byte
	eventIndex int
	eventType  string // current event: field value
}

func NewParser() *Parser {
	return &Parser{}
}

// ParseChunk processes raw bytes from the stream and yields complete SSE events.
// Handles partial lines that span multiple chunks.
func (p *Parser) ParseChunk(chunk []byte) []SSEEvent {
	p.buffer = append(p.buffer, chunk...)
	var events []SSEEvent

	for {
		idx := bytes.IndexByte(p.buffer, '\n')
		if idx == -1 {
			break
		}

		line := string(p.buffer[:idx])
		p.buffer = p.buffer[idx+1:]
		line = strings.TrimRight(line, "\r")

		if line == "" {
			// Empty line = event separator, reset event type
			p.eventType = ""
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			p.eventType = strings.TrimSpace(line[7:])
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataStr := line[6:]
			p.eventIndex++

			eventType := p.eventType
			if eventType == "" {
			eventType = inferEventType(dataStr)
			}

			events = append(events, SSEEvent{
				Index:     p.eventIndex,
				EventType: eventType,
				RawData:   dataStr,
				RawBytes:  len(line) + 1, // +1 for newline
			})
		}
	}

	return events
}

// inferEventType extracts the "type" field from JSON data without full parsing.
func inferEventType(data string) string {
	// Fast path: look for "type":"..." pattern
	idx := strings.Index(data, `"type"`)
	if idx == -1 {
		return "unknown"
	}

	rest := data[idx+6:]
	rest = strings.TrimLeft(rest, " \t:")
	rest = strings.TrimLeft(rest, " \t")

	if len(rest) > 0 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1]
		}
	}
	return "unknown"
}
