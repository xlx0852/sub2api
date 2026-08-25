# Change: Use DeepSeek Official Chinese Pricing

## Why

The DeepSeek V4 billing path already applies peak/off-peak time windows, but its hard-coded CNY baseline is stale. It currently charges the previous Flash/Pro rates and does not explicitly cover the newly added Vision Exp model.

## What Changes

- Pin DeepSeek V4 Flash and Vision Exp to the current official Chinese Flash rates.
- Pin DeepSeek V4 Pro to the current official Chinese Pro rates.
- Preserve the existing UTC peak windows and off-peak half-price multiplier.
- Synchronize bundled fallback pricing and regression tests.

## Impact

- Affected specs: usage billing and model catalog pricing.
- No database migration or API contract change.
