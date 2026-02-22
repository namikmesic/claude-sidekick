package config

import (
	"github.com/caarlos0/env/v11"
)

type Config struct {
	Port             int    `env:"PORT" envDefault:"8090"`
	LogLevel         string `env:"LOG_LEVEL" envDefault:"info"`
	DatabaseURL      string `env:"DATABASE_URL" envDefault:"postgres://sidekick:sidekick@localhost:5433/sidekick?sslmode=disable"`
	AnthropicBaseURL string `env:"ANTHROPIC_UPSTREAM_URL" envDefault:"https://api.anthropic.com"`
	AnthropicAPIKey  string `env:"ANTHROPIC_API_KEY"`
	WriterBufferSize int    `env:"WRITER_BUFFER_SIZE" envDefault:"10000"`
	WriterBatchSize  int    `env:"WRITER_BATCH_SIZE" envDefault:"100"`
	WriterFlushMs    int    `env:"WRITER_FLUSH_MS" envDefault:"100"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
