-- Migration 170 expanded request_type to include compact=5, but migration 173
-- accidentally rewrote the CHECK constraint back to (0,1,2,3,4) only.
-- Compact usage rows (request_type=5) then fail with:
--   pq: new row for relation "usage_logs" violates check constraint
--   "usage_logs_request_type_check"
--
-- Restore the full allow-list:
--   0=unknown, 1=sync, 2=stream, 3=ws_v2, 4=cyber, 5=compact
ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3, 4, 5)) NOT VALID;
