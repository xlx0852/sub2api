-- Real quota-window ledger per account (not calendar projection).
-- One open row per (account_id, kind); history rows keep final start/end for P&L.

CREATE TABLE IF NOT EXISTS account_quota_windows (
    id              BIGSERIAL PRIMARY KEY,
    account_id      BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform        VARCHAR(32) NOT NULL DEFAULT '',
    kind            VARCHAR(16) NOT NULL,
    start_at        TIMESTAMPTZ NOT NULL,
    end_at          TIMESTAMPTZ NOT NULL,
    window_minutes  INT,
    source          VARCHAR(32) NOT NULL DEFAULT 'observed',
    closed_reason   VARCHAR(32),
    used_percent_open  DOUBLE PRECISION,
    used_percent_close DOUBLE PRECISION,
    is_open         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_quota_windows_range_chk CHECK (end_at > start_at)
);

CREATE INDEX IF NOT EXISTS idx_account_quota_windows_account_kind_start
    ON account_quota_windows (account_id, kind, start_at DESC);

CREATE INDEX IF NOT EXISTS idx_account_quota_windows_account_open
    ON account_quota_windows (account_id, kind)
    WHERE is_open = TRUE;

-- At most one open window per account+kind.
CREATE UNIQUE INDEX IF NOT EXISTS uq_account_quota_windows_one_open
    ON account_quota_windows (account_id, kind)
    WHERE is_open = TRUE;
