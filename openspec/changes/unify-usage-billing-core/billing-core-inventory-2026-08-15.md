# Billing-core inventory — 2026-08-15

## Input mapping

| Gateway | OpenAI | Canonical meaning |
|---|---|---|
| `ForwardResult.RequestID` | `OpenAIForwardResult.RequestID/BillingRequestID` | billing identity source |
| `ClaudeUsage` | `OpenAIUsage` | provider-extracted usage buckets |
| `Model/UpstreamModel` | `Model/BillingModel/UpstreamModel` | display and pricing model candidates |
| `RecordUsageInput` entities/endpoints/client metadata | same fields on `OpenAIRecordUsageInput` | billing subject and attribution |
| `ForceCacheBilling` | absent | Gateway sticky-failover bucket policy |
| absent | `CyberBlocked` | OpenAI request-type attribution |
| Gateway image fields | OpenAI image/video fields | optional media billing |
| Gateway duration/stream/reasoning | OpenAI duration/stream/reasoning plus tier/compact/WS | common log plus optional attribution |

Both paths already share domain `Account/APIKey/User`, `ChannelUsageFields`, `UsageBillingCommand`, `applyUsageBilling`, `UsageSettlementPayloadV1`, repository apply/settle, post-commit effects, and best-effort legacy usage-log persistence.

## Duplicate implementation map

| Concern | Gateway implementation | OpenAI implementation |
|---|---|---|
| top-level cost dispatch | `calculateRecordUsageCost` | `calculateOpenAIRecordUsageCost` |
| token cost | `calculateTokenCost` | `calculateOpenAIRecordUsageTokenCost` |
| image cost | `calculateImageCost` | `calculateOpenAIImageCost` |
| video cost | none | `calculateOpenAIVideoCost` |
| channel pricing | `resolveChannelPricing` | `resolveOpenAIChannelPricing` |
| group multiplier | `getUserGroupRateMultiplier` | inline resolver |
| normalization | `normalizeClaudeUsageForBilling` | `normalizeOpenAIUsageForBilling` |
| usage log | `buildRecordUsageLog` | inline `UsageLog` literal and field mutation |
| durable/legacy commit | same shape in both `RecordUsage` bodies | same shape in both `RecordUsage` bodies |

## Differences that must remain named

1. Gateway/Gemini long-context uses the caller-provided fixed `200000/2.0`; OpenAI uses account-level `IsOpenAILongContextBillingEnabled` and model pricing policy.
2. OpenAI exhausts model candidates and records zero cost when pricing is unavailable; Gateway logs the calculation failure and records zero cost through its existing fallback.
3. OpenAI splits total input into mutually exclusive input/cache-read/cache-creation buckets; Gateway preserves provider buckets and can move all input into cache-read under `ForceCacheBilling`.
4. OpenAI WS without durable handoff can retain the upstream request ID; durable and non-WS paths use the local billing identity rules.
5. OpenAI-only `CyberBlocked`, compact, service tier, video and WS metrics remain optional usage-log fields.

## Existing characterization coverage

- Long context: `TestGatewayServiceRecordUsageWithLongContext_*`, `TestOpenAIGatewayServiceRecordUsage_Gpt54LongContextBilling*`.
- Pricing unavailable: `TestOpenAIGatewayServiceRecordUsage_MissingPricingRecordsZeroCostUsageLog`, `...UnpricedTokenModelFallsBackToZeroCostUsageLog`.
- WS identity: `...WSModePrefersUpstreamRequestID...`, `...WSModePrefersLocalBillingRequestID`.
- OpenAI bucket split: `...ClampsActualInputTokensToZero`, `...GPT56SeparatesCacheWriteForBillingAndStats`.
- Cyber/media/tier: `TestRecordUsageMarksCyberRequestType`, image/video/channel pricing tests, priority/flex tier tests.

Gateway missing-pricing zero-cost persistence and `ForceCacheBilling` log buckets require explicit characterization before moving the implementations.
