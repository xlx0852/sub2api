---
name: update-model-catalog
description: Safely update, validate, publish, and verify Sub2API first-party model metadata in the project catalog and the xlx0852/model-catalog shared repository. Use when adding, renaming, mapping, retiring, repricing, or changing capabilities of OpenAI, Anthropic, Gemini, Antigravity, Grok, or Bedrock models; when the user asks how to update models; or when catalog.json, model mappings, fallback pricing, media flags, aliases, UI presets, catalog versions, checksums, or remote model refresh are involved.
---

# Update Model Catalog

Keep these boundaries explicit:

- Public catalog: model metadata, defaults, aliases, public fallback costs, and UI presets.
- Account upstream sync: models a credential can actually call.
- Database configuration: account whitelist/mapping, channel pricing, tenant routing, and custom group lists.

Never put secrets, private upstream URLs, tenant prices, credentials, or account data in the public catalog.

## Required files

Update the canonical project file:

```text
backend/resources/model-catalog/catalog.json
```

Then synchronize the embedded copy:

```text
backend/internal/pkg/modelcatalog/catalog.json
```

Publish the same validated document to:

```text
https://github.com/xlx0852/model-catalog
├── catalog.json
├── catalog.sha256
└── versions/<version>/
```

Read [references/catalog-format.md](references/catalog-format.md) before changing fields, mappings, aliases, prices, or presets.

## Workflow

1. Inspect both project catalog copies, the shared repository, and the active model-related changes. Do not overwrite unrelated work.
2. Edit `backend/resources/model-catalog/catalog.json` as the canonical source.
3. Increment integer `version` and set `updated_at` to the publication time in UTC RFC 3339 form.
4. Synchronize the embedded copy only after the canonical document is final:

   ```bash
   cp backend/resources/model-catalog/catalog.json backend/internal/pkg/modelcatalog/catalog.json
   ```

5. Run the bundled verifier from the Sub2API repository root:

   ```bash
   python3 skills/update-model-catalog/scripts/verify_catalog.py
   ```

6. Run relevant backend tests:

   ```bash
   cd backend
   go test ./internal/pkg/modelcatalog ./internal/pkg/openai ./internal/pkg/claude \
     ./internal/pkg/geminicli ./internal/pkg/xai ./internal/domain
   ```

7. Clone or update the shared repository outside the Sub2API worktree. Copy the validated canonical catalog into `catalog.json`.
8. In the shared repository, run:

   ```bash
   python3 scripts/validate_catalog.py --write-checksum
   mkdir -p "versions/$(jq -r .version catalog.json)"
   cp catalog.json catalog.sha256 "versions/$(jq -r .version catalog.json)/"
   git diff --check
   ```

9. Confirm the version directory does not already contain different content. Never rewrite an already published version; increment the version instead.
10. Commit and push the shared repository. Wait for its `Validate catalog` GitHub Actions workflow to pass.
11. Verify published bytes and checksum:

    ```bash
    curl -fsSLO https://raw.githubusercontent.com/xlx0852/model-catalog/main/catalog.json
    curl -fsSLO https://raw.githubusercontent.com/xlx0852/model-catalog/main/catalog.sha256
    sha256sum --check catalog.sha256
    ```

12. Trigger or wait for Sub2API refresh. Check `GET /api/v1/admin/system/model-catalog/status`; confirm version, source, hash, success count, and empty `last_error`.
13. Verify the affected model surfaces. For model IDs/mappings, check account selection and `/v1/models`; for metadata, check model catalog/model plaza; for fallback prices, run pricing tests.

## Safety rules

- Treat the remote repository as distribution, not the editing source. Start from the Sub2API canonical catalog unless explicitly importing a reviewed remote change.
- Reject duplicate or empty model IDs, unknown alias targets, negative prices, cyclic `alias_of`, invalid JSON, stale versions, and checksum mismatch.
- Do not publish a catalog merely because an upstream `/v1/models` response contains a new ID. Confirm provider, product status, expected routing, capabilities, and pricing first.
- Do not automatically add a public catalog model to account whitelists, channel pricing, or tenant routing.
- Preserve the last-known-good remote version until the new GitHub Actions validation passes.
- If production misbehaves, publish a corrected higher version or disable `model_catalog.remote_enabled`; do not silently mutate an immutable version snapshot.
