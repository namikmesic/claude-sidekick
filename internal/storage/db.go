package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/namikmesic/claude-sidekick/internal/storage/migrations"
	"github.com/rs/zerolog/log"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sql, err := migrations.FS.ReadFile("001_initial.up.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	_, err = pool.Exec(ctx, string(sql))
	if err != nil {
		return fmt.Errorf("run migration: %w", err)
	}

	log.Info().Msg("database migrations applied")
	return nil
}
