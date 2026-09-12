# Model Pricing Data

This directory contains a local copy of the mirrored model pricing data as a fallback mechanism.

## Source
The original file is maintained by the LiteLLM project and mirrored into the `price-mirror` branch of this repository via GitHub Actions:
- Mirror branch (configurable via `PRICE_MIRROR_REPO`): https://raw.githubusercontent.com/<your-repo>/price-mirror/model_prices_and_context_window.json
- Upstream source: https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json

## Purpose
This local copy serves as a fallback when the remote file cannot be downloaded due to:
- Network restrictions
- Firewall rules
- DNS resolution issues
- GitHub being blocked in certain regions
- Docker container network limitations

## Update Process
The pricingService will:
1. First attempt to download the latest version from GitHub
2. If download fails, use this local copy as fallback
3. Log a warning when using the fallback file

## Manual Update
To manually update this file with the latest pricing data (if automation is unavailable):
```bash
curl -s https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json -o model_prices_and_context_window.json
```

## File Format
The file contains JSON data with model pricing information including:
- Model names and identifiers
- Input/output token costs
- Context window sizes
- Model capabilities

Last pricing review: 2026-09-11 (upstream snapshots retrieved 2026-09-10).

The snapshot contains project-specific aliases and CNY entries; do not blindly
replace it with LiteLLM's USD data. The September review compared all 218 existing
entries against the current LiteLLM source (195 exact matches, 23 unmatched local
aliases/provider entries). Matching cost fields were refreshed while preserving
currencies, model metadata and custom entries. Unmatched entries were retained;
this comparison is not a claim that every provider's live price was verified.

DeepSeek V4.1 Flash uses official API ID `deepseek-flash`.
`deepseek-v4.1-flash` is a local compatibility/pricing name with an explicit model
mapping preset to `deepseek-flash`. Peak CNY prices per million tokens are 2 input,
0.04 cache hit and 8 output. Off-peak is half, using Beijing weekday/time rules
in BillingService. Cache-creation tokens retain the project's input-price convention.
The existing V4 Flash and Vision Exp names use the same new rates; retired
`deepseek-chat`/`deepseek-reasoner` retain this project's Flash billing fallback.
V4 Pro remains 9 / 0.30 / 27 until its announced 2026-09-14 12:00 Beijing transition;
that future price transition is not applied early by this snapshot.

Official references:
- https://api-docs.deepseek.com/zh-cn/quick_start/pricing
- https://api-docs.deepseek.com/updates/
- https://developers.openai.com/api/docs/models/gpt-5.6-sol
- https://developers.openai.com/api/docs/models/gpt-realtime-2
- https://ai.google.dev/gemini-api/docs/pricing

GPT-5.6 Sol now uses 4 / 0.40 / 20 USD per million tokens; cache writes are 1.25x
input. Its service-tier fields and static fallback are updated together. Google
audio, TTS and image entries preserve distinct per-token/per-image fields.
Historical release artifacts and tenant/channel price overrides are not rewritten.
