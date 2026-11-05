-- 213: 回填 L1 成本基，并把已知成本基的售价行切换为 fx_auto。
--
-- 原则：切换前后客户价格必须完全一致。脚本先断言"派生值 = 当前值"（每 token 1e-12 容差，
-- 即每百万 token 1e-6 元），任一不符即抛异常整笔回滚，绝不带着偏差上线。
--
-- 汇率统一取 7.0（与运营当天的定价口径一致）。以后改汇率只需改 fx_rate + 重跑派生，
-- 不必逐行重算售价。

-- 1) 成本基（美元 / 每百万 token）
INSERT INTO price_costs (vendor, model_pattern, currency, input_per_mtok, cached_per_mtok, output_per_mtok, cache_write_per_mtok, discount_pct, observed_at, source_url, notes)
VALUES
    ('venice',     'deepseek-v4-1-flash-1017-uncensored', 'USD', 0.33,  0.00825, 1.32,  0.33,  0, '2026-11-05T00:00:00Z', 'https://api.venice.ai',     '运营给定 $0.33 / $0.00825 / $1.32'),
    ('refuseless', 'deepseek-v4.1-flash-abliterated',      'USD', 0.14,  0.035,   0.50,  0.14,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价（忽略限时折扣）'),
    ('refuseless', 'deepseek-v4.1-flash-abliterated-free', 'USD', 0.14,  0.035,   0.50,  0.14,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价（该账号暂未授权）'),
    ('refuseless', 'deepseek-v4.1-flash-fast',            'USD', 0.01,  0.002,   0.08,  0.01,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价'),
    ('refuseless', 'glm-5.3-abliterated',                 'USD', 1.00,  0.15,    2.00,  1.00,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价'),
    ('refuseless', 'glm-5.3-flash-abliterated',           'USD', 0.12,  0.04,    0.50,  0.12,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价（限时缓存折扣已忽略）'),
    ('refuseless', 'minimax-m3.1-flash',                  'USD', 0.02,  0.002,   0.08,  0.02,  0, '2026-11-05T00:00:00Z', 'https://refuseless.com/pricing', '官方原价'),
    ('hinoki',     'hinoki-offensive',                    'USD', 0.80,  0.08,    1.20,  0.80,  0, '2026-11-05T00:00:00Z', 'https://inference.hinoki.security/v1/models', '官方价目表（缓存写入未公布，按输入价计）')
ON CONFLICT (vendor, model_pattern) DO UPDATE
SET currency = EXCLUDED.currency,
    input_per_mtok = EXCLUDED.input_per_mtok,
    cached_per_mtok = EXCLUDED.cached_per_mtok,
    output_per_mtok = EXCLUDED.output_per_mtok,
    cache_write_per_mtok = EXCLUDED.cache_write_per_mtok,
    discount_pct = EXCLUDED.discount_pct,
    observed_at = EXCLUDED.observed_at,
    source_url = EXCLUDED.source_url,
    notes = EXCLUDED.notes,
    updated_at = NOW();

-- 2) 逐行断言 + 切换为 fx_auto
DO $$
DECLARE
    fx      numeric := 7.0;
    rec     record;
    cost    price_costs%ROWTYPE;
    d_in    numeric;
    d_out   numeric;
    d_cr    numeric;
    d_cw    numeric;
BEGIN
    FOR rec IN
        SELECT * FROM (VALUES
            (222::bigint, 'venice',     'deepseek-v4-1-flash-1017-uncensored'),
            (223::bigint, 'refuseless', 'deepseek-v4.1-flash-abliterated'),
            (224::bigint, 'refuseless', 'deepseek-v4.1-flash-abliterated-free'),
            (225::bigint, 'refuseless', 'deepseek-v4.1-flash-fast'),
            (226::bigint, 'refuseless', 'glm-5.3-abliterated'),
            (227::bigint, 'refuseless', 'glm-5.3-flash-abliterated'),
            (228::bigint, 'refuseless', 'minimax-m3.1-flash'),
            (229::bigint, 'hinoki',     'hinoki-offensive')
        ) AS t(pricing_id, vendor, model_pattern)
    LOOP
        SELECT * INTO cost FROM price_costs
        WHERE vendor = rec.vendor AND model_pattern = rec.model_pattern;
        CONTINUE WHEN NOT FOUND;

        d_in  := ROUND(cost.input_per_mtok       * (1 - cost.discount_pct / 100) * fx / 1000000, 12);
        d_out := ROUND(cost.output_per_mtok      * (1 - cost.discount_pct / 100) * fx / 1000000, 12);
        d_cr  := ROUND(COALESCE(cost.cached_per_mtok, 0)      * (1 - cost.discount_pct / 100) * fx / 1000000, 12);
        d_cw  := ROUND(COALESCE(cost.cache_write_per_mtok, cost.input_per_mtok) * (1 - cost.discount_pct / 100) * fx / 1000000, 12);

        IF EXISTS (
            SELECT 1 FROM channel_model_pricing p
            WHERE p.id = rec.pricing_id
              AND (
                    ABS(COALESCE(p.input_price, 0)       - d_in)  > 1e-12
                 OR ABS(COALESCE(p.output_price, 0)      - d_out) > 1e-12
                 OR ABS(COALESCE(p.cache_read_price, 0)  - d_cr)  > 1e-12
                 OR ABS(COALESCE(p.cache_write_price, 0) - d_cw)  > 1e-12
              )
        ) THEN
            RAISE EXCEPTION 'pricing row % (%) derives to % / % CNY per MTok but the stored price differs — aborting migration',
                rec.pricing_id, rec.model_pattern, ROUND(d_in * 1000000, 6)::text, ROUND(d_out * 1000000, 6)::text;
        END IF;

        UPDATE channel_model_pricing
        SET fx_mode = 'fx_auto',
            cost_id = cost.id,
            fx_rate = fx,
            markup_pct = cost.discount_pct * 0,
            input_price = d_in,
            output_price = d_out,
            cache_read_price = d_cr,
            cache_write_price = d_cw,
            updated_at = NOW()
        WHERE id = rec.pricing_id;
    END LOOP;
END $$;
