## ADDED Requirements

### Requirement: Kimi OAuth quota reporting

The backend SHALL query the Kimi coding usage endpoint for Kimi OAuth accounts
and expose the five-hour and weekly quota windows through the existing account
usage response.

#### Scenario: Successful quota query

- **WHEN** a Kimi OAuth account's usage is requested
- **THEN** the backend sends an authenticated request with the account's Kimi
  device headers and returns `five_hour` and `seven_day` utilization and reset
  times when supplied by the upstream response, together with local window
  statistics when usage logs are available.

#### Scenario: Local request statistics and estimate

- **WHEN** the account has successful usage logs in the official five-hour or
  weekly window
- **THEN** the usage response includes request, token, and cost statistics for
  each window, allowing the existing UI to linearly project consumption at
  100% utilization.

#### Scenario: Expired access token

- **WHEN** the first quota request returns HTTP 401
- **THEN** the backend refreshes the Kimi OAuth token and retries once before
  returning a reauthentication error.

### Requirement: Kimi quota refresh cadence

The backend SHALL cache successful Kimi quota results for ten minutes per
account, coalesce concurrent requests, and allow an explicit forced refresh to
bypass the success cache.

#### Scenario: Cached result

- **WHEN** the same account is read again within ten minutes without force
- **THEN** the backend returns the cached quota without another upstream call.

#### Scenario: Forced refresh

- **WHEN** an administrator explicitly refreshes the account quota
- **THEN** the backend performs a new upstream query even if the ten-minute
  cache entry is still fresh.
