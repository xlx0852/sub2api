-- 208: 诊断（定时测试）与模型纯度归因联动。
--
-- 计划级开关：诊断跑连通性时顺带做一次主动式纯度归因；间隔限制避免每次 cron 都烧上游 token。
-- 结果级关联：诊断结果挂上本次产生的纯度运行，便于在同一个面板里回看。
ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS auto_purity BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS purity_interval_minutes INTEGER NOT NULL DEFAULT 1440,
    ADD COLUMN IF NOT EXISTS purity_answers INTEGER NOT NULL DEFAULT 1;

ALTER TABLE scheduled_test_results
    ADD COLUMN IF NOT EXISTS purity_run_id BIGINT REFERENCES model_attribution_runs(id) ON DELETE SET NULL;

-- 记录触发来源，区分操作员手动点击与诊断计划自动触发。
ALTER TABLE model_attribution_runs
    ADD COLUMN IF NOT EXISTS trigger VARCHAR(20) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS plan_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_model_attribution_runs_account_trigger_created
    ON model_attribution_runs (account_id, trigger, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scheduled_test_results_purity_run
    ON scheduled_test_results (purity_run_id) WHERE purity_run_id IS NOT NULL;
