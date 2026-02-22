package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RequestRecord struct {
	ID                   uuid.UUID
	Timestamp            time.Time
	Method               string
	Path                 string
	AccountID            *uuid.UUID
	StatusCode           int
	Success              bool
	ErrorMessage         string
	ResponseTimeMs       int
	FailoverAttempts     int
	Model                string
	InputTokens          int
	OutputTokens         int
	CacheReadTokens      int
	CacheCreationTokens  int
	TotalTokens          int
	CostUSD              float64
	TokensPerSecond      float32
	IsStream             bool
	AgentUsed            string
	ToolCount            int
	ThinkingBudgetTokens int
}

func InsertRequestJob(r *RequestRecord) WriteJob {
	return WriteJobFunc(func(ctx context.Context, pool *pgxpool.Pool) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO requests (
				id, ts, method, path, account_id, status_code, success, error_message,
				response_time_ms, failover_attempts, model, input_tokens, output_tokens,
				cache_read_tokens, cache_creation_tokens, total_tokens, cost_usd,
				tokens_per_second, is_stream, agent_used, tool_count, thinking_budget_tokens
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
			r.ID, r.Timestamp, r.Method, r.Path, r.AccountID,
			r.StatusCode, r.Success, nilIfEmpty(r.ErrorMessage),
			r.ResponseTimeMs, r.FailoverAttempts, nilIfEmpty(r.Model),
			r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreationTokens,
			r.TotalTokens, r.CostUSD, r.TokensPerSecond, r.IsStream, nilIfEmpty(r.AgentUsed),
			nilIfZero(r.ToolCount), nilIfZero(r.ThinkingBudgetTokens),
		)
		return err
	})
}

func UpdateRequestUsageJob(requestID uuid.UUID, ts time.Time, model string, inputTokens, outputTokens, cacheRead, cacheCreation, totalTokens int, costUSD float64, tokensPerSec float32, stopReason, messageID string) WriteJob {
	return WriteJobFunc(func(ctx context.Context, pool *pgxpool.Pool) error {
		_, err := pool.Exec(ctx, `
			UPDATE requests SET
				model = COALESCE($1, model),
				input_tokens = $2,
				output_tokens = $3,
				cache_read_tokens = $4,
				cache_creation_tokens = $5,
				total_tokens = $6,
				cost_usd = $7,
				tokens_per_second = $8,
				stop_reason = COALESCE($9, stop_reason),
				message_id = COALESCE($10, message_id),
				success = TRUE
			WHERE id = $11 AND ts = $12`,
			nilIfEmpty(model), inputTokens, outputTokens, cacheRead, cacheCreation,
			totalTokens, costUSD, tokensPerSec,
			nilIfEmpty(stopReason), nilIfEmpty(messageID),
			requestID, ts,
		)
		return err
	})
}

type PayloadExtras struct {
	SystemPrompt string
	MaxTokens    int
	Temperature  *float64
	TopP         *float64
	MessageCount int
	StopSequence *string
}

func InsertPayloadJob(requestID uuid.UUID, ts time.Time, reqHeaders, respHeaders map[string][]string, reqBody, respBody []byte, extras PayloadExtras) WriteJob {
	return WriteJobFunc(func(ctx context.Context, pool *pgxpool.Pool) error {
		reqH, _ := json.Marshal(reqHeaders)
		respH, _ := json.Marshal(respHeaders)
		_, err := pool.Exec(ctx, `
			INSERT INTO request_payloads (
				request_id, ts, request_headers, request_body, response_headers, response_body,
				system_prompt, max_tokens, temperature, top_p, message_count, stop_sequence
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			requestID, ts, reqH, rawJSON(reqBody), respH, rawJSON(respBody),
			nilIfEmpty(extras.SystemPrompt), nilIfZero(extras.MaxTokens),
			extras.Temperature, extras.TopP,
			nilIfZero(extras.MessageCount), extras.StopSequence,
		)
		return err
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfZero(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func rawJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

func UpdatePayloadResponseJob(requestID uuid.UUID, ts time.Time, respBody []byte, stopSequence *string) WriteJob {
	return WriteJobFunc(func(ctx context.Context, pool *pgxpool.Pool) error {
		_, err := pool.Exec(ctx, `
			UPDATE request_payloads
			SET response_body = $1,
			    stop_sequence = COALESCE($2, stop_sequence)
			WHERE request_id = $3 AND ts = $4`,
			rawJSON(respBody), stopSequence, requestID, ts,
		)
		return err
	})
}
