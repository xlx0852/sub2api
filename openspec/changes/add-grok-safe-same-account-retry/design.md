## Context

`UpstreamFailoverError.RetryableOnSameAccount` currently tells handler loops not to exclude an account before the next iteration, but the next iteration still invokes the scheduler. Grok service error handling can also mark the account temporarily unschedulable before the handler decides to retry. A safe implementation must pin the selected account for the retry attempt and defer account punishment without weakening normal failover.

## Goals / Non-Goals

- Goals:
  - Retry transient Grok Responses and Chat failures once on the exact selected account
  - Retry only before any downstream content is committed
  - Preserve request body, cache identity, model mapping, concurrency accounting, and billing behavior
  - Fall back to the existing account-switch path after retry exhaustion
- Non-Goals:
  - Retry Grok media mutations or WebSocket requests
  - Retry authentication, authorization, policy, validation, or model errors
  - Guarantee that an upstream failed attempt was not charged
  - Add a database-backed retry ledger or client-visible idempotency protocol

## Decisions

- Decision: add a Grok-specific gateway retry configuration
  - The policy is enabled for Grok HTTP Responses and Chat only
  - Defaults are one retry, transient statuses `429/502/503/504/529`, a 500 ms fallback delay, and a bounded `Retry-After`
  - Invalid retry counts, delays, or status codes fail configuration validation

- Decision: pin the selected account in the handler
  - A same-account retry bypasses account selection for exactly the next attempt
  - The retry reuses the account identity and request-scoped Grok cache identity
  - Existing account concurrency acquisition and release remain balanced for every attempt

- Decision: service classifies; handler sequences
  - Grok HTTP forwarding returns `UpstreamFailoverError` with same-account retry metadata for configured transient statuses
  - The handler performs delay, direct retry, then normal account exclusion and failover
  - Grok temporary unscheduling occurs only after the same-account retry budget is exhausted

- Decision: downstream output is the safety boundary
  - A retry is forbidden after the response writer changes, after streaming output starts, or after request cancellation/deadline
  - Stream-internal failures are not broadened by this change unless they already produce a safe pre-output failover error

## Risks / Trade-offs

- A transient response can arrive after upstream work was already charged
  - Mitigation: one retry maximum by default and no media mutation retries
- `Retry-After` can exceed the client deadline
  - Mitigation: cap the delay and abort when the request context cannot accommodate it
- Handler loops can diverge
  - Mitigation: introduce one reusable retry decision/state helper and cover Responses and Chat entry points
- Delayed punishment could leave a bad account eligible briefly
  - Mitigation: pin only the bounded retry attempt, then apply the existing health penalty before failover

## Migration Plan

1. Add configuration and policy tests.
2. Add exact-account retry state and handler tests.
3. Update Grok error classification and defer health punishment.
4. Run focused and full affected-package tests.
5. Deploy through the existing blue-green process; rollback requires only the previous binary.
