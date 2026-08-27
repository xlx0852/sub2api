## 1. Specification and routing audit

- [x] 1.1 Confirm existing OpenAI gateway failover requirements and exact stream state boundaries
- [x] 1.2 Define retryable upstream overload/error classification and attempt limits

## 2. Implementation

- [x] 2.1 Track attempted accounts per request and prefer an untried same-group account
- [ ] 2.2 Add short TTL overload cooling with jittered backoff
- [x] 2.3 Preserve no-replay-after-visible-output and billing invariants
- [x] 2.4 Add structured attempt/failure-stage observability

## 3. Verification

- [x] 3.1 Add unit tests for account selection, retry classification, and attempt limits
- [x] 3.2 Verify existing stream tests for recoverable early errors versus terminal post-output failures
- [ ] 3.3 Run targeted Go tests, build, and OpenSpec strict validation

## 4. Rollout preparation

- [x] 4.1 Prepare test-group canary procedure and acceptance metrics
- [x] 4.2 Document rollback and stop conditions
