-- 订阅账号被上游封禁后的终止与实际退款账本。
CREATE TABLE IF NOT EXISTS account_subscription_terminations (
    id BIGSERIAL PRIMARY KEY,
    cycle_id BIGINT NOT NULL REFERENCES account_subscription_cycles(id) ON DELETE RESTRICT,
    account_id BIGINT NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    reason VARCHAR(64) NOT NULL DEFAULT 'upstream_banned',
    notes TEXT NOT NULL DEFAULT '',
    reversed_at TIMESTAMPTZ,
    reversal_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS account_subscription_terminations_active_cycle_idx
    ON account_subscription_terminations (cycle_id)
    WHERE reversed_at IS NULL;

CREATE INDEX IF NOT EXISTS account_subscription_terminations_account_effective_idx
    ON account_subscription_terminations (account_id, effective_at DESC);

CREATE TABLE IF NOT EXISTS account_subscription_refunds (
    id BIGSERIAL PRIMARY KEY,
    termination_id BIGINT NOT NULL REFERENCES account_subscription_terminations(id) ON DELETE RESTRICT,
    amount NUMERIC(20,8) NOT NULL CHECK (amount > 0),
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    received_at TIMESTAMPTZ NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    voided_at TIMESTAMPTZ,
    void_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS account_subscription_refunds_termination_received_idx
    ON account_subscription_refunds (termination_id, received_at ASC);

CREATE INDEX IF NOT EXISTS account_subscription_refunds_active_received_idx
    ON account_subscription_refunds (received_at ASC)
    WHERE voided_at IS NULL;
