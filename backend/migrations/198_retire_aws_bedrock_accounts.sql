UPDATE accounts
SET status = 'inactive',
    schedulable = FALSE,
    updated_at = NOW()
WHERE LOWER(TRIM(type)) = 'bedrock'
  AND (status <> 'inactive' OR schedulable = TRUE);
