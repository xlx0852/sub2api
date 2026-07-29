## 1. Platform and OAuth

- [x] 1.1 Add the Kimi platform to backend and frontend platform registries
- [x] 1.2 Implement proxy-aware Kimi device authorization and token refresh
- [x] 1.3 Add Redis-backed start, status, cancellation, and one-time login-ticket handling
- [x] 1.5 Add an atomic polling lease and prevent cancellation/save races
- [x] 1.4 Create Kimi OAuth accounts without exposing refresh tokens to the browser

## 2. Token lifecycle and scheduling

- [x] 2.1 Add Kimi token accessor, refresher, cache key, and distributed refresh registration
- [x] 2.2 Preserve rotated refresh tokens, expiry, scope, and device identity
- [x] 2.3 Synchronize refreshed credentials with token and scheduler caches

## 3. Gateway compatibility

- [x] 3.1 Route Kimi Chat Completions and Messages-compatible requests to Kimi Coding
- [x] 3.2 Translate Responses input to Chat Completions for Kimi accounts
- [x] 3.3 Inject stable Kimi device headers and normalize upstream model identifiers
- [x] 3.4 Normalize Kimi thinking and tool-call message chains
- [x] 3.5 Parse streaming usage and preserve downstream protocol semantics

## 4. Administrator UI

- [x] 4.1 Add Kimi to account, group, channel, filter, badge, and icon surfaces
- [x] 4.2 Add a reusable device authorization component with code, countdown, polling, cancellation, and retry
- [x] 4.3 Integrate Kimi OAuth account creation and reauthorization

## 5. Validation

- [x] 5.1 Add device-flow, refresh concurrency, proxy, and credential-security tests
- [x] 5.2 Add gateway routing, header, model, thinking, tool-call, and streaming tests
- [x] 5.3 Add frontend device-flow and account-creation tests
- [x] 5.4 Run focused and full backend tests, frontend type checking/tests, and strict OpenSpec validation
