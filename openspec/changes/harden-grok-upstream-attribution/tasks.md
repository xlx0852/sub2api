## 1. Explicit Retry Suppression

- [x] 1.1 Add strict `x-should-retry: false` parsing and unit tests
- [x] 1.2 Suppress same-account retry, account failover, and account penalties across Grok Responses and Chat paths
- [x] 1.3 Add handler-level regression coverage with multiple available accounts

## 2. Request Identity

- [x] 2.1 Add server-derived Grok request/session/attempt identity helpers
- [x] 2.2 Inject identity headers into Responses, native Chat, and Chat-via-Responses after all untrusted overrides
- [x] 2.3 Add tenant-isolation, cross-protocol, and monotonic-attempt tests

## 3. Token Attribution

- [x] 3.1 Return non-secret Grok credential version/fingerprint/source metadata for the token actually sent
- [x] 3.2 Classify Grok OAuth 401 as stale, current, or unknown against the credential owner
- [x] 3.3 Suppress stale-token account penalties and emit sanitized structured attribution
- [x] 3.4 Add token rotation, shadow-account, fallback, and secret-redaction tests

## 4. Validation

- [x] 4.1 Run formatting and focused normal tests
- [x] 4.2 Run focused race tests and full affected package suites
- [x] 4.3 Run `go vet`, server build, `git diff --check`, and strict OpenSpec validation
