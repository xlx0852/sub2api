## ADDED Requirements

### Requirement: Session identity rotation on overload retry

The OpenAI gateway SHALL rotate the derived Codex `thread-id` both within a request's same-account retries and across subsequent requests that reuse the same client session, after a retryable upstream overload is observed on that account. Rotation MUST be bounded to an overload burst window and MUST return to the deterministic derivation once the window elapses without a further overload. The account-level converged `session_id` MUST NOT be rotated.

#### Scenario: Same-account retry carries a fresh thread-id

- **WHEN** an eligible OpenAI OAuth account returns `server_is_overloaded` and the gateway retries on the same account
- **THEN** the retry request carries a `thread-id` derived with the attempt index included, differing from the failed attempt's `thread-id`
- **AND** the `session-id` on the retry equals the account-level converged value used by the failed attempt

#### Scenario: Next request from the same client session also rotates

- **WHEN** an account reports `server_is_overloaded` for a request whose client session identity is known, and that client later issues a further request under the same client session identity within the burst window
- **THEN** the further request carries a `thread-id` derived with the overload epoch included, differing from the derivation used before the overload, even though the further request starts with attempt index zero

#### Scenario: Rotation expires with the burst window

- **WHEN** the burst window elapses with no further overload for that account and client session
- **THEN** subsequent requests from that client session derive the deterministic `thread-id` again

#### Scenario: The first attempt of an unaffected session is unchanged

- **WHEN** a client session has no recorded overload for the selected account
- **THEN** the `thread-id` is the deterministic derivation of account identity and client session, with no attempt or epoch component

#### Scenario: Rotation is observable

- **WHEN** a retry or a subsequent request rotates the derived `thread-id`
- **THEN** the gateway records a structured field indicating rotation occurred, without logging credentials or request content

### Requirement: Immediate turn-state discard on overload

The OpenAI gateway SHALL discard the pinned `x-codex-turn-state` for the affected account and pool on the first `server_is_overloaded` outcome, and MUST NOT require a run of consecutive 5xx responses before discarding. The discard MUST also apply when the overload cannot be replayed because visible output was already sent to the client.

#### Scenario: First overload drops the pinned ticket

- **WHEN** an upstream response reports `server_is_overloaded` for an account/pool that has a pinned turn-state
- **THEN** the gateway discards that pinned turn-state so the next attempt re-draws pool routing
- **AND** the discard is recorded as a structured event

#### Scenario: Unreplayable stream overload still drops the ticket

- **WHEN** a stream fails with `server_is_overloaded` after visible content has already reached the client, so the request cannot be replayed
- **THEN** the gateway still discards the pinned turn-state and records the burst, without replaying the request

#### Scenario: Existing backoff is not an obstacle

- **WHEN** a turn-state was discarded recently and another `server_is_overloaded` arrives inside the discard cooldown
- **THEN** the gateway still allows the discard for the overload case rather than suppressing it

### Requirement: In-stream overload shares the recovery path

The OpenAI gateway SHALL classify an HTTP 200 response whose SSE stream terminates with a `response.failed` event carrying `error.code == "server_is_overloaded"` as the same recoverable overload condition as an HTTP 503 from the upstream, subject to the existing no-replay-after-visible-output boundary.

#### Scenario: Stream-level overload before visible output

- **WHEN** the upstream returns HTTP 200 and the stream fails with `server_is_overloaded` before any visible content reached the client
- **THEN** the gateway applies the same rotation and account-failover decisions it applies to an HTTP 503 overload

#### Scenario: Stream-level overload after visible output

- **WHEN** the stream fails with `server_is_overloaded` after visible content has been sent to the client
- **THEN** the gateway does not replay the request and records the terminal failure stage

### Requirement: Overload burst retry budget

The OpenAI gateway SHALL raise the retry and rotation budget for an account group for a bounded window after an overload is observed on that group, and SHALL reset the raised budget when the window elapses without a further overload.

#### Scenario: Budget is raised inside the burst window

- **WHEN** an account group records a `server_is_overloaded` outcome
- **THEN** subsequent requests on that group within the configured burst window receive the raised retry and rotation budget

#### Scenario: Budget returns to baseline

- **WHEN** the burst window elapses with no further overload on the group
- **THEN** subsequent requests use the baseline retry and rotation budget

### Requirement: Overload recovery observability

The OpenAI gateway SHALL record, for each overload recovery decision, the delivery path, the attempt index, whether session identity rotation occurred, whether pinned turn-state was discarded, and the final client outcome, and MUST NOT log credentials or request content.

#### Scenario: Recovery decision is recorded

- **WHEN** the gateway makes a recovery decision for a `server_is_overloaded` outcome
- **THEN** it records the overload delivery path as one of the HTTP-503 or stream-failed variants, the attempt index, and both rotation flags
- **AND** no credential material or prompt content appears in the record
