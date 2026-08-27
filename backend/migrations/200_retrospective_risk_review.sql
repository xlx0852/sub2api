CREATE TABLE IF NOT EXISTS content_moderation_evidence (
    id VARCHAR(36) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    request_id VARCHAR(255) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    provider VARCHAR(64) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL DEFAULT '',
    content_hash CHAR(64) NOT NULL,
    capture_day DATE NOT NULL,
    capture_reason VARCHAR(64) NOT NULL DEFAULT 'baseline',
    object_key VARCHAR(1024) NOT NULL UNIQUE,
    key_version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'reserved',
    plaintext_bytes BIGINT NOT NULL DEFAULT 0,
    stored_bytes BIGINT NOT NULL DEFAULT 0,
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    redacted_excerpt VARCHAR(512) NOT NULL DEFAULT '',
    review_attempts INT NOT NULL DEFAULT 0,
    last_error VARCHAR(1024) NOT NULL DEFAULT '',
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stored_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    reviewed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT chk_content_moderation_evidence_status CHECK (status IN ('reserved', 'stored', 'reviewing', 'reviewed', 'failed', 'expired', 'deleted')),
    CONSTRAINT uq_content_moderation_evidence_daily_content UNIQUE (user_id, content_hash, capture_day),
    CONSTRAINT uq_content_moderation_evidence_request_content UNIQUE (request_id, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_evidence_review_queue
    ON content_moderation_evidence(status, captured_at, id)
    WHERE status IN ('stored', 'reviewing');
CREATE INDEX IF NOT EXISTS idx_content_moderation_evidence_user_day
    ON content_moderation_evidence(user_id, capture_day);
CREATE INDEX IF NOT EXISTS idx_content_moderation_evidence_expires
    ON content_moderation_evidence(expires_at)
    WHERE status NOT IN ('expired', 'deleted');

CREATE TABLE IF NOT EXISTS content_moderation_review_runs (
    id BIGSERIAL PRIMARY KEY,
    run_key VARCHAR(128) NOT NULL UNIQUE,
    scheduled_day DATE NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    manual BOOLEAN NOT NULL DEFAULT FALSE,
    policy_version VARCHAR(64) NOT NULL,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    claimed_count BIGINT NOT NULL DEFAULT 0,
    reviewed_count BIGINT NOT NULL DEFAULT 0,
    flagged_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    cost_units NUMERIC(24, 12) NOT NULL DEFAULT 0,
    error VARCHAR(2048) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    CONSTRAINT chk_content_moderation_review_runs_status CHECK (status IN ('running', 'completed', 'budget_limited', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_review_runs_started
    ON content_moderation_review_runs(started_at DESC);

CREATE TABLE IF NOT EXISTS content_moderation_review_items (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES content_moderation_review_runs(id) ON DELETE CASCADE,
    evidence_id VARCHAR(36) NOT NULL REFERENCES content_moderation_evidence(id) ON DELETE CASCADE,
    policy_version VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    flagged BOOLEAN NOT NULL DEFAULT FALSE,
    highest_category VARCHAR(128) NOT NULL DEFAULT '',
    highest_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    redacted_summary VARCHAR(1024) NOT NULL DEFAULT '',
    input_tokens BIGINT NOT NULL DEFAULT 0,
    cost_units NUMERIC(24, 12) NOT NULL DEFAULT 0,
    error VARCHAR(2048) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    CONSTRAINT chk_content_moderation_review_items_status CHECK (status IN ('running', 'completed', 'failed')),
    CONSTRAINT uq_content_moderation_review_item_policy UNIQUE (evidence_id, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_review_items_run
    ON content_moderation_review_items(run_id, id);

CREATE TABLE IF NOT EXISTS content_moderation_user_risk_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    level VARCHAR(16) NOT NULL DEFAULT 'normal',
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    provenance VARCHAR(64) NOT NULL DEFAULT '',
    review_status VARCHAR(32) NOT NULL DEFAULT 'unreviewed',
    expires_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_content_moderation_user_risk_level CHECK (level IN ('normal', 'watch', 'high', 'critical')),
    CONSTRAINT chk_content_moderation_user_risk_review_status CHECK (review_status IN ('unreviewed', 'confirmed', 'cleared'))
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_user_risk_profiles_active
    ON content_moderation_user_risk_profiles(level, expires_at, updated_at DESC);

CREATE TABLE IF NOT EXISTS content_moderation_risk_events (
    id BIGSERIAL PRIMARY KEY,
    event_key VARCHAR(160) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    evidence_id VARCHAR(36) REFERENCES content_moderation_evidence(id) ON DELETE SET NULL,
    review_item_id BIGINT REFERENCES content_moderation_review_items(id) ON DELETE SET NULL,
    event_type VARCHAR(64) NOT NULL,
    previous_level VARCHAR(16) NOT NULL DEFAULT 'normal',
    new_level VARCHAR(16) NOT NULL DEFAULT 'normal',
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    source VARCHAR(64) NOT NULL DEFAULT 'retrospective',
    actor_admin_id BIGINT,
    note VARCHAR(1024) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_risk_events_user
    ON content_moderation_risk_events(user_id, created_at DESC);
