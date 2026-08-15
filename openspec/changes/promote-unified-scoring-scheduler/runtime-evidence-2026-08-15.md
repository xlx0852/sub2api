# Unified scoring scheduler runtime evidence (2026-08-15)

All remote collection was read-only and aggregate-only. No gateway request was sent, no deployment/config/database state was changed, and no `.env`, container environment, account credentials, request content, response content, tokens, or raw session identifiers were read. The US production target was `89.208.254.77`; Japan staging was `103.150.8.121`.

## Deployment and observation boundary

- US production API: version `0.1.319`, container healthy, loopback port `18243`, binary commit marker `f94204d8a-dirty`.
- Japan staging API: version `0.1.319-staging-80f6f41850`, separated frontend/API deployment, healthy.
- Production has no Prometheus/OTel scheduler export. `openAIAccountRuntimeStats` and `OpenAIAccountSchedulerMetricsSnapshot` are process-local and have no read-only route/ops export in this deployment.
- `usage_logs` covers successfully settled requests, while Nginx logs cover HTTP statuses. Neither alone is a full scheduler-attempt ledger.

## Effective advanced scheduler state

Production database settings:

- `openai_advanced_scheduler_enabled=true`
- `openai_advanced_scheduler_sticky_weighted_enabled=true`
- `openai_advanced_scheduler_subscription_priority_enabled=true`
- TopK and weight database overrides are empty, so deployment config/defaults are effective.

Effective production values:

| Input | Value |
|---|---:|
| TopK | 7 |
| priority | 1.0 |
| load | 1.0 |
| queue | 1.0 |
| error rate | 0.8 |
| TTFT | 1.2 |
| reset | 1.0 |
| quota headroom | 0.0 |
| previous response | 5.0 |
| session sticky | 3.0 |
| compact error | 1.0 |
| compact latency | 0.5 |

Production schedulable supply:

| Platform / credential type | Total | Schedulable |
|---|---:|---:|
| Anthropic API key | 2 | 2 |
| Grok API key | 2 | 2 |
| Grok OAuth | 2 | 2 |
| Kimi OAuth | 2 | 2 |
| OpenAI API key | 4 | 3 |
| OpenAI OAuth | 8 | 7 |

Japan staging has advanced scheduling disabled and only one schedulable OpenAI API-key account. It can validate protocol, hard policies and shadow side-effect isolation, but cannot validate TopK or multi-account load distribution without isolated seeded fixtures.

## Representative production windows

Windows were selected from the previous seven complete days using non-empty hourly settled-request counts: the median active hour is the normal window and the maximum hour is the peak window.

| Window | UTC hour | Settled requests | Platform detail |
|---|---|---:|---|
| Normal | 2026-08-09 10:00 | 1,170 | OpenAI 1,170; 1,167 stream; 1,013 WS; 7 accounts; 2 groups |
| Peak | 2026-08-10 10:00 | 4,204 | OpenAI 4,183; 3,781 stream; 2,100 WS; 7 accounts; 2 groups. Grok 21 |

TTFT and load distribution:

| Window / platform | TTFT samples | p50 | p95 | Per-account request min / p50 / max | Load CV |
|---|---:|---:|---:|---:|---:|
| Normal OpenAI | 1,132 | 2,562 ms | 11,308 ms | 75 / 171 / 283 | 0.4442 |
| Peak OpenAI | 3,653 | 2,918 ms | 7,561 ms | 250 / 679 / 896 | 0.4504 |
| Peak Grok | 20 | 1,436 ms | 2,843 ms | 10 / 10 / 11 | 0.0476 |

Nginx status proxy for the same UTC hours:

| Window | Total | 2xx | 3xx | 4xx | 499 | 5xx |
|---|---:|---:|---:|---:|---:|---:|
| Normal | 2,231 | 1,982 | 153 | 53 | 0 | 4 (0.179%) |
| Peak | 2,741 | 2,432 | 160 | 109 | 46 (1.678%) | 3 (0.109%) |

Nginx totals include control/static traffic and therefore are a conservative HTTP-status proxy, not a per-upstream-attempt error rate.

Current OpenAI quota snapshot:

- Three schedulable API-key accounts have no Codex 5h/7d percentage signal; availability must be false, not favorable zero.
- Seven schedulable OAuth accounts all have 5h and 7d signals. Current 5h p50/max is `0/0`; current 7d p50/max is `38/78`.

## Request classes and candidate rollout groups

Previous 24 hours:

| Group | Platform | Requests | Stream | WS | Media | Accounts | Credential distribution |
|---:|---|---:|---:|---:|---:|---:|---|
| 4 | OpenAI | 26,005 | 25,745 | 10,194 | 24 | 7 | OAuth only in observed traffic |
| 25 | OpenAI | 1,296 | 0 | 0 | 0 | 5 | OAuth 1,039 / API key 257 |
| 20 | OpenAI | 32 | 32 | 0 | 0 | 1 | API key only |
| 23 | Grok | 575 | 571 | 0 | 0 | 3 | OAuth + API key |
| 28 | Anthropic | 44 | 43 | 0 | 0 | 1 | API key only |

All three OpenAI groups are active, do not require OAuth-only or privacy-set, and have no group RPM limit. Groups 4 and 25 allow Messages dispatch; group 20 does not.

Rollout recommendation:

1. shadow group 25 first because it has five active accounts, meaningful mixed credential traffic, and no WS/stream in the observed day;
2. use single-account group 20 as the first enforce control to prove side-effect and kill-switch mechanics without changing account choice;
3. enforce group 25 only after credential-class and mixed-pool diffs are clean;
4. keep high-volume WS-heavy group 4 last.

## Runtime-stat and affinity evidence gaps

- Previous 24-hour logs contain zero `sticky_reeval_switch`, zero `sticky_escape_triggered`, and zero advanced-setting load failures. Zero event logs do not prove zero sticky hits because normal selection/hit counters are not exported.
- Gateway emitted 44 `sticky.scheduler_entry` info events; candidate-list and layer-hit logs are debug-level and unavailable in the production log level.
- Candidate counts before/after hard gates, runtime error-rate EWMA, TTFT EWMA by account, TopK membership, wait-plan decisions, selection-count metrics and exact sticky previous/session hit totals cannot be read from the current deployment.
- A usage-log account-switch proxy reports 6,507 switches among 27,205 comparable OpenAI rows in 24h, but it groups by user/group/model rather than true affinity key and MUST NOT be used as a sticky-switch SLO.

These gaps require an aggregate, non-sensitive `shadow`/ops snapshot before any multi-account `enforce`; they do not block implementing and replay-testing the pure shared scorer.

## Preliminary canary guardrails

Final thresholds must be re-baselined from the new shadow output. Initial stop conditions are:

- hard-policy violation, credential-class crossing, duplicate acquire/release, duplicate sticky/session write, or duplicate wait-plan side effect: exactly zero tolerated;
- Nginx 5xx rate: no more than baseline plus 0.5 percentage points;
- 499 rate: no more than baseline plus 2 percentage points;
- OpenAI TTFT p50/p95: no more than 20% regression for the matching request class/window;
- per-account load CV: no increase greater than 0.10 absolute unless quota/cooldown exclusion explains it;
- any scorer internal fallback or kill-switch trigger blocks scope expansion until explained.

## Approved phase-4.4 shadow and canary thresholds

These are the initial thresholds derived from the normal and peak read-only windows above. They govern shadow review and the later group-scoped canary; they do not authorize production `enforce` by themselves. Each comparison must use the same request class and window type.

| Signal | Normal-window stop threshold | Peak-window stop threshold |
|---|---:|---:|
| Nginx 5xx proxy | > 0.679% (0.179% + 0.5 pp) | > 0.609% (0.109% + 0.5 pp) |
| Nginx 499 proxy | > 2.000% (0% + 2 pp) | > 3.678% (1.678% + 2 pp) |
| OpenAI TTFT p50 | > 3,075 ms (+20%) | > 3,502 ms (+20%) |
| OpenAI TTFT p95 | > 13,570 ms (+20%) | > 9,074 ms (+20%) |
| Per-account load CV | > 0.5442 (+0.10 absolute) | > 0.5504 (+0.10 absolute) |

Hard stop conditions for both windows:

- `hard_gate_violation_total`, credential/capability/quota/cooldown/RPM/window-cost crossing, duplicate acquire/release, duplicate sticky/session mutation and duplicate wait-plan creation must remain exactly zero.
- `fallback_total` must remain zero after the scope's error/TTFT/load samples are warmed. Any `runtime_stats_not_warmed`, scorer-internal fallback or kill-switch event blocks expansion; it is not averaged away.
- The seven observed OAuth candidates must retain quota-signal availability; the three API-key candidates without percentage data must remain `unavailable`, never favorable zero. A load-CV increase is acceptable only when the corresponding structured policy trace attributes it to quota/cooldown exclusion.
- The current deployment has no upstream-attempt ledger, so the Nginx 5xx proxy is the only production-window error/failover ceiling that can be honestly derived. Before any multi-account `enforce`, staging fault injection must separately establish 401/403/429/5xx failover counts and require no final-response regression or duplicate side effect. The 24-hour usage switch proxy remains excluded from the threshold.
- Peak Grok TTFT (20 samples) is retained as an observation only; its sample is too small to authorize Grok enforce. Non-OpenAI target platforms require their own staging/representative shadow windows before scope expansion.

## Route conclusion

The scoring scheduler is actively enabled in production with non-default weights and substantial multi-account OpenAI traffic. Retiring it would remove live behavior. The selected “promote scoring” route is supported, but promotion must start with pure-core equivalence and aggregate shadow observability; production enforce is not yet authorized by this evidence alone.

## Related-change ownership boundary

`harden-openai-pool-outcome-sticky-stream` retains ownership of:

- `UpstreamOutcomeClass`, status/error classification and cooldown/credential health mutation;
- credential class/generation and previous-response/session affinity validity;
- `ComputeOpenAIUsageScore`, sticky interval re-evaluation, lowest-usage routing and sticky escape semantics;
- stream stall clocks, incomplete terminal behavior and partial usage settlement;
- its remaining tasks 2.7, 3.5, 4.1, 4.2 and 4.4.

`promote-unified-scoring-scheduler` owns:

- platform-neutral immutable candidate/signal/score types;
- hard-policy diagnostics before scoring;
- exact migration of the existing OpenAI normalization/weighted TopK mechanics without redefining the inputs above;
- Gateway runtime feedback adapters and bounded shared stats storage;
- `off`/`shadow`/`enforce`, counterfactual diffs and final removal of duplicate scheduler bodies.

Call-site adaptation may consume the related change's stable outputs, but it MUST NOT change its thresholds, state transitions, failure classes, sticky TTL/generation rules or stall settlement. Any required interface that is not stable remains an explicit blocker rather than being reimplemented here.
