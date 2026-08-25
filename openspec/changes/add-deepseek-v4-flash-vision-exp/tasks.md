## 1. Model registration

- [x] 1.1 Add the model and self-mapping to both bundled catalogs.
- [x] 1.2 Add vision billing metadata and frontend whitelist/preset.
- [x] 1.3 Add regression tests and validate JSON/catalog consistency.

## Implementation notes

- 2026-08-26: API `0.1.341` deployed backend-only to US production on port `18262`; frontend remains `0.1.339`, API `0.1.340`/`18261` is retained as rollback. Candidate health/version, binary model-ID presence, unauthenticated catalog boundary, Nginx validation, and post-cutover public checks passed. Backup: `/opt/sub2api/backups/pre-0.1.341-20260825T161633Z`.
