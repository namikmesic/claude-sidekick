package storage

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// WriteJob represents a unit of work to execute against the database.
type WriteJob interface {
	Execute(ctx context.Context, pool *pgxpool.Pool) error
}

// WriteJobFunc adapts a function into a WriteJob.
type WriteJobFunc func(ctx context.Context, pool *pgxpool.Pool) error

func (f WriteJobFunc) Execute(ctx context.Context, pool *pgxpool.Pool) error {
	return f(ctx, pool)
}

// BatchWriter collects write jobs and flushes them in batches.
type BatchWriter struct {
	pool      *pgxpool.Pool
	jobs      chan WriteJob
	batchSize int
	flushMs   int
	wg        sync.WaitGroup
}

func NewBatchWriter(pool *pgxpool.Pool, bufferSize, batchSize, flushMs int) *BatchWriter {
	w := &BatchWriter{
		pool:      pool,
		jobs:      make(chan WriteJob, bufferSize),
		batchSize: batchSize,
		flushMs:   flushMs,
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

func (w *BatchWriter) Enqueue(job WriteJob) {
	select {
	case w.jobs <- job:
	default:
		log.Warn().Msg("write queue full, dropping job")
	}
}

func (w *BatchWriter) loop() {
	defer w.wg.Done()

	ticker := time.NewTicker(time.Duration(w.flushMs) * time.Millisecond)
	defer ticker.Stop()

	batch := make([]WriteJob, 0, w.batchSize)

	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				w.flush(batch)
				return
			}
			batch = append(batch, job)
			if len(batch) >= w.batchSize {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (w *BatchWriter) flush(batch []WriteJob) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, job := range batch {
		if err := job.Execute(ctx, w.pool); err != nil {
			log.Error().Err(err).Msg("write job failed")
		}
	}
}

func (w *BatchWriter) Shutdown() {
	close(w.jobs)
	w.wg.Wait()
}
