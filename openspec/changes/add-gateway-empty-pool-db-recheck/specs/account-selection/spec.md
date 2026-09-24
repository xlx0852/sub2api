## ADDED Requirements

### Requirement: Empty scheduler pool must recheck the database before failing
When the gateway candidate pool for a group/platform bucket is empty — either the bucket listing returned no
schedulable account or no account passed the hard candidate gates — the gateway SHALL discard a snapshot-only
result, force one database-backed refresh of that bucket through the existing throttled refresh path, and retry
candidate selection once. The recheck MUST be skipped when the caller excluded accounts, when candidates exist
but stay unschedulable (busy pool / bounded wait), or when no scheduler snapshot service is configured. The
gateway SHALL emit `gateway_scheduler_empty_pool_db_recheck` on a performed refresh,
`gateway_scheduler_empty_pool_db_recheck_failed` when the refresh itself errors (throttling excepted), and
`gateway_scheduler_empty_pool_db_recheck_unrecovered` when the retry still yields no account.

#### Scenario: Snapshot omits an account the database still has
- **WHEN** the first candidate pass returns no candidate for the requested model because the snapshot for that bucket is missing an account that the database still lists as supporting it
- **THEN** the bucket is refreshed from the database, the selection is retried once, and the request is served by that account instead of failing with model_not_found

#### Scenario: No account supports the requested model even in the database
- **WHEN** the forced database refresh returns a pool that still has no account supporting the requested model
- **THEN** the request keeps the existing no-account classification and a `gateway_scheduler_empty_pool_db_recheck_unrecovered` warning is recorded

#### Scenario: Failover exclusions or a busy pool
- **WHEN** the caller excluded accounts after a prior failure, or candidates exist but no concurrency slot or wait plan is available
- **THEN** no database refresh is attempted and the existing result and error classification are unchanged

#### Scenario: Recheck throttling
- **WHEN** a forced refresh for the same group/platform bucket already ran inside the throttle window
- **THEN** the bucket is not read from the database again and the original no-account error is returned unchanged
