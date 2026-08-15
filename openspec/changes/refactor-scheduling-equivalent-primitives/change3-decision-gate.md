# Change 3 decision gate (deferred)

No scheduler philosophy is selected or implemented by this change. A future proposal must first collect a production-safe, read-only snapshot for at least one representative peak window and one normal window.

## Required runtime evidence

- Advanced scheduler enablement by group and the effective values of `openai_advanced_scheduler_*`, including sticky-weighted, TopK/exploration and score weights.
- `openAIAccountRuntimeStats` selection count, upstream error rate, TTFT distribution, concurrency-full events, sticky previous/session hits, sticky re-evaluation switches and escape reasons.
- Candidate counts before/after hard gates, lowest-usage tie frequency, selected credential class and account-switch frequency per request class (sync, stream, WS, compact, image/media).
- Gateway rule scheduler candidate counts, sticky hits, quota/RPM/window-cost exclusions, wait-plan decisions and mixed Antigravity participation for the same windows.
- Operator-confirmed accounts/groups that intentionally rely on advanced scheduling, plus rollback tolerances for affinity, TTFT, error rate and quota distribution.

## Decision routes

- Promote scoring: requires evidence that scoring materially improves TTFT/error/load distribution and a design for Gateway runtime-stat collection plus rule gates as `CandidatePolicy`.
- Retire scoring: requires evidence that the advanced settings are unused or provide no material benefit; unique credential/compact/channel/sticky-epoch behavior must first move into the load-aware engine with differential tests.
- Keep both: use when both philosophies have active, distinct production use cases; only shared primitives continue to converge.

Any route changes candidate thresholds, score/order, sticky escape or failure behavior and therefore requires a new OpenSpec change. It must not be appended to this behavior-equivalent refactor.
