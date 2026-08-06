-- 订阅成本配置：开启后到期按上一期费用/天数自动写入下一充值周期。
ALTER TABLE account_cost_configs
    ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN account_cost_configs.auto_renew IS
    '订阅自动续期：true 时在上一周期结束后按相同 period_fee/period_days 自动创建下一周期';
