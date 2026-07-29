-- 删除 billing_cycle_day：计费窗口已改为从 credentials.expires_at 推导，该字段不再参与计算。
ALTER TABLE account_cost_configs DROP COLUMN IF EXISTS billing_cycle_day;
