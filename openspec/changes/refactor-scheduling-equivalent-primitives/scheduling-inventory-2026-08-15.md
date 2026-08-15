# Scheduling equivalent-primitives inventory (2026-08-15)

Baseline: `50f9f1fb9f8b99040b75f6801532e2835f1b011e` (`refactor(billing): 合并 usage 结算核心`), clean worktree before this change.

| Area | Current production implementations | Equivalent boundary | Must remain platform-specific |
|---|---|---|---|
| Channel pricing restriction | `GatewayService.checkChannelPricingRestriction`, `OpenAIGatewayService.checkChannelPricingRestriction` | Entire function body is equivalent | Upstream per-account model mapping/check remains separate |
| Account listing | Gateway/OpenAI `listSchedulableAccounts` | Snapshot-first and grouped/ungrouped single-platform repository query | Gateway mixed Anthropic/Gemini + Antigravity filtering; OpenAI compatible-platform normalization and later privacy/credential/runtime gates |
| Priority/LRU ordering | One service implementation `sortAccountsByPriorityAndLastUsed`, already called by both paths | `ComparePriorityLRU`, equal-run grouping and shuffle mechanics | `preferOAuth` partition and load-aware sort keys |
| Sticky candidate handling | Gateway Layer 1.5, OpenAI `tryStickySessionHit`, advanced `selectBySessionHash` | Presence/exclusion/clear/candidate-match decision only | Gateway `checkAndRegisterSession`, wait queue and RPM gates; OpenAI credential class, epoch, transport/capability, runtime block, quota re-eval/escape and weighted scoring |
| Prefetch | Gateway window-cost/RPM context prefetch; OpenAI request-scoped account/parent `GetByIDs` prefetch | No common payload or fallback semantics; keep separate | Gateway combines Redis counters, cache and batch/fallback SQL with metrics; OpenAI hydrates credential/parent account snapshots and fail-opens on batch read failure |

The active `harden-openai-pool-outcome-sticky-stream` change owns outcome classes, credential generation, sticky re-evaluation, lowest-usage and failover-class behavior. Those symbols and settings are excluded from this change except for call-site adaptation that preserves their current order and values.
