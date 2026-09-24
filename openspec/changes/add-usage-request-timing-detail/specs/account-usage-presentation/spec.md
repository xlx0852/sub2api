## ADDED Requirements

### Requirement: Request timing details for usage records
The usage record interface SHALL allow a user with access to a usage record to open a request timing detail view showing the recorded request path and timing data without exposing request bodies or credentials.

#### Scenario: A usage record contains timing stages
- **WHEN** the user opens a usage record that has `duration_ms`, `first_token_ms`, or `first_output_stages`
- **THEN** the detail view SHALL show the request model, request ID, creation time, inbound/upstream paths, first-token latency, total duration, and every available recorded stage
- **AND** the detail view SHALL identify cache/token and model-mapping summary data when present

#### Scenario: Historical usage record lacks stage data
- **WHEN** the user opens a historical usage record whose optional timing stage fields are missing
- **THEN** the detail view SHALL show `-` for unavailable stages and SHALL NOT infer or fabricate missing durations

#### Scenario: User opens timing details from the usage table
- **WHEN** the user activates the timing-detail action on a usage row
- **THEN** a modal SHALL open without navigating away from the usage page
- **AND** closing the modal SHALL preserve the current filters, page, and scroll context

#### Scenario: Timing details are displayed for different request paths
- **WHEN** the usage record uses `/v1/messages`, `/v1/responses`, `/v1/chat/completions`, or another recorded endpoint
- **THEN** the detail view SHALL display the recorded inbound path and upstream path independently
