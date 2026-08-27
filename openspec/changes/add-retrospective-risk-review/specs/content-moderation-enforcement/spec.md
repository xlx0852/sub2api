## ADDED Requirements

### Requirement: Risk-weighted retrospective evidence capture

The system SHALL capture a bounded, deterministic, risk-weighted sample of newly supplied client content for retrospective review without archiving every complete conversation.

#### Scenario: A normal user is selected for evidence capture

- **WHEN** a request is selected by the daily deterministic baseline sample and the user has remaining daily item and byte budget
- **THEN** the system stores only the normalized newly supplied client content and attribution metadata as one idempotent evidence item

#### Scenario: A higher-risk user receives increased sampling weight

- **WHEN** configured non-content risk signals increase a user's sampling weight
- **THEN** the system may select more of that user's requests for evidence capture without treating selection itself as a violation or enforcement reason

#### Scenario: Evidence collection is over capacity

- **WHEN** the bounded evidence queue or the user's daily budget is exhausted
- **THEN** the gateway continues serving the request, records a drop/limit metric without content, and does not block or penalize the user

### Requirement: Encrypted and isolated evidence storage

The system MUST store raw retrospective evidence only as compressed application-encrypted objects under the configured dedicated object-store prefix and MUST keep raw content out of PostgreSQL, Redis, application logs, and source-controlled configuration.

#### Scenario: Evidence is written successfully

- **WHEN** a selected evidence item is persisted
- **THEN** the system compresses and encrypts the normalized original new client content before upload, uses a non-guessable object key under `risk-evidence/`, and persists only metadata, hashes, encryption-key version, and object location in PostgreSQL

#### Scenario: Object storage is unavailable

- **WHEN** encryption or object upload fails
- **THEN** the system records a metadata-only failure and operational metric, does not persist plaintext elsewhere, and does not fail the client request

### Requirement: Dedicated least-privilege cloud credentials

The system MUST use project-specific runtime credentials limited to the configured evidence prefix, and MUST NOT use the Alibaba Cloud bootstrap credential at application runtime.

#### Scenario: Runtime credential accesses the evidence prefix

- **WHEN** the application uploads, reads, or deletes an object under `sicts/risk-evidence/*`
- **THEN** the dedicated RAM policy permits only the required object operation

#### Scenario: Runtime credential attempts administrative or cross-scope access

- **WHEN** the dedicated credential attempts to modify Bucket/RAM configuration or access another Bucket or prefix
- **THEN** Alibaba Cloud denies the operation and the negative permission test records no secret value

### Requirement: Evidence retention and deletion

The system SHALL enforce short raw-evidence retention through both application state transitions and object-store lifecycle rules.

#### Scenario: A pending evidence item expires

- **WHEN** an unflagged `pending/` object reaches its configured retention deadline
- **THEN** the raw object is deleted within the lifecycle window while non-content audit metadata may remain

#### Scenario: A finding requires manual review

- **WHEN** an offline review creates a reviewable finding
- **THEN** the system may retain the encrypted object under `flagged/` only until the configured review deadline and then deletes it regardless of case status

### Requirement: Idempotent nightly retrospective review

The system SHALL provide a persistent, single-leader, budget-bounded nightly review job that can resume after interruption without reviewing or charging the same evidence-policy pair twice.

#### Scenario: The nightly job processes eligible evidence

- **WHEN** the scheduled review window begins and the instance holds the job lock
- **THEN** the system claims eligible evidence in stable pages, decrypts it only in memory, creates a context-preserving review copy that removes secret values without deleting URLs, hashes, UUIDs, code, or natural-language intent, invokes the configured secondary review policy without user identity fields, and persists only the result and policy version

#### Scenario: Repeated secrets appear in one evidence item

- **WHEN** the same credential value appears multiple times in one decrypted evidence item
- **THEN** the review copy replaces every occurrence with the same evidence-local typed placeholder while the encrypted original remains recoverable until expiry

#### Scenario: The nightly budget is exhausted

- **WHEN** the configured item, token, or cost budget is reached
- **THEN** the job stops claiming new evidence, preserves unprocessed items for the next allowed run, and reports a non-error budget-limited outcome

#### Scenario: A review dependency fails

- **WHEN** object retrieval, decryption, or the secondary reviewer fails
- **THEN** the system applies bounded retries and records the failure but does not change user status or enforcement state

### Requirement: Persistent user risk profiles

The system SHALL maintain database-backed user risk profiles with level, score/confidence, reason codes, provenance, expiry, and review status, using Redis only as an acceleration cache for real-time enforcement.

#### Scenario: A first high-confidence offline finding is recorded

- **WHEN** a user receives a first high-confidence retrospective finding without an existing escalation condition
- **THEN** the system creates an auditable risk event, moves the user to watch/high monitoring for a bounded period, and forces the existing real-time moderation path to sample that user at 100% without automatically disabling the user

#### Scenario: Repeated or manually confirmed findings escalate

- **WHEN** configured repeated high-confidence findings or administrator confirmation satisfy the escalation policy
- **THEN** the system may invoke the existing tiered API-Key/user enforcement path and records the exact evidence and policy that authorized the action

#### Scenario: A risk profile expires or is cleared

- **WHEN** the profile TTL expires without renewed evidence or an administrator clears the profile
- **THEN** the system removes forced elevated sampling after cache reconciliation and retains an audit event describing the transition

### Requirement: Shared upstream account isolation

Retrospective risk findings MUST affect only attributable users and their API Keys and MUST NOT disable, penalize, or reduce the scheduler health of shared upstream accounts.

#### Scenario: A user is confirmed as high risk

- **WHEN** retrospective review confirms a user's repeated harmful activity
- **THEN** any automated action is limited to that user or attributable API Keys and leaves all shared upstream account state unchanged

### Requirement: Review privacy and operator auditability

The system SHALL minimize review data and audit every operator access to raw evidence and every risk-profile or enforcement mutation.

#### Scenario: An administrator views a case normally

- **WHEN** an administrator opens a retrospective finding without elevated evidence access
- **THEN** the UI returns only a redacted excerpt, scores, reason codes, provenance, and retention deadline

#### Scenario: An administrator requests raw evidence

- **WHEN** an authorized administrator completes the required secondary authorization to view still-retained raw evidence
- **THEN** the system decrypts the evidence for that request only and records administrator, case, reason, time, and outcome in the security audit log
