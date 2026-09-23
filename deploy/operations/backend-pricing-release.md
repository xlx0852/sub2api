# Backend resources must be part of the release

Use the same reviewed source tree for the binary and its runtime resources. The
pricing file is **not** the model catalog: `fallback_pricing` may contain alias
records (`alias_of`), which the LiteLLM price loader cannot interpret. Do not
export that map directly into a runtime pricing JSON.

## Package and validate

From `backend/`:

```sh
make release-bundle
# Or package an already-built linux/amd64 binary from an isolated deploy tree:
python3 scripts/package-release.py --binary /path/to/sub2api \
  --output /path/to/sub2api-release.tar.gz
```

The archive contains:

- `sub2api`
- `resources/model-pricing/model_prices_and_context_window.json` (the reviewed
  repository price file, copied byte-for-byte)
- `resources/model-catalog/catalog.json`
- `SHA256SUMS` (covers both resources and the binary)

Extract into a **new versioned directory**, then run `sha256sum -c SHA256SUMS`
there before any promotion. `package-release.py` fails if either resource is
missing or invalid and never replaces a previously valid archive on failure.

Both source Dockerfiles and GoReleaser images include `backend/resources` under
`/app/resources`. GoReleaser archives also carry both resources at the paths
above. For hand-run containers with bind mounts, image contents alone are not
enough: an older `/app/resources` bind mount hides them.

## Promote on a bind-mounted deployment

1. Inspect the running container (do not print its credential environment), and
   save a permission-restricted rollback snapshot.
2. In the replacement container point `/app/sub2api`, `/app/resources`, and any
   `/app/release-model-catalog` override at **the same new release's files**.
   Never keep a resource mount pointing at an unrelated old release.
3. Keep `PRICING_DATA_DIR=/app/data/pricing` on the writable data volume. Keep the
   remote pricing policy unchanged unless an operator explicitly approves it.
4. Price data can change customer charges. Compare the new file against the
   currently active table and verify representative model/tier/currency prices
   on a candidate before promotion. No price rewrite is implied by packaging.
5. The pricing service loads `model_pricing.json` in `PRICING_DATA_DIR` **before**
   its fallback file. Merely updating the fallback file and restarting does not
   overwrite an existing cache. When deliberately promoting reviewed prices in
   local-only mode, back up the old cache, atomically install the reviewed file
   there (or move the cache aside to let startup seed it), and restart. Preserve
   ownership so the runtime user can write this directory.
6. Verify the initialization log names the expected path/count, health succeeds,
   and billable requests resolve the expected rates. Rollback must restore the
   binary/resource mounts **and** the previous pricing cache together.

No production price files or container settings are changed by `make
release-bundle`; it produces an artifact for the normal deployment approval and
canary process.
