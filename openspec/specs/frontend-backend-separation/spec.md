# frontend-backend-separation Specification

## Purpose
定义独立前端产物、公共运行时配置、控制面契约与版本兼容边界，兼顾外部静态托管和嵌入式部署。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements
### Requirement: Environment-agnostic frontend artifact

The frontend SHALL build into a standalone static artifact that contains no deployment-specific values, and the same artifact SHALL be usable both by independent static hosting and by the optional embedded packaging.

#### Scenario: One artifact serves both topologies
- **WHEN** the frontend is built once
- **THEN** the resulting artifact can be served by a static web server and can be embedded into the backend binary without rebuilding

### Requirement: Public runtime configuration endpoint

The backend SHALL expose public runtime configuration through a public API endpoint, and the frontend SHALL fetch it at startup. The system MUST NOT rely on server-side HTML injection for configuration delivery.

#### Scenario: Frontend bootstraps in any topology
- **WHEN** the frontend loads in embedded or separated deployment
- **THEN** it retrieves public configuration from the endpoint and renders identically in both topologies

### Requirement: Control-plane API contract

Control-plane APIs (authentication, user, admin, payment) SHALL be described by an OpenAPI specification that is kept in sync with the implementation via CI checks, and the frontend SHALL consume these APIs through a client generated from that specification. Gateway protocol endpoints (OpenAI, Anthropic, and Gemini compatible surfaces) MUST keep upstream protocol semantics and are excluded from contract code generation.

#### Scenario: Contract drift is introduced
- **WHEN** a control-plane endpoint changes without updating the OpenAPI specification
- **THEN** the CI contract check fails before merge

#### Scenario: Gateway endpoints stay protocol-faithful
- **WHEN** a client calls an OpenAI-, Anthropic-, or Gemini-compatible gateway endpoint
- **THEN** request and response semantics follow the upstream protocol unchanged by the contract tooling

### Requirement: Optional embedded packaging with unchanged default deployment

Builds with the embed tag SHALL serve the frontend artifact as plain static files from the single binary. Default deployment and upgrade flows (installation script, single-container compose, admin dashboard self-update, setup wizard) MUST continue to work unchanged.

#### Scenario: Existing single-binary user upgrades
- **WHEN** an existing installation upgrades through the admin dashboard or install script
- **THEN** the embedded single-binary experience, including the setup wizard and web console, works as before without new required components

### Requirement: Separated deployment option

The system SHALL support a first-class separated deployment where the backend runs as a pure API service and the frontend is hosted statically, including an official compose variant, a reference reverse-proxy configuration covering SSE buffering, WebSocket upgrade, and underscore header handling, and a strict CORS allowlist for split-origin setups.

#### Scenario: Operator deploys the separated variant
- **WHEN** an operator uses the separated compose variant with the reference proxy configuration
- **THEN** the web console, control-plane APIs, and gateway SSE and WebSocket traffic function equivalently to the embedded deployment

### Requirement: Frontend-backend version skew guard

The backend SHALL expose its version and minimum compatible frontend version through a public endpoint, and the frontend SHALL verify compatibility at startup in separated deployments and surface a warning on mismatch.

#### Scenario: Version mismatch after partial upgrade
- **WHEN** the deployed frontend version is below the backend's minimum compatible frontend version
- **THEN** the frontend surfaces an explicit incompatibility warning instead of failing silently

### Requirement: Setup wizard parity

The first-run setup wizard SHALL be implemented as frontend views backed by backend setup APIs, and MUST work equivalently in embedded and separated deployments.

#### Scenario: First run in separated deployment
- **WHEN** a fresh installation starts in separated deployment mode
- **THEN** the operator completes database, Redis, and admin account setup through the wizard just as in the embedded deployment

