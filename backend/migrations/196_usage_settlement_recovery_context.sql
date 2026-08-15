ALTER TABLE usage_settlement_attempts
    ADD COLUMN IF NOT EXISTS recovery_context_version SMALLINT NULL,
    ADD COLUMN IF NOT EXISTS recovery_context JSONB NULL;

ALTER TABLE usage_settlement_attempts
    DROP CONSTRAINT IF EXISTS usage_settlement_attempts_recovery_context_pair_check;

ALTER TABLE usage_settlement_attempts
    ADD CONSTRAINT usage_settlement_attempts_recovery_context_pair_check CHECK (
        (recovery_context IS NULL AND recovery_context_version IS NULL)
        OR (recovery_context IS NOT NULL AND recovery_context_version = 1)
    );

COMMENT ON COLUMN usage_settlement_attempts.recovery_context IS
    'Versioned non-secret scalar context for exact stored-response usage recovery; never stores prompts, response bodies, or credentials';
