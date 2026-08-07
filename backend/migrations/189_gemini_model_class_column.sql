-- Gemini 配额聚合的 flash/pro 分类生成列。
-- 背景: GetGeminiUsageTotalsBatch 原用 LOWER(COALESCE(model,'')) LIKE '%flash%' OR LIKE '%lite%'
-- 在窗口内逐行分类, 前导通配符使 model 索引失效、逐行表达式计算。
-- model_class 为 STORED 生成列(表达式 immutable), 语义与 Go 侧 geminiModelClassFromName
-- (internal/service/gemini_quota.go: lower+Contains "flash"/"lite") 及
-- usage_log_repo_stats.go 注释声明一致; 查询改为 model_class = 'flash' 即可走索引。
-- 索引单独放在 189b_gemini_model_class_index_notx.sql（非事务执行）。
-- 注意: ADD COLUMN 会整表重写(usage_logs 为 insert-only 归档表, 一次性成本),
-- 建议低峰执行; 186 迁移已在同表加过 STORED 生成列(先例)。

ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS model_class TEXT
  GENERATED ALWAYS AS (
    CASE WHEN LOWER(model) LIKE '%flash%' OR LOWER(model) LIKE '%lite%'
         THEN 'flash' ELSE 'pro' END
  ) STORED;

COMMENT ON COLUMN usage_logs.model_class IS
  'Gemini flash/pro 分类(STORED 生成列), 与 Go 侧 geminiModelClassFromName 语义一致; 供 quota 聚合走索引替代 LIKE 分类';
