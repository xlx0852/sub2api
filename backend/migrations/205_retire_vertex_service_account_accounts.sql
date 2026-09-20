UPDATE accounts
SET status = 'inactive',
    schedulable = FALSE,
    updated_at = NOW()
WHERE LOWER(TRIM(type)) = 'service_account'
  AND (status <> 'inactive' OR schedulable = TRUE);
