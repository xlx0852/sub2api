-- 上游响应模型审计：记录上游回包声明的模型，以及与「送上游模型」是否一致。
-- 两列都可空且无默认值，PostgreSQL 11+ 为瞬时加列（不重写表）。
--   upstream_response_model：上游响应（协议转换/客户端改写前）声明的模型名。
--   upstream_model_mismatch：三态，NULL 表示上游未声明模型（或该行早于此字段）。
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200),
    ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN;
