-- 207: 账号模型纯度归因（主动式模型归因）运行记录。
--
-- 每次探针运行落一行：账号、实际探测模型、账号声称的模型、归因结论、概率分布、
-- 以及逐条挑战的原始回答（JSONB）。运行本身不改动账号调度状态，账号删除时级联清理。
CREATE TABLE IF NOT EXISTS model_attribution_runs (
    id                     BIGSERIAL PRIMARY KEY,
    account_id             BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    status                 VARCHAR(20) NOT NULL DEFAULT 'succeeded',
    platform               VARCHAR(50) NOT NULL DEFAULT '',
    transport              VARCHAR(50) NOT NULL DEFAULT '',
    probe_model            VARCHAR(200) NOT NULL DEFAULT '',
    claimed_model          VARCHAR(200) NOT NULL DEFAULT '',
    claimed_candidate      VARCHAR(200),
    verdict                VARCHAR(20) NOT NULL DEFAULT 'inconclusive',
    prediction             VARCHAR(200),
    prediction_display_name VARCHAR(200),
    probability            DOUBLE PRECISION,
    family_prediction      VARCHAR(50),
    family_probability     DOUBLE PRECISION,
    probabilities          JSONB,
    bank_digest            VARCHAR(100) NOT NULL DEFAULT '',
    bank_source            VARCHAR(300) NOT NULL DEFAULT '',
    calibration_queries    VARCHAR(10) NOT NULL DEFAULT '',
    calibration_beta       DOUBLE PRECISION,
    cv_accuracy            DOUBLE PRECISION,
    answers                JSONB,
    error_message          TEXT NOT NULL DEFAULT '',
    duration_ms            INTEGER NOT NULL DEFAULT 0,
    created_by             BIGINT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_attribution_runs_account_created
    ON model_attribution_runs (account_id, created_at DESC);

-- 账号列表按「最近一次结论」排序/过滤时需要找到每个账号最新的一行。
CREATE INDEX IF NOT EXISTS idx_model_attribution_runs_account_latest
    ON model_attribution_runs (account_id, id DESC);
