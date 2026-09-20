## 1. Audit and baseline

- [x] 1.1 Inventory Gemini API batch provider production wiring, public types, docs, and tests; confirm Vertex and generic batch infrastructure are separate and remain in scope.
- [x] 1.2 Read-only audit the current US production `batch_image_jobs`, `batch_image_items`, and `batch_image_events` tables; record the 2026-08-16 zero-row snapshot without reading user/account data.
- [x] 1.3 Capture the targeted pre-change batch image test baseline.

## 2. Remove Gemini API batch provider

- [x] 2.1 Remove `GeminiAPIBatchImageProvider`, its Gemini Batch HTTP client/JSONL/state mapping, and provider-specific tests.
- [x] 2.2 Remove `gemini_api` from domain/service provider constants, registry construction, supported-provider validation, and automatic selection order; preserve `vertex`.
  - 后续：Vertex 供应已整包退役，批图实现栈已删除，不再保留 Vertex Provider。
- [x] 2.3 Make explicit `gemini_api` requests return the existing unsupported-provider error without falling through to Vertex.
- [x] 2.4 Migrate generic repository/settlement/download/cleanup tests from the retired constant to Vertex or local fake providers.

## 3. Client and documentation cleanup

- [x] 3.1 Remove `gemini_api` from frontend batch-image provider typing while preserving Vertex and backward-compatible unknown-string decoding where required.
- [x] 3.2 Update `docs/BATCH_IMAGE_MVP.md` to describe Vertex-only provider support and remove Gemini API Batch setup/cost instructions.
- [x] 3.3 Confirm no Gemini Batch-only contract remains in OpenAPI or deployment examples; do not remove generic `batch_image.*` keys.

## 4. Verification and rollout

- [x] 4.1 Run targeted repository/provider/worker/settlement/download/cleanup tests, including race coverage where practical.
- [x] 4.2 Run archtest, `git diff --check`, strict OpenSpec validation, and root `make build`.
- [ ] 4.3 In staging, verify explicit `gemini_api` rejection, Vertex registry/worker behavior, startup with batch image disabled, and paired backend rollback.
- [x] 4.4 Immediately before production cutover, re-run the read-only data gate. Abort if any non-terminal Gemini job, unsettled hold, or pending output cleanup exists.
- [ ] 4.5 Deploy backend-only, verify health and unaffected HTTP/SSE/WS plus billing canaries, retain the old backend for rollback, and archive only after observation.

## Implementation notes

- 2026-08-16: current US production snapshot returned zero rows in all three batch-image tables. This is evidence for proposal approval only and must be refreshed at 4.4.
- 2026-08-16: 67 non-admin provider/worker/settlement/download/cleanup unit-tag tests pass. The broader batch-image name filter also exposes two stale admin group tests that still create the retired Gemini platform; implementation must migrate those fixtures rather than re-enable Gemini account creation.
- 2026-08-16: 2.1 deleted the Gemini API batch provider, HTTP/File API client, JSONL/state mapping, and their dedicated unit tests. Registry/domain references are intentionally handled next in 2.2 before compiling.
- 2026-08-16: 2.2 removed the retired constant and inline-result error, made supported-provider validation and default/config registries Vertex-only, and changed automatic selection order to Vertex only. Generic fixtures are migrated separately in 2.4.
- 2026-08-16: 2.3 added an explicit public-service regression case for `provider=gemini_api`; validation returns `BATCH_IMAGE_UNSUPPORTED_PROVIDER` before provider/account selection, so the request cannot fall through to Vertex or reach upstream.
- 2026-08-16: 2.4 migrated public/repository/settlement/download/cleanup/MVP fixtures to Vertex, reduced two-provider expectations to one, and moved the shared bounded HTTP client helper out of the deleted Gemini file. Unit-tag batch/Vertex tests pass; integration-tag repository tests compile but local execution stops in TestMain because rootless Docker is unavailable.
- 2026-08-16: 3.1 removed the explicit `gemini_api` frontend request option while retaining `string` decoding for old/unknown responses. 3.2 now documents Vertex-only support, the currently unregistered public routes, and the separation from ordinary Gemini/Antigravity gateway traffic; generic batch configuration remains unchanged.
- 2026-08-16: 3.3 found no Gemini Batch-only OpenAPI or deployment-example contract. Remaining `GEMINI_API_KEY` documentation belongs to the ordinary Gemini-compatible client gateway, not batch supply; all 62 generic `batch_image.*` config references remain intact.
- 2026-08-16: 4.1 targeted batch/Vertex unit tests pass with `-race`. Integration-tag repository tests compile but cannot execute because local testcontainers cannot find rootless Docker. The full unit-tag service run has unrelated existing failures in quota/subscription/auth dirty WIP; no batch-image test remains in its failure list.
- 2026-08-16: 4.2 archtest, both strict OpenSpec validations, Gemini Batch symbol/constant residual scans, `git diff --check`, backend build, frontend `vue-tsc`, and Vite production build pass. Remaining `gemini_api` text is limited to the explicit rejection tests and retirement documentation.
- 2026-08-16: 4.3 remains unchecked because the user explicitly directed this rollout to the US production host instead of staging; production success is not relabeled as staging evidence.
- 2026-08-16: 4.4 immediately rechecked US production before cutover: `batch_image_jobs`, `batch_image_items`, and `batch_image_events` were all zero, so the financial lifecycle gate passed without data mutation.
- 2026-08-16: 4.5 production backend-only rollout is in observation, not complete. API `0.1.322` is live on port 18247 with frontend `0.1.321` unchanged; API `0.1.321` remains healthy on 18246 as rollback. Public health/version and unauthenticated HTTP/SSE/WS contracts passed. The first five-minute aggregate contained 220 usage rows (220 stream, 134 WS) with normal aggregate cost persistence and no candidate panic/database/Redis/billing severe logs. Keep 4.5 unchecked until the observation window is accepted.
