ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_output_stages JSONB;

COMMENT ON COLUMN usage_logs.first_output_stages IS
    'Gateway-side first-output latency stages; null means the request predates or does not support stage instrumentation.';

ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS first_output_eligible_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_pre_upstream_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_pre_upstream_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_connect_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_connect_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_first_output_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_first_output_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS retry_failover_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS retry_failover_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_first_output_write_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_first_output_write_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_first_output_total_sum_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_first_output_total_count BIGINT NOT NULL DEFAULT 0;
