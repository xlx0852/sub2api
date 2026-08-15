ALTER TABLE usage_settlement_attempts
    ADD COLUMN IF NOT EXISTS authoritative BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_usage_settlement_attempts_authoritative_claim
    ON usage_settlement_attempts (state, next_attempt_at, lease_expires_at, id)
    WHERE authoritative = TRUE
      AND state IN ('ready', 'processing', 'failed_retryable');

COMMENT ON COLUMN usage_settlement_attempts.authoritative IS
    'Immutable claim eligibility captured when the attempt is created; shadow and pre-migration rows remain false';
