-- Change-only snapshots for external provider status observability.

CREATE TABLE IF NOT EXISTS provider_status_snapshots (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    source_url TEXT NOT NULL,
    overall_indicator VARCHAR(32) NOT NULL,
    overall_description VARCHAR(128) NOT NULL,
    components JSONB NOT NULL DEFAULT '[]'::jsonb,
    incidents JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_updated_at TIMESTAMPTZ,
    fetched_at TIMESTAMPTZ NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_status_provider_hash
    ON provider_status_snapshots (provider, content_hash);

CREATE INDEX IF NOT EXISTS idx_provider_status_provider_fetched
    ON provider_status_snapshots (provider, fetched_at DESC);

COMMENT ON TABLE provider_status_snapshots IS
    'Change-only snapshots of public provider status sources for Ops correlation; never used for gateway routing.';
