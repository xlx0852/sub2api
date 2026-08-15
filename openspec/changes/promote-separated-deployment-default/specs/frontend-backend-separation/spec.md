## ADDED Requirements

### Requirement: Component-scoped releases

The deployment workflow SHALL require an explicit `frontend`, `backend`, or `both` scope and SHALL build, upload, switch, and roll back only the selected component. A release that changes a cross-component contract MUST be classified as `both` unless compatibility tests prove the deployed counterpart remains supported.

#### Scenario: Frontend-only change
- **WHEN** an operator deploys a UI-only change whose minimum backend version is satisfied by the live API
- **THEN** only the frontend artifact and frontend current/rollback pointers change
- **AND** the API container, binary, port, resources, database, and API rollback pointers remain unchanged

#### Scenario: Backend-only change
- **WHEN** an operator deploys an API-only change whose minimum frontend version is satisfied by the live console
- **THEN** only the API artifact/container and API current/rollback pointers change
- **AND** the frontend files, symlinks, and frontend rollback pointers remain unchanged

#### Scenario: Coupled contract change
- **WHEN** a release changes OpenAPI/generated-client, runtime-config, authentication, or another incompatible cross-component contract
- **THEN** the deployment workflow rejects a single-component scope and requires a compatibility-ordered `both` deployment

### Requirement: Independent component versions and rollback

The frontend and backend SHALL have independent version sources, immutable artifacts, current pointers, and one retained rollback each. Rolling back one component MUST NOT replace or restart the other component unless the compatibility contract explicitly requires a paired rollback.

#### Scenario: Frontend rollback
- **WHEN** a newly switched frontend has a rendering regression while the API remains healthy and compatible
- **THEN** the operator atomically restores the previous frontend artifact without restarting or replacing the API

#### Scenario: Backend rollback
- **WHEN** a newly switched API violates its health or behavior gate while the frontend remains compatible
- **THEN** the operator restores the previous API container/port without changing the served frontend artifact

## MODIFIED Requirements

### Requirement: Default separated deployment

The system SHALL use a first-class separated same-origin deployment as the default production topology: the backend runs as a pure API service, nginx hosts the standalone frontend artifact, and nginx proxies control-plane, gateway, SSE, and WebSocket paths to an internal or loopback-only API listener. The official compose, production runbook, release artifacts, and rollback metadata MUST support this topology without requiring the API and frontend to be released together.

#### Scenario: Operator uses the default production deployment
- **WHEN** an operator follows the official production compose or runbook
- **THEN** nginx serves the static console and proxies API/gateway traffic to a non-embedded backend
- **AND** the API is not directly exposed as the public frontend server

#### Scenario: Same-origin protocol parity
- **WHEN** clients use the separated default topology
- **THEN** the web console, control-plane APIs, gateway HTTP/SSE, WebSocket upgrades, and underscore headers behave equivalently to the former embedded topology

### Requirement: Compatibility embedded packaging

Builds with the embed tag SHALL continue to serve the frontend artifact as plain static files from a single binary for existing installations and compatibility distribution. Embedded packaging MUST remain testable and upgradeable, but it SHALL NOT be the default production topology or force separated deployments to release both components together.

#### Scenario: Existing embedded installation remains supported
- **WHEN** an existing operator has not opted into the separated migration
- **THEN** the embedded setup wizard and web console continue to work without adding nginx or a separate frontend artifact

#### Scenario: Operator migrates from embed to separated
- **WHEN** an operator follows the documented migration
- **THEN** the old embedded container/config remains a tested rollback until the separated observation window passes

### Requirement: Frontend-backend version skew guard

The backend SHALL expose its version and minimum compatible frontend version through a public endpoint. The frontend artifact SHALL expose its own version and minimum compatible backend version. Frontend startup and deployment preflight MUST evaluate both compatibility directions and surface or reject incompatible partial upgrades instead of failing silently.

#### Scenario: Frontend is too old for backend
- **WHEN** the deployed frontend version is below the candidate backend's minimum frontend version
- **THEN** a backend-only deployment is rejected before traffic switch
- **AND** the running frontend surfaces an explicit incompatibility warning if such skew is observed at runtime

#### Scenario: Backend is too old for frontend
- **WHEN** the live backend version is below the candidate frontend's minimum backend version
- **THEN** a frontend-only deployment is rejected before the frontend symlink switch

## RENAMED Requirements

- FROM: `### Requirement: Optional embedded packaging with unchanged default deployment`
- TO: `### Requirement: Compatibility embedded packaging`
- FROM: `### Requirement: Separated deployment option`
- TO: `### Requirement: Default separated deployment`
