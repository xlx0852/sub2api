## ADDED Requirements

### Requirement: Anonymous pricing catalog
The system SHALL provide a pricing catalog that can be viewed without authentication and SHALL expose only public model and effective price fields.

#### Scenario: Anonymous visitor views pricing
- **WHEN** an unauthenticated visitor opens the public home page
- **THEN** the visitor can search and filter public models and see their effective public prices
- **AND** the visitor is not redirected to login

#### Scenario: Private commercial data is excluded
- **WHEN** the public snapshot is generated
- **THEN** exclusive groups, user-specific rates, channel names, group identifiers, account data and upstream configuration are omitted

### Requirement: Public price selection
The system SHALL compute each model's displayed public price only from active public groups and SHALL select the lowest applicable effective public price.

#### Scenario: Public and exclusive offers coexist
- **WHEN** a model is linked to both public and exclusive groups
- **THEN** only public active group offers participate in the anonymous displayed price

### Requirement: Cached anonymous read path
The system SHALL serve anonymous pricing requests from a long-lived snapshot and SHALL prevent concurrent anonymous requests from multiplying database or upstream work.

#### Scenario: Snapshot is fresh
- **WHEN** an anonymous request arrives while an L1 snapshot is fresh
- **THEN** the response is served without Redis, database or upstream access

#### Scenario: Snapshot is stale
- **WHEN** an anonymous request arrives while a prior snapshot exists but is stale
- **THEN** the prior snapshot is returned immediately
- **AND** at most one background rebuild is started per cache generation

#### Scenario: Rebuild fails
- **WHEN** snapshot rebuilding fails and a prior successful snapshot exists
- **THEN** the system continues serving the prior snapshot

#### Scenario: Cold cache concurrency
- **WHEN** concurrent anonymous requests arrive before any snapshot exists
- **THEN** at most one snapshot generation reaches the database
- **AND** other requests do not independently query the database or upstream

### Requirement: HTTP and edge cache protection
The system SHALL emit cache validators and shared-cache directives and SHALL apply independent anonymous request limits.

#### Scenario: Client has current ETag
- **WHEN** `If-None-Match` matches the current public snapshot ETag
- **THEN** the endpoint returns `304 Not Modified` without a response body

#### Scenario: Anonymous request burst
- **WHEN** an IP exceeds the configured public pricing request budget
- **THEN** the endpoint returns `429 Too Many Requests`
- **AND** no snapshot regeneration or database query is triggered by the rejected request

### Requirement: Authenticated catalog compatibility
The system SHALL preserve the existing authenticated model plaza semantics.

#### Scenario: Logged-in user views model plaza
- **WHEN** an authenticated user opens the existing model plaza
- **THEN** the user continues to see groups, channels and rates authorized for that user
- **AND** the public snapshot does not replace the authenticated response
