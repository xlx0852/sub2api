## Context

OpenAI and Grok currently use provider-specific authorization-code sessions stored in process memory. Kimi is introducing a Redis-backed device session, but its read-poll-write sequence needs an atomic lease so concurrent status requests cannot duplicate upstream polls or recreate a cancelled session. CPA demonstrates useful device flows and a common status/cancel contract, while Sub2API already has stronger distributed token refresh and proxy infrastructure.

## Goals / Non-Goals

- Goals:
  - Make OpenAI and Grok login work without a localhost callback
  - Keep PKCE and all token/import login methods available
  - Make active authorization sessions survive instance routing and restarts
  - Keep sensitive device codes and refresh tokens server-side
  - Prevent duplicate polls, replayed completion tickets, and cancel/save races
  - Respect upstream refresh cooldown signals
- Non-Goals:
  - Replace the existing OAuth refresh coordinator
  - Introduce CPA's file-based credential store or plugin system
  - Remove provider-specific token parsing and account enrichment
  - Change upstream inference routing

## Decisions

- Decision: expose additive device-code modes for OpenAI and Grok
  - Authorization-code PKCE remains available as a fallback.
- Decision: store device and authorization-code session state in Redis behind a provider-neutral interface
  - The stored record includes provider, flow, proxy identity, expiry, next poll time, version, and server-side token result.
- Decision: acquire a short Redis polling lease before contacting a device token endpoint
  - Only one instance may poll a session at a time; status readers otherwise receive the current state and retry delay.
- Decision: cancellation writes a short-lived tombstone and versioned updates use compare-and-set
  - A poll that began before cancellation cannot recreate the session or persist credentials.
- Decision: completion returns an opaque one-time ticket
  - Account creation or reauthorization consumes it atomically; OAuth tokens are never returned by a status response.
- Decision: xAI device endpoints come from OIDC discovery
  - Only HTTPS endpoints on `x.ai` or its subdomains are accepted, and the discovered token endpoint is persisted with the account.
- Decision: OpenAI device login feeds its resulting authorization code and PKCE verifier into the existing token exchange and account-enrichment pipeline
  - Token parsing, privacy enrichment, refresh behavior, and credential normalization remain single-sourced.
- Decision: refresh throttling is represented as a typed error with a retry time
  - The shared background refresh service skips attempts until the retry time instead of applying only fixed exponential delays.

## Risks / Trade-offs

- Device endpoints are provider contracts that may change
  - Isolate endpoint parsing, validate response shapes, and keep PKCE fallback modes.
- Redis unavailability can interrupt new login sessions
  - Return an explicit service-unavailable error; never fall back to unsafe process-local device state.
- A generic session abstraction can hide provider differences
  - Share lifecycle operations only; provider clients retain their own protocol and token logic.
- Existing dirty-tree work overlaps account and OAuth files
  - Use additive files and narrow guarded edits without resetting or regenerating unrelated changes.

## Migration Plan

1. Add the shared Redis session repository and move Kimi onto atomic polling operations
2. Add Grok device authorization and account-ticket consumption
3. Add OpenAI Codex device authorization and account-ticket consumption
4. Add shared status/cancel APIs and reusable frontend device flow
5. Add Claude refresh cooldown handling
6. Verify existing PKCE/import flows and multi-instance race scenarios

## Open Questions

- None blocking. Device mode is additive and may be enabled per provider after targeted live-account verification.
