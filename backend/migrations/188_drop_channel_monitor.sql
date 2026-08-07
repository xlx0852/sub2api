-- Migration: 188_drop_channel_monitor
-- 下线「端点监控」（主动探测）。分组可用性改为真实流量口径
-- （usage_logs 成功 + ops_error_logs 失败），见 admin groups/:id/availability。
-- CASCADE 处理 FK 依赖（templates→monitors 等）。

DROP TABLE IF EXISTS channel_monitor_histories CASCADE;
DROP TABLE IF EXISTS channel_monitor_daily_rollups CASCADE;
DROP TABLE IF EXISTS channel_monitor_aggregation_watermark CASCADE;
DROP TABLE IF EXISTS channel_monitor_request_templates CASCADE;
DROP TABLE IF EXISTS channel_monitors CASCADE;
