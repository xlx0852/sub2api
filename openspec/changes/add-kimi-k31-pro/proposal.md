# Change: Add Kimi K3.1 Pro catalog and pricing support

## Why

Kimi K3.1 Pro is available from Moonshot's official API, but the model catalog does not expose it and
billing does not have an exact price entry for it. Requests using `kimi-k3.1-pro` can therefore miss the
intended catalog metadata and can be captured by the existing broad `kimi-k3` fallback pricing rule,
which would apply the wrong rates. Compatibility spellings such as `kimi3.1` and
`moonshot/kimi-k3.1-pro` are also not normalized for Kimi upstream traffic.

## What Changes

- Add `kimi-k3.1-pro` as the canonical Kimi chat model, with the display name `Kimi K3.1 Pro`, reasoning
  and image/video capabilities, and a 1,048,576-token context window.
- Add a self mapping, a client preset, and the public compatibility alias `kimi3.1`.
- Add one CNY fallback price entry for the canonical model. Standard rates use the base price fields;
  Highspeed rates use the existing `*_priority` service-tier fields instead of creating a second model
  or a parallel Highspeed price entry.
- Resolve `kimi3.1`, `kimi-k3.1-pro`, and `moonshot/kimi-k3.1-pro` to the canonical entry before the
  existing broad `kimi-k3` fallback match can run.
- Normalize those same three spellings to `kimi-k3.1-pro` for Kimi upstream requests while preserving
  the existing trailing-parentheses behavior and removing a trailing `[1m]` marker.
- Mirror the canonical price in the local model-pricing snapshot without replacing its existing Kimi
  aliases or unrelated entries.

## Official Rates

The confirmed rates below are CNY per one million tokens and apply to K3.1 Pro context caching with the
published five-minute TTL:

| Service tier | Input | Cache hit | Cache write | Output |
| --- | ---: | ---: | ---: | ---: |
| Standard | 13.00 | 1.30 | 13.00 | 68.00 |
| Highspeed | 26.00 | 1.30 | 26.00 | 136.00 |

## Sources

- Kimi K3.1 Pro quickstart: [official quickstart](https://platform.kimi.ai/docs/guide/kimi-k3-1-pro-quickstart.md)
- Kimi chat pricing: [official pricing](https://platform.kimi.ai/docs/pricing/chat.md)

## Impact

- Affected specs: `model-catalog`, `kimi-oauth-accounts`.
- Affected code: both model catalog copies, the local model-pricing snapshot,
  `backend/internal/service/billing_service.go`, and
  `backend/internal/service/openai_codex_transform.go`.
- No database migration or public API shape change. Existing Kimi models, GPT/Grok pricing, and scheduler
  cache behavior remain unchanged.
- The new catalog and fallback prices take effect with the next binary/client artifact deployment.

## Approval Gate

Implementation must not begin until this change has passed strict OpenSpec validation and the user has
approved the proposal.
