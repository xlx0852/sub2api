## MODIFIED Requirements
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
