## Context

Provider pipelines now expose explicit lifecycle stages, but the generic select stage currently performs no selection. Selection happens before forwarding when the handler supplies the account.

## Goals / Non-Goals

- Goals: measure whether the no-op stage can be removed, reduce unnecessary indirection, preserve observable behavior.
- Non-Goals: redesign account scheduling, change WebSocket lifecycle, or remove protocol-specific pipeline stages.

## Decisions

- Use existing pipeline stage-order tests and provider forwarding tests as the baseline.
- Add a focused characterization test for account admission and lease/settlement invariants before changing stage composition.
- Treat any response, usage, lease, retry, or error difference as a failed ablation.

## Risks / Trade-offs

- Removing a stage may hide a future extension point. Mitigate by retaining the explicit admission boundary and documenting the decision.
- Stage metrics may change cardinality. Compare instrumentation output before and after.

## Migration Plan

1. Capture baseline tests and stage names.
2. Remove only the proven no-op select stage.
3. Run backend and frontend gates.
4. Revert the ablation if behavior or observability differs without an explicit reason.
