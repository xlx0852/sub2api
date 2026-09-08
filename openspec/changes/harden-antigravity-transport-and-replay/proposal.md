# Change: Harden Antigravity Gemini transport, fingerprint, BaseURL, and reasoning replay

## Why
Antigravity (Google Cloud Code) upstream endpoints enforce strict TLS/HTTP transport fingerprints, client version floors, and multi-turn thought signature integrity.
1. `sub2api` currently connects with standard Go HTTP/2, which deviates from the official Antigravity client's HTTP/1.1 fingerprint (no ALPN, no HTTP/2) and lacks per-account connection pool isolation, exposing accounts to Google anti-abuse risk.
2. The default User-Agent version is outdated (`1.23.2` on Windows), and the upstream endpoints omit `https://daily-cloudcode-pa.googleapis.com`, where newer Gemini models are primarily released.
3. In multi-turn tool calling (e.g. Cline, Cursor), clients often strip or omit Google's `thoughtSignature`. Without a reasoning replay cache, requests either fail with upstream 400 errors or lose thinking continuity.

## What Changes
- **TLS & HTTP/1.1 Transport Hardening (P0)**:
  - Add `HTTPUpstreamProfileAntigravity` to upstream HTTP profile definitions.
  - Enforce HTTP/1.1 (`ForceAttemptHTTP2 = false`, empty `TLSNextProto`, `NextProtos = nil`), disabling ALPN / HTTP/2 to match the native Antigravity client wire fingerprint.
  - Enforce account-level connection pool isolation (`account:<accountID>` or `account:<accountID>|proxy:<proxyKey>`) so different Antigravity accounts never share TLS connections.
  - Expand connection pool reuse limits (`MaxIdleConnsPerHost = 100`, `IdleConnTimeout = 10m`) matching native client behavior.
  - Apply HTTP/1.1 transport to all Antigravity OAuth, onboarding, and quota client calls in `pkg/antigravity`.
- **BaseURL & User-Agent Modernization (P1)**:
  - Add `https://daily-cloudcode-pa.googleapis.com` as the primary endpoint in `ForwardBaseURLs`, keeping prod and sandbox as fallbacks.
  - Upgrade default UA version to `>= 2.9.1` and align format to `antigravity/hub/<version> darwin/arm64`.
  - Attach `google-api-nodejs-client/10.3.0` and `X-Goog-Api-Client: gl-node/22.21.1` on onboarding/control plane requests.
- **Reasoning Replay & Thought Signature Cache (P2)**:
  - Implement a session-scoped reasoning replay cache for Antigravity Gemini requests.
  - Record generated `thoughtSignature` from streaming responses indexed by session and tool call / turn.
  - In inbound requests, detect missing `thoughtSignature` on model parts or `functionCall` items and automatically restore them from cache, preventing upstream 400 `thought signature missing` rejections.
  - Fall back to `skip_thought_signature_validator` only when no cached signature exists.
- **Scope Restriction**:
  - Keep Claude models excluded (strict Gemini-only scope preserved as per `remove-antigravity-non-gemini`).

## Impact
- Affected capability: `antigravity`
- Affected components:
  - `backend/internal/domain/http_upstream_profile.go`
  - `backend/internal/repository/http_upstream.go`
  - `backend/internal/pkg/antigravity/oauth.go`
  - `backend/internal/pkg/antigravity/client.go`
  - `backend/internal/service/antigravity_pipeline_gemini.go`
  - `backend/internal/service/antigravity_gateway_service.go`
  - `backend/internal/service/antigravity_gateway_streaming.go`
  - `backend/internal/service/antigravity_reasoning_replay.go` (new)
