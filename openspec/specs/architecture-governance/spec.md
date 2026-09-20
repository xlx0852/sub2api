# architecture-governance Specification

## Purpose
约束后端依赖方向、统一网关与调度结算边界及前端模块职责，防止平台迁移形成重复主体和隐式副作用。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Layered dependency direction

The backend SHALL be organized into `domain`, `ports`, `app`, `infra`, and `transport` layers with inward-only dependencies: `domain` MUST NOT import any other internal layer, `ports` MAY import only `domain`, `app` MAY import `domain`, `ports`, and stateless `pkg` libraries, `infra` MAY import `domain`, `ports`, `ent`, and `pkg` but MUST NOT import `app`, and `transport` MAY import `app` but MUST NOT be imported by it.

#### Scenario: Repository implements an application-defined interface
- **WHEN** a persistence or cache implementation is provided for an interface consumed by application code
- **THEN** the interface is declared in `ports` and the implementation compiles without importing any `app` package

#### Scenario: Dependency violation is introduced
- **WHEN** a change adds an import that violates the layer rules (for example `infra` importing `app`)
- **THEN** the CI architecture check fails before merge

### Requirement: Unified gateway pipeline

All upstream platform traffic SHALL flow through a single gateway pipeline with explicit lifecycle stages (parse, admission, selection, rewrite, transport, relay, settlement). Provider-specific protocol conversion SHALL live in per-provider adapters, and after a platform completes migration it MUST NOT retain a parallel legacy forwarding path. Retired platforms or offline features MUST NOT retain background workers or parallel forwarding services in the application runtime.

#### Scenario: Platform migration completes
- **WHEN** a platform finishes migrating into the unified pipeline
- **THEN** its legacy forwarding path is removed in the same change and all its traffic flows through the pipeline stages

#### Scenario: A new upstream platform is added
- **WHEN** a new upstream platform is integrated
- **THEN** it is implemented as a provider adapter plus stage implementations, reusing shared scheduling, billing, stream relay, and settlement code without duplicating them

#### Scenario: Cross-cutting rewrite concerns stay single-sourced
- **WHEN** fingerprint, model mapping, or parameter override behavior changes
- **THEN** the change is made once in the rewrite stage middleware and applies to HTTP, passthrough, and WebSocket transports alike

#### Scenario: Retired features do not run background workers
- **WHEN** a gateway capability or supplier (such as batch image or native Gemini direct) is retired or routes are removed
- **THEN** its background worker loops, queue polling routines, and parallel compat services are removed from application assembly

### Requirement: Single account scheduler

The system SHALL provide one `AccountSelector` entry point for all platforms, composed of policy plugins (priority, load awareness, sticky sessions, capability filtering, quota pause and cooldown, mixed scheduling). Platform-specific behavior MUST be expressed as policies rather than separate scheduler implementations.

#### Scenario: Scheduling behavior parity after unification
- **WHEN** the two legacy schedulers are merged behind the unified interface
- **THEN** existing sticky-session, capability, and cooldown semantics are preserved for every platform as verified by regression tests

### Requirement: Single usage settlement path

The system SHALL record usage and billing through one settlement service with a unified usage model. Provider adapters MUST map protocol-specific usage events into that model instead of implementing their own settlement.

#### Scenario: Usage recorded for any platform
- **WHEN** a forwarded request completes on any platform
- **THEN** billing, rate-limit feedback, and session-window updates are performed by the shared settlement service with unchanged billing semantics

### Requirement: Injection-only assembly

Application components SHALL be wired exclusively through dependency injection with module-level provider sets and a lifecycle registry for start and stop hooks. Package-level mutable globals and setter-based wiring MUST NOT be used.

#### Scenario: A component needs a collaborator after refactor
- **WHEN** a service requires a collaborator such as the web search manager or idempotency coordinator
- **THEN** the collaborator is passed through its constructor and no package-level setter or mutable global is introduced

#### Scenario: A background service registers lifecycle hooks
- **WHEN** a module provides a background service with start and stop behavior
- **THEN** it registers with the lifecycle registry and shutdown runs in reverse registration order without manual cleanup lists

### Requirement: Frontend feature modularity

The frontend SHALL organize code into feature modules with colocated views, components, API bindings, and stores, and SHALL define platform-specific account form fields in a single schema registry consumed by the create, edit, and bulk-edit account forms.

#### Scenario: Account platform fields change
- **WHEN** a platform adds or changes an account form field
- **THEN** the change is made once in the platform field registry and all account modals render it consistently

#### Scenario: Settings sections load independently
- **WHEN** a user opens the admin settings area
- **THEN** each settings section is a lazily loaded sub-route component rather than one monolithic view

### Requirement: Scheduling primitive extraction preserves selection semantics

The system SHALL share behavior-equivalent scheduling primitives across Gateway and OpenAI without changing either Strategy's candidate thresholds, sticky-session policy, scoring, cooldown, quota pause, failure punishment, or credential-class behavior.

#### Scenario: A common primitive replaces duplicate code

- **WHEN** channel restriction, account listing, sorting, shuffling, or sticky candidate validation is migrated to a shared primitive
- **THEN** the old and shared implementations produce the same candidate set, ordering groups, exclusion result, and sticky decision for the same inputs

#### Scenario: Strategies differ after the common boundary

- **WHEN** Gateway requires session registration or OpenAI requires sticky epoch, quota re-evaluation, escape, runtime blocking, or transport compatibility checks
- **THEN** those checks remain in their existing Strategy and execute in the same order as before the extraction

#### Scenario: Advanced scheduler behavior is active

- **WHEN** OpenAI advanced scheduling, sticky weighted selection, lowest-usage selection, or TopK exploration is enabled
- **THEN** this refactor does not change its score inputs, weights, candidate gates, exploration set, or selected account

### Requirement: Removed quota auto-pause controls have no enforcement path
The system SHALL remove the OpenAI 5h and 7d quota auto-pause controls and enforcement path. Historical per-account metadata MAY remain stored but MUST NOT affect account selection, sticky routing, or schedulability. Expiry auto-pause, provider cooldowns, and Grok quota behavior SHALL remain independent.

#### Scenario: OpenAI account exceeds historical 5h or 7d threshold
- **WHEN** an OpenAI account contains historical `auto_pause_5h_threshold` or `auto_pause_7d_threshold` metadata and its usage exceeds that value
- **THEN** OpenAI scheduling does not skip the account because of that metadata

#### Scenario: Admin opens Ops settings
- **WHEN** an administrator views advanced Ops settings
- **THEN** no 5h/7d OpenAI quota auto-pause controls or validation fields are rendered or accepted

#### Scenario: Expired account is evaluated
- **WHEN** an account reaches its configured expiration and `auto_pause_on_expired` is enabled
- **THEN** expiry handling continues unchanged
