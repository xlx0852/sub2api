## 1. Shared authorization sessions

- [x] 1.1 Add a provider-neutral Redis OAuth session model and repository
- [x] 1.2 Add atomic poll lease, versioned update, cancellation tombstone, and one-time consume operations
- [x] 1.3 Move Kimi device polling onto the atomic lifecycle and cover cancel/poll races
- [x] 1.4 Add shared authenticated status and cancellation endpoints

## 2. Grok device authorization

- [x] 2.1 Add xAI OIDC discovery with strict HTTPS and host validation
- [x] 2.2 Add proxy-aware device start, poll, slow-down, denial, expiry, and refresh endpoint persistence
- [x] 2.3 Add Grok device login tickets and account creation/reauthorization consumption
- [x] 2.4 Preserve existing Grok PKCE and refresh-token workflows

## 3. OpenAI device authorization

- [x] 3.1 Add Codex device user-code request and polling with numeric/string interval support
- [x] 3.2 Exchange the device authorization result through the existing OpenAI token pipeline
- [x] 3.3 Add OpenAI device login tickets and account creation/reauthorization consumption
- [x] 3.4 Preserve PKCE, refresh-token, mobile token, PAT, and Codex session import workflows

## 4. Refresh hardening

- [x] 4.1 Add typed OAuth refresh errors carrying retry time
- [x] 4.2 Honor Claude `Retry-After` and `Retry-After-Ms` across instances
- [x] 4.3 Ensure cancellation of one request does not corrupt a shared refresh operation

## 5. Administrator UI

- [x] 5.1 Add a reusable device authorization component with code, countdown, polling, cancellation, and retry
- [x] 5.2 Add OpenAI and Grok login-mode selection with device mode recommended
- [x] 5.3 Reuse device authorization for account creation and reauthorization
- [x] 5.4 Keep every existing login and import option visible and functional

## 6. Validation

- [x] 6.1 Cover Redis multi-instance polling, cancellation, expiry, and replay races
- [x] 6.2 Cover Grok OIDC validation and all device-code states
- [x] 6.3 Cover OpenAI device intervals, pending responses, timeout, proxy, and token exchange
- [x] 6.4 Cover Claude refresh cooldown parsing and enforcement
- [ ] 6.5 Run focused backend tests, frontend type checking/tests, full relevant service tests, and strict OpenSpec validation (focused checks and strict validation pass; the full unit service suite still has unrelated baseline pricing failures)
