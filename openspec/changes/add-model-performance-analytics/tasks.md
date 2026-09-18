## 1. Backend Contract and Aggregation

- [x] 1.1 Add domain types for performance KPI summaries, metric coverage, request points, comparison windows, and query filters.
- [x] 1.2 Implement raw usage aggregation for request totals, success/failure counts, TTFT, duration, input context, and derived generation TPS.
- [x] 1.3 Implement bounded request-point retrieval with model/platform/group/request-type filters and failure markers.
- [x] 1.4 Implement same-length comparison-window aggregation and explicit wave-window support.
- [x] 1.5 Add admin handler, route registration, request validation, and OpenAPI/control-plane contract updates if required by the route inventory.

## 2. Frontend Performance Analysis

- [x] 2.1 Add typed admin API client methods for the performance endpoint.
- [x] 2.2 Add a performance section to the existing Ops dashboard with KPI cards and metric switching.
- [x] 2.3 Add request-time scatter/trend chart, failure markers, context-size summary, and comparison view.
- [x] 2.4 Add responsive states for loading, empty data, partial metric coverage, and query errors.
- [x] 2.5 Add Chinese and English translations and preserve existing Ops navigation/filters.

## 3. Verification

- [x] 3.1 Add backend unit/repository/handler tests for formulas, filters, null handling, comparison windows, and point caps.
- [x] 3.2 Add frontend component/API tests for KPI rendering, metric switching, failure markers, and empty/partial data.
- [x] 3.3 Run targeted backend tests and frontend tests, typecheck, and production build.
- [x] 3.4 Cross-check metric formulas against the existing Ops overview/OpenAI token-stat fields with unit tests; live seeded-data comparison remains a deployment follow-up.
