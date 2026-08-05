-- 模型级全局并发预算（JSON map：canonical model → 并发上限）。
-- 仅对配置了预算的模型生效；值为 0 或缺失 = 不限制。
-- 预算满时该模型请求会被降级让路（HTTP 429 / WS 1013），不再抢占其它模型的账号并发。
INSERT INTO settings (key, value, updated_at) VALUES ('model_concurrency_limits', '{"gpt-5.6-luna":8}', NOW())
ON CONFLICT (key) DO NOTHING;
