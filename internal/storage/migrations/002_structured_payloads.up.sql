-- Add analytics fields to the hot requests table
ALTER TABLE requests
    ADD COLUMN IF NOT EXISTS stop_reason            TEXT,
    ADD COLUMN IF NOT EXISTS message_id             TEXT,
    ADD COLUMN IF NOT EXISTS tool_count             SMALLINT,
    ADD COLUMN IF NOT EXISTS thinking_budget_tokens INTEGER;

-- Convert BYTEA → JSONB (idempotent: only runs if columns are still bytea)
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'request_payloads'
          AND column_name = 'request_body'
          AND data_type = 'bytea'
    ) THEN
        ALTER TABLE request_payloads
            ALTER COLUMN request_body  TYPE JSONB USING convert_from(request_body,  'UTF8')::jsonb,
            ALTER COLUMN response_body TYPE JSONB USING convert_from(response_body, 'UTF8')::jsonb;
    END IF;
END $$;

-- Add extracted payload columns
ALTER TABLE request_payloads
    ADD COLUMN IF NOT EXISTS system_prompt  TEXT,
    ADD COLUMN IF NOT EXISTS max_tokens     INTEGER,
    ADD COLUMN IF NOT EXISTS temperature    REAL,
    ADD COLUMN IF NOT EXISTS top_p          REAL,
    ADD COLUMN IF NOT EXISTS message_count  SMALLINT,
    ADD COLUMN IF NOT EXISTS stop_sequence  TEXT;
