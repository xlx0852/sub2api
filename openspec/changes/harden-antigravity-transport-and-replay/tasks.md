## 1. Transport & Wire Fingerprint Hardening (P0)
- [x] 1.1 Add `HTTPUpstreamProfileAntigravity` to `backend/internal/domain/http_upstream_profile.go`.
- [x] 1.2 Implement `upstreamProtocolModeAntigravityH1` in `backend/internal/repository/http_upstream.go`: disable HTTP/2, clear `TLSNextProto`, strip ALPN `NextProtos`, and enforce credential/account connection pool isolation.
- [x] 1.3 Apply widened connection pool settings (`maxIdleConnsPerHost >= 100`, `idleConnTimeout = 10m`) for Antigravity upstream requests.
- [x] 1.4 Propagate `HTTPUpstreamProfileAntigravity` in `backend/internal/service/antigravity_pipeline_gemini.go` and `antigravity_gateway_service.go`.
- [x] 1.5 Update `backend/internal/pkg/antigravity/client.go` to enforce HTTP/1.1 transport on OAuth token exchange, onboarding, and quota discovery calls.

## 2. BaseURL & User-Agent Modernization (P1)
- [x] 2.1 Update `backend/internal/pkg/antigravity/oauth.go`: add `antigravityDailyBaseURL = "https://daily-cloudcode-pa.googleapis.com"` and make it the primary endpoint in `BaseURLs` / `ForwardBaseURLs()`.
- [x] 2.2 Bump `DefaultUserAgentVersion` to `"2.9.1"` and update `BuildUserAgent` format to `antigravity/hub/<version> darwin/arm64`.
- [x] 2.3 Attach node client and goog-api headers for onboarding / control plane requests in `backend/internal/pkg/antigravity/client.go`.

## 3. Reasoning Replay & Thought Signature Cache (P2)
- [x] 3.1 Implement `AntigravityReasoningReplayCache` in `backend/internal/service/antigravity_reasoning_replay.go` (session-indexed storage for thought signatures).
- [x] 3.2 Capture and record upstream thought signatures from streaming chunks in `backend/internal/service/antigravity_gateway_streaming.go`.
- [x] 3.3 Inspect inbound Gemini turns in `forwardGeminiRewrite` / `antigravity_pipeline_gemini.go`: restore missing `thoughtSignature` from cache for function calls or model parts before sending to upstream.
- [x] 3.4 Fallback to `skip_thought_signature_validator` only when no cached signature is available.

## 4. Verification & Testing
- [x] 4.1 Add unit tests for HTTP/1.1 transport configuration, ALPN suppression, and pool isolation in `http_upstream_test.go`.
- [x] 4.2 Add unit tests for BaseURL ordering and modern User-Agent generation in `oauth_test.go`.
- [x] 4.3 Add unit tests for Reasoning Replay capture, cache hit, and signature injection in `antigravity_reasoning_replay_test.go`.
- [x] 4.4 Run all targeted Antigravity package and service tests to ensure zero regressions.
