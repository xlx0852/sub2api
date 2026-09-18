## ADDED Requirements

### Requirement: Local control plane
The tray companion MUST expose a loopback-only HTTP control plane authenticated
by a local capability token, providing health, status, setup, model selection,
repair, and model list operations.

#### Scenario: Reject unauthenticated control access
- **WHEN** a client calls a control endpoint without the capability token
- **THEN** the server responds with 401 and performs no configuration changes

#### Scenario: Setup writes managed Codex provider
- **WHEN** a client posts a valid setup payload with base URL and API key
- **THEN** the companion writes a marked `model_providers.sub2api` block and
  sets `model_provider` / `model` for Codex Responses traffic

### Requirement: Safe Codex config mutation
The companion MUST snapshot Codex config before mutation and MUST be able to
repair or restore the managed block without deleting unrelated user settings
when ownership markers still match.

#### Scenario: Repair restores managed block
- **WHEN** the managed provider block is missing but a snapshot exists
- **THEN** repair restores the managed block and returns status OK

### Requirement: Lightweight presentation
The companion MUST provide a system tray or headless control server and MUST
open the settings panel through the system browser using embedded static assets,
without bundling a Chromium runtime.

#### Scenario: Headless serves panel
- **WHEN** the companion starts with headless mode
- **THEN** the control server listens on loopback and serves the embedded panel
  HTML under the capability path
