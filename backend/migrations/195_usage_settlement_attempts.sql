-- Durable handoff for terminal provider usage. This table is additive and is
-- not consumed by the legacy billing path until the settlement worker is
-- explicitly enabled by a later rollout phase.
CREATE TABLE IF NOT EXISTS usage_settlement_attempts (
    id BIGSERIAL PRIMARY KEY,
    billing_identity VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    group_id BIGINT NULL,
    provider VARCHAR(32) NOT NULL,
    request_type SMALLINT NOT NULL DEFAULT 0,
    provider_request_id VARCHAR(255) NULL,
    inbound_endpoint TEXT NULL,
    upstream_endpoint TEXT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'started',
    payload_version SMALLINT NOT NULL DEFAULT 1,
    payload JSONB NULL,
    payload_hash VARCHAR(64) NULL,
    lease_owner VARCHAR(128) NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NULL,
    reconciliation_deadline TIMESTAMPTZ NULL,
    last_error_code VARCHAR(64) NULL,
    last_error_message TEXT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ready_at TIMESTAMPTZ NULL,
    settled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_settlement_attempts_state_check CHECK (state IN (
        'started',
        'ready',
        'processing',
        'settled',
        'reconciliation_required',
        'failed_retryable',
        'failed_terminal'
    )),
    CONSTRAINT usage_settlement_attempts_payload_pair_check CHECK (
        (payload IS NULL AND payload_hash IS NULL)
        OR (payload IS NOT NULL AND payload_hash IS NOT NULL)
    ),
    CONSTRAINT usage_settlement_attempts_ready_payload_check CHECK (
        state NOT IN ('ready', 'processing', 'settled', 'failed_retryable')
        OR payload IS NOT NULL
    ),
    CONSTRAINT usage_settlement_attempts_lease_pair_check CHECK (
        (lease_owner IS NULL AND lease_expires_at IS NULL)
        OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
    ),
    CONSTRAINT usage_settlement_attempts_settled_at_check CHECK (
        (state = 'settled' AND settled_at IS NOT NULL)
        OR (state <> 'settled' AND settled_at IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_settlement_attempts_identity_api_key
    ON usage_settlement_attempts (billing_identity, api_key_id);

CREATE INDEX IF NOT EXISTS idx_usage_settlement_attempts_claim
    ON usage_settlement_attempts (state, next_attempt_at, lease_expires_at, id)
    WHERE state IN ('ready', 'processing', 'failed_retryable');

CREATE INDEX IF NOT EXISTS idx_usage_settlement_attempts_reconciliation
    ON usage_settlement_attempts (provider, reconciliation_deadline, id)
    WHERE state = 'reconciliation_required';

CREATE INDEX IF NOT EXISTS idx_usage_settlement_attempts_account_created
    ON usage_settlement_attempts (account_id, created_at DESC);

COMMENT ON TABLE usage_settlement_attempts IS 'Recoverable settlement handoff keyed by a locally controlled request or turn identity';
COMMENT ON COLUMN usage_settlement_attempts.payload IS 'Versioned billing and usage-log scalar snapshot; never stores prompts, response bodies, or credentials';
