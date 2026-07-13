## 1. Backend

- [x] 1.1 Add tenant-isolated Grok cache identity derivation and request/header application helpers.
- [x] 1.2 Apply cache identity consistently to Grok Responses, Messages, retry, failover, and WebSocket HTTP bridge paths.
- [x] 1.3 Keep compact, media, account-probe, and auxiliary image-description requests outside conversation caching.
- [x] 1.4 Add strict Chat Completions eligibility checks and route eligible Grok 4.5 requests through Responses.
- [x] 1.5 Preserve existing Grok API-key, CLI proxy, custom gateway, media, and native WebSocket behavior while integrating upstream changes.
- [x] 1.6 Preserve cache-read usage through sync, stream, Chat, Messages, and Responses conversions.

## 2. Validation

- [x] 2.1 Add tests for tenant isolation, explicit and content-derived seeds, missing-context fail-closed behavior, and failover identity stability.
- [x] 2.2 Add tests for Free OAuth tool-free augmentation and explicit tool-intent preservation.
- [x] 2.3 Add tests for eligible Chat-to-Responses routing and incompatible raw-Chat fallback.
- [x] 2.4 Add tests for Messages, WebSocket HTTP bridge, account probes, cached-token usage, and current Grok routing compatibility.
- [x] 2.5 Run targeted Grok, handler, Messages, WebSocket, and server-entry test suites.
- [x] 2.6 Validate this OpenSpec change in strict mode.
