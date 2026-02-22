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

// Anthropic SSE message_delta payload (final output token count).
type MessageDelta struct {
	Type  string `json:"type"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
