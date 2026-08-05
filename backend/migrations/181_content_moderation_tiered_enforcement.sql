ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS api_key_disabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS enforcement_error TEXT NOT NULL DEFAULT '';
