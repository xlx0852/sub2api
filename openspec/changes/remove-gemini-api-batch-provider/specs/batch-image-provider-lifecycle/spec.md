## ADDED Requirements

### Requirement: Gemini API Batch Image Provider Is Retired

The system MUST NOT advertise, automatically select, or submit new batch image
jobs through provider `gemini_api`. An explicit request for `gemini_api` SHALL
return the existing unsupported-provider error and MUST NOT silently route the
job to Vertex or any other provider.

#### Scenario: Explicit Gemini API provider is rejected

- **GIVEN** a batch image request explicitly names provider `gemini_api`
- **WHEN** provider validation or selection runs
- **THEN** the request is rejected as an unsupported provider
- **AND** no upstream Gemini Batch API request is made
- **AND** the request is not silently submitted to Vertex

### Requirement: No remaining Google batch-image provider

After Vertex Service Account supply retirement, the system MUST NOT keep a
Vertex batch-image Provider, worker, queue, settlement, download, or cleanup
runtime. Group schema columns and generic `batch_image.*` configuration keys
MAY remain to avoid a schema migration.

#### Scenario: Vertex batch provider is not available

- **GIVEN** Vertex Service Account supply has been retired
- **WHEN** a batch image job would select provider `vertex`
- **THEN** no Vertex batch Provider, worker, or GCS/Batch Prediction client is assembled
- **AND** the request is not submitted upstream

### Requirement: Retirement Requires An Empty Financial Lifecycle Gate

The system SHALL require a pre-deployment gate before a build removes Gemini API
batch recovery code. The gate MUST verify that every target environment has no
non-terminal `gemini_api` job, no unsettled hold belonging to such a job, and no
pending output cleanup that requires the Gemini provider.

If any gate is non-zero, deployment MUST stop. Operators MUST use the existing
worker, settlement, cancellation, and cleanup paths to reach a safe terminal
state; they MUST NOT bypass financial lifecycle behavior by directly rewriting
job status in SQL.

#### Scenario: Non-terminal Gemini job blocks deployment

- **GIVEN** a target environment contains a non-terminal `gemini_api` batch image job
- **WHEN** the pre-deployment retirement gate runs
- **THEN** deployment of the provider-removal build is blocked
- **AND** no migration silently marks the job terminal or releases its hold outside the existing billing path

#### Scenario: Empty environment passes the retirement gate

- **GIVEN** the target environment has no Gemini API batch jobs, unsettled holds, or provider-dependent pending cleanup
- **WHEN** the pre-deployment retirement gate runs
- **THEN** the environment is eligible for the provider-removal rollout
