-- 212: 播种 L3 修饰因子，把原本写死在代码里的倍率变成数据。
--
-- 播种后行为仍然由 pricing.pipeline 决定：
--   * legacy（默认）完全不读 price_modifiers，扣费行为不变；
--   * 切到 unified 后，这里的记录才接管峰谷与档位倍率。
--
-- 关键语义：同一 scope 内"首个命中生效"（按 priority, id 排序），因此峰时窗口用
-- priority 10 的 ×1.0 行表达，兜底行用 priority 100 的 ×0.5 行表达。若改成连乘，
-- 两条峰时窗口会各乘一次 0.5，把峰时价算成 1/4。

INSERT INTO price_modifiers (scope, target, kind, when_mode, window_start, window_end, tz, weekdays, tier_value, multiplier, enabled, priority, notes)
SELECT 'vendor_time', 'deepseek-v4*', 'window', 'inside', '01:00', '04:00', 'UTC', '{1,2,3,4,5}', '', 1.0, TRUE, 10,
       'DeepSeek 官方峰时窗口 A（工作日 UTC 01:00-04:00），峰时按原价'
WHERE NOT EXISTS (
    SELECT 1 FROM price_modifiers
    WHERE scope = 'vendor_time' AND target = 'deepseek-v4*'
      AND window_start = '01:00' AND window_end = '04:00'
);

INSERT INTO price_modifiers (scope, target, kind, when_mode, window_start, window_end, tz, weekdays, tier_value, multiplier, enabled, priority, notes)
SELECT 'vendor_time', 'deepseek-v4*', 'window', 'inside', '06:00', '10:00', 'UTC', '{1,2,3,4,5}', '', 1.0, TRUE, 10,
       'DeepSeek 官方峰时窗口 B（工作日 UTC 06:00-10:00），峰时按原价'
WHERE NOT EXISTS (
    SELECT 1 FROM price_modifiers
    WHERE scope = 'vendor_time' AND target = 'deepseek-v4*'
      AND window_start = '06:00' AND window_end = '10:00'
);

INSERT INTO price_modifiers (scope, target, kind, when_mode, window_start, window_end, tz, weekdays, tier_value, multiplier, enabled, priority, notes)
SELECT 'vendor_time', 'deepseek-v4*', 'window', 'inside', '', '', 'UTC', NULL, '', 0.5, TRUE, 100,
       'DeepSeek 谷时兜底：两个峰时窗口之外（含周末）按 50% 计费'
WHERE NOT EXISTS (
    SELECT 1 FROM price_modifiers
    WHERE scope = 'vendor_time' AND target = 'deepseek-v4*'
      AND COALESCE(window_start, '') = '' AND COALESCE(window_end, '') = ''
);

INSERT INTO price_modifiers (scope, target, kind, when_mode, window_start, window_end, tz, weekdays, tier_value, multiplier, enabled, priority, notes)
SELECT 'request_tier', '*', 'tier', 'inside', '', '', 'UTC', NULL, 'flex', 0.5, TRUE, 10,
       'Flex 服务档位官方统一 0.5 倍（原 serviceTierCostMultiplier 常量）'
WHERE NOT EXISTS (
    SELECT 1 FROM price_modifiers
    WHERE scope = 'request_tier' AND tier_value = 'flex'
);
