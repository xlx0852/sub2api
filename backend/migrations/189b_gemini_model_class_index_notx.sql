-- 覆盖索引: 覆盖 GetGeminiUsageTotalsBatch 的 (account_id, created_at) 过滤 + model_class 分类列。
-- 依赖 189_gemini_model_class_column.sql 先建好 model_class 生成列。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_account_created_at_model_class
  ON usage_logs (account_id, created_at) INCLUDE (model_class);
