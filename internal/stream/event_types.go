package stream

// SSEEvent represents a single parsed SSE event from the stream.
type SSEEvent struct {
	Index     int    // ordinal within this request's stream
	EventType string // message_start, content_block_delta, message_delta, etc.
	RawData   string // raw JSON string from the data: line
	RawBytes  int    // byte length of this SSE frame
}

// Anthropic SSE message_start payload (for usage extraction).
type MessageStart struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type ContentBlockStart struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"` // "text" | "tool_use" | "thinking"
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
}

type ContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"` // "text_delta" | "input_json_delta" | "thinking_delta"
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		Thinking    string `json:"thinking"`
	} `json:"delta"`
}

// Anthropic SSE message_delta payload (final output token count and stop reason).
type MessageDelta struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason   string  `json:"stop_reason"`
		StopSequence *string `json:"stop_sequence"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
