## ADDED Requirements

### Requirement: Claude OAuth / Setup Token supply has no production execution path

The system SHALL reject creation, import, conversion, re-authorization, credential application and token refresh of an Anthropic account whose type is `oauth` or `setup-token`. After retirement the system MUST NOT expose Anthropic authorization endpoints, generate authorization URLs or Setup Token URLs, exchange authorization codes or session cookies, hold a Claude token provider, use a Claude token cache, refresh Anthropic OAuth credentials in the background, or require a Claude OAuth session store in the runtime. Enforcement MUST exist below the HTTP and frontend layers, and MUST NOT alter Anthropic API-Key supply or any other provider's OAuth supply.

#### Scenario: Administrator attempts to create an Anthropic OAuth or Setup Token account

- **WHEN** an administrator submits a single or bulk account creation request with `platform=anthropic` and `type=oauth` or `type=setup-token`
- **THEN** the request is rejected and no account, credential, group binding, scheduler snapshot or other creation side effect is persisted

#### Scenario: Administrator converts an existing account into an Anthropic OAuth account

- **WHEN** an update request would change an existing account to `platform=anthropic` with `type=oauth` or `type=setup-token`
- **THEN** the change is rejected and the account keeps its previous platform and type

#### Scenario: Administrator opens the Anthropic account creation or re-authorization flow

- **WHEN** the Anthropic account options are rendered in the account creation, edit or re-authorization UI
- **THEN** the OAuth, Setup Token and cookie authorization options are not offered, and the Anthropic API-key option remains available

#### Scenario: Authorization endpoints are called directly

- **WHEN** a client calls any retired Claude authorization route (`generate-auth-url`, `generate-setup-token-url`, `exchange-code`, `exchange-setup-token-code`, `cookie-auth`, `setup-token-cookie-auth`)
- **THEN** the route is not registered and no authorization URL, token exchange or credential write occurs

#### Scenario: An existing Anthropic OAuth account is refreshed

- **WHEN** a single-account refresh, bulk refresh, re-authorization apply or background token refresh targets an Anthropic `oauth` / `setup-token` account
- **THEN** no Anthropic token endpoint is contacted, no credential or token cache entry is written, and the account is not made schedulable by the attempt

#### Scenario: CRS export contains Claude OAuth accounts

- **WHEN** an administrator syncs from a CRS instance whose export contains `claudeAccounts` with `authType=oauth` or `authType=setup-token`
- **THEN** those entries are not imported, while `claudeConsoleAccounts` (Anthropic API key), OpenAI, Gemini and other platform entries keep their existing import behavior

#### Scenario: Ordinary Anthropic traffic is scheduled

- **WHEN** Anthropic API-key traffic is admitted and forwarded after retirement
- **THEN** no Claude OAuth token provider, token cache, authorization endpoint or session store is reachable, and the API-key path is unchanged

### Requirement: Claude OAuth retirement preserves historical records

The retirement SHALL retain historical Anthropic `oauth` / `setup-token` account rows, their encrypted credentials, group references, usage, billing, settlement and audit records. The retirement MUST be idempotent, MUST NOT require a destructive schema or data migration, and MUST leave non-Anthropic and Anthropic API-key records untouched. Any Redis token-cache key for a retired account MUST be removable as a side effect of the retirement rather than reused.

#### Scenario: Retired accounts exist in the database

- **GIVEN** Anthropic `oauth` / `setup-token` rows existed before retirement
- **WHEN** the retired build serves account management, data export or usage/audit reads
- **THEN** the rows remain identifiable and deletable, expose no action that can refresh, re-authorize, test with upstream credentials or reactivate them, and no record is deleted or rewritten

#### Scenario: Retirement is deployed repeatedly

- **WHEN** the retired build is deployed on an environment where it has already been deployed, or on an environment with no retired rows
- **THEN** startup, migrations and traffic handling complete without error and without additional data changes

#### Scenario: Stale Claude token cache entries remain in Redis

- **GIVEN** a Claude token cache entry exists for an account id from before retirement
- **WHEN** traffic or refresh runs after retirement
- **THEN** the entry is not read or written by any Anthropic path and the removal of the cache key does not affect other providers' token caches
