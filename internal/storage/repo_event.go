package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/namikmesic/claude-sidekick/internal/stream"
)

// InsertSSEEventsJob creates a batch insert job for SSE events using COPY protocol.
func InsertSSEEventsJob(requestID uuid.UUID, ts time.Time, events []stream.SSEEvent) WriteJob {
	return WriteJobFunc(func(ctx context.Context, pool *pgxpool.Pool) error {
		rows := make([][]interface{}, len(events))
		for i, ev := range events {
			rows[i] = []interface{}{
				ts,
				requestID,
				ev.Index,
				ev.EventType,
				ev.RawData,
				ev.RawBytes,
			}
		}

		_, err := pool.CopyFrom(ctx,
			pgx.Identifier{"sse_events"},
			[]string{"ts", "request_id", "event_index", "event_type", "data_json", "raw_bytes"},
			pgx.CopyFromRows(rows),
		)
		return err
	})
}
