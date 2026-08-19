-- Add source-currency metadata only. Existing monetary values remain unchanged.
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'USD';

UPDATE channel_model_pricing
SET currency = 'USD'
WHERE currency IS NULL OR BTRIM(currency) = '';

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS pricing_currency VARCHAR(3);

COMMENT ON COLUMN channel_model_pricing.currency IS 'Model base-price source currency (USD/CNY); never triggers automatic FX conversion';
COMMENT ON COLUMN usage_logs.pricing_currency IS 'Immutable model base-price currency snapshot (USD/CNY); NULL means legacy USD display fallback';

-- Comprehensive cost multipliers are operator-controlled. Keep upstream probes
-- observational, but remove the legacy flag that allowed them to overwrite rate_multiplier.
UPDATE accounts
SET extra = extra - 'upstream_billing_rate_sync_enabled',
    updated_at = NOW()
WHERE extra ? 'upstream_billing_rate_sync_enabled';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'channel_model_pricing_currency_check'
    ) THEN
        ALTER TABLE channel_model_pricing
            ADD CONSTRAINT channel_model_pricing_currency_check CHECK (currency IN ('USD', 'CNY'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_pricing_currency_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_pricing_currency_check CHECK (pricing_currency IS NULL OR pricing_currency IN ('USD', 'CNY'));
    END IF;
END $$;
