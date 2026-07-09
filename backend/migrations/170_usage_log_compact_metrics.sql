-- Compact request diagnostics for Codex /responses/compact routing.
-- These columns are written/read via raw SQL in usage_log_repo_{insert,query}.go
-- (not through ent schema generation). Keep this migration in sync with those
-- insert/select column lists when changing field names or types.
ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS compact_payload_bytes BIGINT,
  ADD COLUMN IF NOT EXISTS compact_retry_count INTEGER,
  ADD COLUMN IF NOT EXISTS compact_client_canceled BOOLEAN;

-- request_type enum expansion:
--   0=unknown, 1=sync, 2=stream, 3=ws_v2, 4=cyber, 5=compact
-- Migration 061 only allowed (0,1,2,3). Without this refresh, inserting
-- compact (5) or cyber (4) usage rows fails the CHECK constraint.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_logs_request_type_check'
    ) THEN
        ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_request_type_check;
    END IF;

    ALTER TABLE usage_logs
        ADD CONSTRAINT usage_logs_request_type_check
        CHECK (request_type IN (0, 1, 2, 3, 4, 5));
END
$$;
