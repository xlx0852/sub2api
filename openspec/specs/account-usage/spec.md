# account-usage Specification

## Purpose
提供 Kimi OAuth 配额查询、缓存刷新及配额恢复信息，统一账号用量展示与调度恢复依据。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

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

### Requirement: Kimi quota scheduling recovery

The system SHALL remove a Kimi OAuth account from scheduling while any official
quota window is full and SHALL automatically make it eligible again no later
than the latest full window's official reset time.

#### Scenario: Official window is full

- **WHEN** a successful Kimi quota query reports a five-hour or weekly window at 100 percent with a future reset time
- **THEN** the account is temporarily unschedulable until the latest full-window reset

#### Scenario: Official window resets

- **WHEN** the temporary block reaches its official reset time
- **THEN** normal scheduling eligibility automatically resumes without an operator action

#### Scenario: Gateway receives quota 429

- **WHEN** a Kimi OAuth request receives HTTP 429
- **THEN** the gateway refreshes official quota in the background and synchronizes the scheduling block with the returned reset time
