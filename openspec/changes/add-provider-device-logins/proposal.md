# Change: Add Provider Device Logins

## Why

Sub2API's hosted administrator UI relies on authorization-code callbacks for OpenAI and Grok. Those flows are fragile when the browser and gateway run on different machines, and each provider currently owns an in-memory OAuth session implementation. Device authorization and a shared Redis session lifecycle make remote login reliable while preserving every existing login option.

## What Changes

- Add OpenAI Codex device-code login alongside the existing PKCE, token, PAT, and session-import modes
- Add xAI/Grok OIDC-discovered device authorization alongside the existing PKCE flow
- Add a shared Redis-backed provider OAuth session lifecycle with polling, cancellation, one-time consumption, and cross-instance safety
- Add atomic polling leases and cancellation protection for Kimi and other device flows
- Honor provider token-refresh `Retry-After` signals, beginning with Claude
- Add reusable administrator UI for device authorization and reauthorization
- Preserve all existing authorization-code and credential-import workflows

## Impact

- Affected specs: `provider-device-authorization`, `oauth-refresh-backoff`
- Related change: `add-kimi-oauth-accounts`
- Affected code: OAuth clients and services, Redis repositories, admin routes, account creation/reauthorization UI, token refresh policy
- No database schema change
- No removal or behavioral replacement of existing login methods
