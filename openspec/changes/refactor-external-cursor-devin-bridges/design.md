## Context

The current production release contains native Cursor and Devin protocol
clients. Cursor's standalone bridge exposes OpenAI Chat Completions plus
`/api/account`; Devin's standalone bridge exposes Responses/Chat/Anthropic
HTTP endpoints and an optional status panel. Both bridges are single-provider
credential processes, so the current production inventory of one Cursor and
one Devin account can be migrated one-to-one.

## Goals / Non-Goals

- Goals: isolate upstream protocol churn, restore usable quota visibility,
  keep Sub2API billing/scheduling, and preserve a proven rollback path.
- Non-Goals: invent a new provider protocol, expose sidecars publicly, or
  claim OAuth refresh where the upstream project does not provide it.

## Decisions

- Use the local `grok_bot_2api_temp` backup as the Cursor bridge because it
  already implements the required Sand transport and quota endpoint.
- Use `leookun/devin-2api` as the Devin bridge because it implements the
  Devin Connect adapter behind standard HTTP APIs.
- Bind sidecars to a private Docker network/loopback path and put any required
  API-key enforcement at the sidecar or an authenticated reverse-proxy edge.
- Keep Sub2API's generic OpenAI-compatible forwarding as the only gateway
  transport after migration. Provider-specific quota adapters may read JSON
  sidecar endpoints but must not import provider protobuf/client packages.
- Migrate existing records only after sidecar probes succeed; preserve the
  original native account data and current release for rollback.

## Risks / Trade-offs

- Sidecar restarts and per-process credentials become operational concerns;
  mitigate with health checks, restart policy, and one-to-one account markers.
- Cursor bridge authentication is bearer-token based rather than a separate
  API key; network isolation or a reverse proxy is mandatory.
- Devin's status panel is not a stable public quota contract; map only fields
  observed in the deployed bridge and show unknown when unavailable.

## Migration Plan

1. Build and deploy both bridges on unused private ports.
2. Probe health, models, forwarding, and usage without changing live account
   routing.
3. Create external HTTP account configurations and verify billing/scheduling.
4. Switch traffic and quota reads, then observe and compare with native data.
5. Retire native provider code only after rollback markers and sidecar health
   remain stable for the agreed observation window.

## Open Questions

- Whether external bridge accounts should retain `cursor`/`devin` labels as
  metadata or be normalized to `openai` `apikey` records.
- Whether sidecar quota data should be displayed as provider credits, usage
  windows, or both when the upstream omits one of them.
