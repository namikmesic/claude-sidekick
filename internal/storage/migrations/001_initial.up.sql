-- Accounts: OAuth accounts for load balancing
CREATE TABLE IF NOT EXISTS accounts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT NOT NULL UNIQUE,
    provider              TEXT NOT NULL DEFAULT 'anthropic',
    api_key               TEXT,
    refresh_token         TEXT,
    access_token          TEXT,
    expires_at            TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used             TIMESTAMPTZ,
    request_count         BIGINT DEFAULT 0,
    account_tier          SMALLINT DEFAULT 1,
    paused                BOOLEAN DEFAULT FALSE,
    rate_limited_until    TIMESTAMPTZ,
    session_start         TIMESTAMPTZ,
    session_request_count INTEGER DEFAULT 0
);

-- Requests: one row per proxied API call (hypertable)
CREATE TABLE IF NOT EXISTS requests (
    id                    UUID NOT NULL,
    ts                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    method                TEXT NOT NULL,
    path                  TEXT NOT NULL,
    account_id            UUID REFERENCES accounts(id) ON DELETE SET NULL,
    status_code           SMALLINT,
    success               BOOLEAN,
    error_message         TEXT,
    response_time_ms      INTEGER,
    failover_attempts     SMALLINT DEFAULT 0,
    model                 TEXT,
    input_tokens          INTEGER DEFAULT 0,
    output_tokens         INTEGER DEFAULT 0,
    cache_read_tokens     INTEGER DEFAULT 0,
    cache_creation_tokens INTEGER DEFAULT 0,
    total_tokens          INTEGER DEFAULT 0,
    cost_usd              NUMERIC(12,8) DEFAULT 0,
    tokens_per_second     REAL,
    is_stream             BOOLEAN DEFAULT FALSE,
    agent_used            TEXT,
    PRIMARY KEY (id, ts)
);

SELECT create_hypertable('requests', by_range('ts'), if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_requests_account_ts ON requests (account_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_requests_model_ts ON requests (model, ts DESC) WHERE model IS NOT NULL;

-- SSE Events: every SSE chunk as a separate row (hypertable)
CREATE TABLE IF NOT EXISTS sse_events (
    id          BIGINT GENERATED ALWAYS AS IDENTITY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id  UUID NOT NULL,
    event_index INTEGER NOT NULL,
    event_type  TEXT NOT NULL,
    data_json   JSONB,
    raw_bytes   INTEGER,
    PRIMARY KEY (id, ts)
);

SELECT create_hypertable('sse_events', by_range('ts'), if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_sse_events_request ON sse_events (request_id, ts, event_index);
CREATE INDEX IF NOT EXISTS idx_sse_events_type ON sse_events (event_type, ts DESC);

-- Request Payloads: full request/response bodies
CREATE TABLE IF NOT EXISTS request_payloads (
    request_id       UUID NOT NULL,
    ts               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_headers  JSONB,
    request_body     BYTEA,
    response_headers JSONB,
    response_body    BYTEA,
    PRIMARY KEY (request_id, ts)
);

SELECT create_hypertable('request_payloads', by_range('ts'), if_not_exists => TRUE);
