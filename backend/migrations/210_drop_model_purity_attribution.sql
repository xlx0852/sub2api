-- 210: 彻底移除模型纯度归因（ModelTrace 指纹检查）。
--
-- 该能力被判定为无用：每次探测要向上游发送数百个整数的长挑战，消耗账号配额，
-- 而结论只是闭集内的概率估计。操作员要求完全下掉，因此结构与数据一并删除，不再保留。
--
-- 顺序要点：scheduled_test_results.purity_run_id 外键引用 model_attribution_runs，
-- 必须先删引用列，再删表（否则 PostgreSQL 报 other objects depend on it）。
ALTER TABLE scheduled_test_results
    DROP COLUMN IF EXISTS purity_run_id;

DROP TABLE IF EXISTS model_attribution_runs CASCADE;

ALTER TABLE scheduled_test_plans
    DROP COLUMN IF EXISTS auto_purity,
    DROP COLUMN IF EXISTS purity_interval_minutes,
    DROP COLUMN IF EXISTS purity_answers;
