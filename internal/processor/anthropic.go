package processor

import (
	"encoding/json"
	"strings"
)

type AnthropicRequest struct {
	Model         string          `json:"model"`
	Messages      []ReqMessage    `json:"messages"`
	System        json.RawMessage `json:"system"`        // string OR []SystemBlock
	MaxTokens     int             `json:"max_tokens"`
	Temperature   *float64        `json:"temperature"`
	TopP          *float64        `json:"top_p"`
	TopK          *int            `json:"top_k"`
	Stream        bool            `json:"stream"`
	Tools         []Tool          `json:"tools"`
	ToolChoice    json.RawMessage `json:"tool_choice"` // "auto" | "any" | {"type":"tool","name":"..."}
	StopSequences []string        `json:"stop_sequences"`
	Thinking      *ThinkingConfig `json:"thinking"`
	Metadata      json.RawMessage `json:"metadata"`
}

type ReqMessage struct {
	Role    string          `json:"role"`    // "user" | "assistant"
	Content json.RawMessage `json:"content"` // string OR []ContentBlock
}

type SystemBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control"`
}

type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

type AnthropicResponse struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Role         string      `json:"role"`
	Content      []RespBlock `json:"content"`
	Model        string      `json:"model"`
	StopReason   string      `json:"stop_reason"` // "end_turn" | "max_tokens" | "tool_use" | "stop_sequence"
	StopSequence *string     `json:"stop_sequence"`
	Usage        UsageInfo   `json:"usage"`
}

type RespBlock struct {
	Type     string          `json:"type"` // "text" | "tool_use" | "thinking"
	Text     string          `json:"text"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Thinking string          `json:"thinking"`
}

type UsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type ParsedRequest struct {
	SystemPrompt         string
	MaxTokens            int
	Temperature          *float64
	TopP                 *float64
	MessageCount         int
	ToolCount            int
	ThinkingBudgetTokens int
}

// Returns zero-value ParsedRequest on parse failure.
func ParseRequest(body []byte) ParsedRequest {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ParsedRequest{}
	}

	var budget int
	if req.Thinking != nil {
		budget = req.Thinking.BudgetTokens
	}

	return ParsedRequest{
		SystemPrompt:         extractSystemPrompt(req.System),
		MaxTokens:            req.MaxTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		MessageCount:         len(req.Messages),
		ToolCount:            len(req.Tools),
		ThinkingBudgetTokens: budget,
	}
}

// extractSystemPrompt handles both string and []SystemBlock forms.
func extractSystemPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var blocks []SystemBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	texts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}
