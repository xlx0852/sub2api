-- Per-API-key force OpenAI Fast (service_tier=priority) mode.
-- When enabled, OpenAI gateway requests for this key are forced to priority
-- and billed at Fast/priority rates, unless admin Fast policy blocks priority.
ALTER TABLE api_keys
  ADD COLUMN IF NOT EXISTS force_openai_fast BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN api_keys.force_openai_fast IS
  'When true, force OpenAI gateway service_tier=priority (Fast) for this API key and bill accordingly';
