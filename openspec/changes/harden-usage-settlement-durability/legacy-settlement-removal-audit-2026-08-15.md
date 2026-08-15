# Legacy settlement removal audit — 2026-08-15

## Decision

The legacy non-durable submission path is still a live production path and must not be deleted yet. It is the required billing authority for `off` mode and for groups outside the explicit authoritative scope. Removing it now would contradict the change's preserved default-deployment requirement and would stop billing unselected traffic.

## Current live call chain

1. Gateway and OpenAI handlers build a `RecordUsage` task at 13 non-test submission sites covering Messages/Responses/Chat, Gemini compatibility, Embeddings, Images, Grok media, and WS turns.
2. `GatewayHandler.submitUsageRecordTask` and `OpenAIGatewayHandler.submitUsageRecordTask` execute synchronously only when the request group is durability-enabled; otherwise they submit to `UsageRecordWorkerPool` or use the bounded synchronous fallback.
3. `GatewayService.RecordUsage` and `OpenAIGatewayService.RecordUsage` first call `handoffUsageSettlement`. Only an authoritative selected group returns without invoking the legacy `applyUsageBilling` plus best-effort usage-log path.
4. Wire construction and cleanup still create and stop `UsageRecordWorkerPool` because `off` and unselected groups need it.

The legacy path is therefore not dead code or a compatibility shell. The current selected-group bypass already prevents it from executing for authoritative traffic, while preserving it for every other request.

## Evidence available

| Gate | Evidence | Status |
|---|---|---|
| Deterministic old/new protocol and usage differential | Main path, disconnect, short path, and WS fixtures | Passed |
| Real staging shadow reconciliation | Anthropic/OpenAI/Kimi, six HTTP/SSE requests | Passed |
| Real staging authoritative accounting | Anthropic/OpenAI/Kimi plus OpenAI WS, eight billing identities | Passed |
| Crash recovery | Four SIGKILL boundaries | Passed |
| Shadow-to-authoritative isolation | Six historical shadow rows remained `authoritative=false` and unclaimed | Passed |
| Paired old backend/frontend rollback | Japan staging | Passed |
| Grok successful authoritative request | Staging OAuth credential is revoked | Missing |
| Antigravity successful authoritative request | No staging account is bound | Missing |
| Gemini successful authoritative request | Staging path is retired | Missing |
| Production observation across all active groups and entry paths | Production was not accessed | Missing |

## Preconditions before deletion

All of the following are required:

1. Every production provider and billable entry path has a successful authoritative canary, including JSON/SSE/WS or media forms that the path supports.
2. All active production groups have been promoted deliberately; no real traffic depends on `off` or an unselected-group fallback.
3. During the agreed observation window, every selected identity satisfies `settled event = usage_logs = billing dedup = balance/quota delta`, with zero unexplained retryable, terminal-failure, or reconciliation backlog.
4. Historical shadow rows remain `authoritative=false`; no operator has bulk-promoted them.
5. The paired old backend/frontend rollback remains available until the observation window ends.
6. The project explicitly resolves the post-removal semantics of `off` and unselected groups. Keeping `off` as the default while deleting its only billing path is not valid; changing that default is outside the current behavior-preserving promise.

Until these conditions are met, task 6.5 must stay unchecked and the worker-pool construction, handler fallback, and both legacy `RecordUsage` commit branches must remain.
