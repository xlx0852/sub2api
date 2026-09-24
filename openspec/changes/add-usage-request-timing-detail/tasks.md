# Tasks

## Backend

- [x] 1.1 Add `FirstOutputStages` to usage DTOs and mapper for user/admin usage responses.
- [x] 1.2 Add mapper regression coverage for populated and historical-missing stage data.

## Frontend

- [x] 2.1 Add typed `FirstOutputStages` data to frontend usage types.
- [x] 2.2 Add request timing detail modal with summary, paths, stages, tokens/cache and mapping data.
- [x] 2.3 Add usage-table detail entry without breaking copy behavior, filters, pagination, or export.
- [x] 2.4 Add Chinese/English labels and targeted usage coverage.

## Verification

- [x] 3.1 Backend DTO/mapper tests and `go build ./...`.
- [x] 3.2 Frontend `vue-tsc --noEmit` and targeted usage tests（29 tests passed）.
- [x] 3.3 `openspec validate add-usage-request-timing-detail --strict`.
