## ADDED Requirements

### Requirement: Account-scoped Codex fingerprint policy

The system SHALL expose `off`, `device`, `session`, and `full` convergence policies for OpenAI OAuth accounts. Existing accounts without an explicit policy MUST remain `off`, while newly created OAuth accounts SHALL default to `session` in the admin form.

#### Scenario: Existing account has no policy
- **WHEN** an existing OAuth account has no `codex_fingerprint_mode` value
- **THEN** the gateway passes its Codex identity fields without convergence

#### Scenario: New OAuth account is created
- **WHEN** an administrator creates a new OpenAI OAuth account without changing the fingerprint selector
- **THEN** the account is saved with `codex_fingerprint_mode=session`

### Requirement: Consistent identity projection

For each upstream request, the system MUST compute convergence identifiers once and apply the same values to HTTP headers, Responses `client_metadata`, and embedded turn metadata.

#### Scenario: Session convergence request
- **WHEN** a request uses `session` convergence
- **THEN** installation and session identifiers are account-stable, thread identifiers are deterministically derived from the original client session, and all carriers contain matching values

#### Scenario: Full convergence request
- **WHEN** a request uses `full` convergence
- **THEN** installation, session, thread, root, parent, fork, and window identity fields form one account-scoped thread graph while each turn retains a unique turn identifier

### Requirement: Transport parity

The system MUST apply the selected policy consistently to standard Responses HTTP, OAuth passthrough HTTP, and Responses WebSocket v2 traffic, and MUST NOT apply it to API-key accounts or compact requests.

#### Scenario: OAuth WebSocket request
- **WHEN** an OAuth Responses WebSocket v2 turn is forwarded with convergence enabled
- **THEN** its first frame, replayed frame, and upstream handshake metadata expose the same converged identity

#### Scenario: Non-OAuth request
- **WHEN** an API-key account forwards a Responses request
- **THEN** the convergence layer makes no identity changes

### Requirement: Preserve non-identity metadata

The convergence layer SHALL preserve unknown metadata and known non-identity fields such as sandbox mode, auto-review state, tool inventory, and routing hints.

#### Scenario: Metadata contains new non-identity fields
- **WHEN** embedded turn metadata contains fields not owned by the convergence layer
- **THEN** those fields remain byte-semantically equivalent after identity rewriting
