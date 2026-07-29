-- 每次实际充值/订阅生效形成独立周期；周期之间允许存在停用空档。
CREATE TABLE IF NOT EXISTS account_subscription_cycles (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL,
    starts_at DATE NOT NULL,
    period_fee NUMERIC(20,8) NOT NULL CHECK (period_fee >= 0),
    period_days INTEGER NOT NULL CHECK (period_days > 0),
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS account_subscription_cycles_account_starts_idx
    ON account_subscription_cycles (account_id, starts_at DESC);
