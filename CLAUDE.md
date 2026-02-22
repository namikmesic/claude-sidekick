# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Sidekick is a **Go reverse proxy** for the Anthropic API that captures analytics, token usage, and audit logs. It sits between clients and the upstream Anthropic API, duplicating responses for async analytics while transparently proxying traffic.

## Commands

```bash
make build       # Compile to ./bin/sidekick
make run         # Build and run locally (requires DB)
make docker-up   # Start TimescaleDB + sidekick via docker-compose
make docker-down # Stop and remove containers
make docker-build # Rebuild images and restart
make clean       # Remove binaries and docker volumes
```

Copy `.env.example` to `.env` before running locally.

## Architecture

```
Client → Proxy Handler → Anthropic API
              │
              ├── Stream: TeeReader duplicates response body
              │     ├── Client stream (forwarded immediately)
              │     └── Analytics stream → SSE Parser → Processor
              │
              └── Non-stream: Full body buffered → Processor
                                                      │
                                          BatchWriter (async, 100ms / 100-job flush)
                                                      │
                                              TimescaleDB
```

### Key packages

| Package | Path | Role |
|---------|------|------|
| `main` | `cmd/sidekick/` | Startup, server init, graceful shutdown |
| `config` | `internal/config/` | Env var config via caarlos0/env |
| `proxy` | `internal/proxy/` | HTTP reverse proxy handler, header filtering, URL construction |
| `processor` | `internal/processor/` | Extracts model/token usage from SSE and JSON responses |
| `stream` | `internal/stream/` | SSE parser, stream tee, event types |
| `storage` | `internal/storage/` | DB pool, migrations, batch writer, repo functions |

### Data flow for streaming requests

1. Proxy receives request → generates UUID, timestamps
2. Request body buffered, forwarded upstream with filtered headers
3. Response `Content-Type: text/event-stream` detected
4. `TeeBody` splits stream: client gets original, analytics gets copy
5. Processor goroutine reads SSE events via incremental parser
6. `message_start` event yields model + input tokens; `message_delta` yields output tokens
7. Storage jobs enqueued to `BatchWriter` channel (non-blocking, drops if full)
8. Batch writer flushes every 100ms or 100 jobs, uses pgx COPY for SSE events

### Database

TimescaleDB with four hypertables (partitioned by timestamp):
- `requests` — request metadata, token usage, response times
- `sse_events` — every SSE frame from streaming responses
- `request_payloads` — full request/response headers and bodies
- `accounts` — multi-account config for load balancing

Migrations are embedded via `go:embed` in `internal/storage/migrations/`.

### Notable patterns

- **Non-blocking writes**: `BatchWriter.Enqueue()` drops jobs if channel full rather than blocking the proxy
- **Header redaction**: `Authorization` and `x-api-key` replaced with `[REDACTED]` before storage
- **Accept-Encoding stripping**: Removed from upstream requests to prevent compressed SSE responses
- **Incremental SSE parsing**: Parser maintains state across chunks to handle partial lines
- **COPY protocol**: pgx COPY used for bulk SSE event insertion
