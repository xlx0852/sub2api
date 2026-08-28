CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_settlement_attempts_started_reconciliation
    ON usage_settlement_attempts (reconciliation_deadline, id)
    WHERE state = 'started' AND reconciliation_deadline IS NOT NULL;
