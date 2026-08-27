CREATE TABLE IF NOT EXISTS content_moderation_evidence_access_grants (
    id BIGSERIAL PRIMARY KEY,
    evidence_id VARCHAR(36) NOT NULL REFERENCES content_moderation_evidence(id) ON DELETE CASCADE,
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash CHAR(64) NOT NULL UNIQUE,
    reason VARCHAR(512) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_evidence_access_grants_lookup
    ON content_moderation_evidence_access_grants(evidence_id, admin_id, expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE IF NOT EXISTS content_moderation_operator_audits (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(64) NOT NULL,
    evidence_id VARCHAR(36) REFERENCES content_moderation_evidence(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reason VARCHAR(512) NOT NULL DEFAULT '',
    outcome VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_content_moderation_operator_audit_outcome
        CHECK (outcome IN ('granted','denied','success','failed'))
);

CREATE INDEX IF NOT EXISTS idx_content_moderation_operator_audits_created
    ON content_moderation_operator_audits(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_content_moderation_operator_audits_evidence
    ON content_moderation_operator_audits(evidence_id, created_at DESC)
    WHERE evidence_id IS NOT NULL;

-- A crashed reviewer must not leave evidence permanently claimed. Old claims are
-- returned to the retry queue by ClaimEvidence; this index keeps that scan bounded.
CREATE INDEX IF NOT EXISTS idx_content_moderation_evidence_stale_claims
    ON content_moderation_evidence(claimed_at, id)
    WHERE status = 'reviewing';
