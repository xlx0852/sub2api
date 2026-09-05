# Change: Ablate redundant account lifecycle stages

## Why

The current provider pipelines include generic admission and select stages that only repeat a nil account check after the handler and selector have already established an account. This adds indirection and stage bookkeeping without changing routing behavior.

## What Changes

- Run a controlled ablation of the no-op `selectAccount` stage across provider pipelines.
- Keep account validation at the first boundary that can return a client-facing error.
- Compare stage order, request/response behavior, lease release, settlement, and latency before and after the ablation.
- Remove the stage only if characterization and full regression tests show no unaccounted behavior difference.

## Impact

- Affected code: `backend/internal/service/*pipeline*.go`, lifecycle stage tests, and pipeline instrumentation.
- Affected capability: provider forwarding lifecycle.
- No protocol, billing, scheduling, or deployment contract changes are intended.
