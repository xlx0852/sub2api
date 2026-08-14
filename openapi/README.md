# Control-plane OpenAPI

`control-plane.yaml` is the spec-first contract for authentication, end-user, and payment HTTP APIs.

- Gateway protocol endpoints (`/v1/messages`, `/v1/chat/completions`, Codex, Gemini, Antigravity, …) stay on upstream semantics and are **not** in this file.
- Admin APIs are added in later batches.
- This first slice locks **path + method** coverage. Request/response field schemas stay envelope-level until the TypeScript client is generated.

Canonical public runtime config is `GET /api/v1/public/config`. `GET /api/v1/settings/public` remains as a compatibility alias. Separated deploys also call `GET /api/v1/public/version` for frontend/backend skew checks.

CI: `go test ./internal/server -run TestControlPlaneOpenAPIMatchesRoutes` fails if a scoped route is added or removed without updating the spec.

Frontend client (7.2):

- Generate path/method types: `cd frontend && npm run generate:api`
- Check committed types: `cd frontend && npm run check:api`
- Feature modules call `controlPlaneRequest` so path + method stay aligned with this spec. Axios interceptors (401 refresh, envelope unwrap) stay in `src/api/client.ts`.
- Domain request/response types stay in the feature wrappers until field-level schemas land.
- Admin API modules stay handwritten until the admin spec batch.
