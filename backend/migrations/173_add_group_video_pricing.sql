ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_rate_independent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS video_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS video_price_480p DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS video_price_720p DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS video_price_1080p DECIMAL(20,8);

COMMENT ON COLUMN groups.video_rate_independent IS '视频生成是否使用独立倍率；false 表示共享分组有效倍率';
COMMENT ON COLUMN groups.video_rate_multiplier IS '视频生成独立倍率，仅 video_rate_independent=true 时生效';
COMMENT ON COLUMN groups.video_price_480p IS '480p 视频生成每秒价格（USD/s）';
COMMENT ON COLUMN groups.video_price_720p IS '720p 视频生成每秒价格（USD/s）';
COMMENT ON COLUMN groups.video_price_1080p IS '1080p 视频生成每秒价格（USD/s）';
