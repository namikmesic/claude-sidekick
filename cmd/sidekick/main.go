package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/namikmesic/claude-sidekick/internal/config"
	"github.com/namikmesic/claude-sidekick/internal/processor"
	"github.com/namikmesic/claude-sidekick/internal/proxy"
	"github.com/namikmesic/claude-sidekick/internal/storage"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	ctx := context.Background()
	pool, err := storage.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if err := storage.RunMigrations(ctx, pool); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	writer := storage.NewBatchWriter(pool, cfg.WriterBufferSize, cfg.WriterBatchSize, cfg.WriterFlushMs)
	proc := processor.New(writer)
	handler := proxy.NewHandler(cfg, writer, proc)

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info().
			Int("port", cfg.Port).
			Str("upstream", cfg.AnthropicBaseURL).
			Msg("sidekick proxy started")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-done
	log.Info().Msg("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server.Shutdown(shutdownCtx)
	writer.Shutdown()
	log.Info().Msg("shutdown complete")
}
