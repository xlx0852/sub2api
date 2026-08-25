## 1. Pricing

- [x] 1.1 Update DeepSeek hard-coded peak baselines and Vision Exp classification.
- [x] 1.2 Synchronize bundled fallback pricing copies.
- [x] 1.3 Update peak/off-peak and model-specific billing regression tests.

## Implementation notes

- 2026-08-26: Rechecked the user-provided official Chinese pricing page. Current rates are Flash/Vision Exp peak ¥3/¥9/¥0.10 and Pro peak ¥9/¥27/¥0.30 per million input/output/cache-hit tokens; idle is half-price, with peak only on Beijing weekdays 09:00–12:00 and 14:00–18:00. The implementation now enforces weekday-only peak windows. Local billing, pricing, catalog, and JSON regression checks pass. API `0.1.342` is deployed backend-only to US production on `18263`; frontend remains `0.1.339`, API `0.1.341`/`18262` is retained as rollback. Backup: `/opt/sub2api/backups/pre-0.1.342-20260825T163137Z`.
