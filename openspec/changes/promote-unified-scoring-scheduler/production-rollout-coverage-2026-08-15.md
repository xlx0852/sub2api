# Production rollout coverage — 2026-08-15

## Scope and safety

This inventory was produced from read-only aggregate queries against the authorized US production database and a local source audit. It did not send business requests, read credentials, print account/user names, or inspect request content.

The rollout at inventory time was `{"default":"off","groups":{"openai":{"4":"enforce"}}}`. After the inventory, the user explicitly accepted skipping multi-day evidence collection for low-traffic groups. All OpenAI groups moved to platform-level enforce at `2026-08-15T13:46:08Z`. A real Gateway enforce branch was then implemented and deployed directly to US production at the user's direction, skipping Japan staging; at `2026-08-15T14:07:37Z` rollout became `{"default":"enforce"}`. The new 18244 container remained healthy with restart count zero through the initial cutover window; the old 18243 container was stopped but retained for rollback.

## Active and historical scopes

| Platform / group | Usage 24h | Usage 7d | Schedulable candidates now | Current protocol evidence | Disposition |
|---|---:|---:|---:|---|---|
| OpenAI / 4 | 25,501 | 236,622 | 7 OAuth | Responses stream + WSv2, chat completions, images | `enforce` passed; keep as control scope |
| OpenAI / 25 | 122 | 5,018 | 5 (4 OAuth, 1 non-OAuth) | Responses stream + WSv2; historical chat completions | Next `shadow -> enforce` candidate |
| Grok / 23 | 577 | 3,092 | 3 (2 OAuth, 1 non-OAuth) | Responses sync + stream; historical chat completions | Shadow-only until Gateway enforce exists |
| OpenAI / 20 | 0 | 3,205 | 1 non-OAuth | Historical Responses sync + stream | No current live window; cannot canary now |
| OpenAI / 30 | 0 | 1,796 | 0 | Historical Responses sync | Stale historical scope; no candidate pool now |
| Kimi / 26 | 0 | 440 | 2 OAuth | Historical Messages sync + stream | No current live window and Gateway enforce absent |
| Anthropic / 18 | 0 | 307 | 1 non-OAuth | Historical Messages sync + stream | No current live window and Gateway enforce absent |
| Anthropic / 28 | 0 | 167 | 1 non-OAuth | Historical Messages sync + stream | No current live window and Gateway enforce absent |

Additional inventory facts:

- OpenAI group 4 peak observed hour in the last seven days: 3,955 requests; group 25 peak: 876.
- Grok group 23 peak observed hour: 360 requests.
- Group 29 has nine currently schedulable OpenAI accounts but no active API key or usage evidence, so it is not a production canary scope.
- No Gemini production usage appeared in the seven-day aggregate; staging already records its current retired/unavailable supply baseline.

## Source audit blocker

`backend/internal/service/gateway_scheduling.go` consults rollout mode only for `shadow`: it computes the counterfactual shared policy/score result before the legacy side-effect path. There is no `RolloutEnforce` branch that makes the Gateway shared score produce the live selection. Therefore Grok, Kimi, Anthropic, Gemini, mixed Antigravity, passthrough, and other Gateway paths cannot honestly be marked enforce-tested yet.

This is not a compatibility wrapper or evidence-only omission. Implementing Gateway enforce changes live account selection and must retain the existing policies, affinity, acquire/wait, routing, mixed-platform and failover semantics under the approved rollout gate. Until that work and its staging/production evidence are complete, task 6.1 cannot approve deletion of either legacy scheduler body.

## Next action

1. Keep the user-approved OpenAI platform-level enforce under the existing instant `off` kill switch.
2. Implement and validate an explicit Gateway enforce path before any non-OpenAI production enforce or task 6.1 deletion approval.
3. Preserve the existing Gateway hard policies, affinity, acquire/wait, routing, mixed-platform and failover semantics under `off`/`shadow`; only an explicit enforce scope may use the shared order.
