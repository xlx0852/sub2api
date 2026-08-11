-- Window-aligned utilization/cost samples used for confidence-aware quota estimates.

CREATE TABLE IF NOT EXISTS account_quota_usage_observations (
    id              BIGSERIAL PRIMARY KEY,
    quota_window_id BIGINT NOT NULL REFERENCES account_quota_windows(id) ON DELETE CASCADE,
    account_id      BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform        VARCHAR(32) NOT NULL DEFAULT '',
    kind            VARCHAR(16) NOT NULL,
    observed_at     TIMESTAMPTZ NOT NULL,
    used_percent    DOUBLE PRECISION NOT NULL,
    requests        BIGINT NOT NULL DEFAULT 0,
    tokens          BIGINT NOT NULL DEFAULT 0,
    account_cost    NUMERIC(20,10) NOT NULL DEFAULT 0,
    standard_cost   NUMERIC(20,10) NOT NULL DEFAULT 0,
    user_cost       NUMERIC(20,10) NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_quota_usage_observations_used_chk
        CHECK (used_percent >= 0 AND used_percent <= 100),
    CONSTRAINT account_quota_usage_observations_nonnegative_chk
        CHECK (requests >= 0 AND tokens >= 0 AND account_cost >= 0 AND standard_cost >= 0 AND user_cost >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_account_quota_usage_observations_window_used
    ON account_quota_usage_observations (quota_window_id, used_percent);

CREATE INDEX IF NOT EXISTS idx_account_quota_usage_observations_window_observed
    ON account_quota_usage_observations (quota_window_id, observed_at ASC);

CREATE INDEX IF NOT EXISTS idx_account_quota_usage_observations_account_kind_observed
    ON account_quota_usage_observations (account_id, kind, observed_at DESC);

