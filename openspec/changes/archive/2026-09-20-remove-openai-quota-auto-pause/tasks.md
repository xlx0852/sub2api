## 1. Inventory and baseline

- [x] 1.1 Record all global and per-account references and existing tests.
- [x] 1.2 Run focused scheduler/settings/account/form regression tests.

## 2. Remove implementation

- [x] 2.1 Remove global settings fields, cache/context plumbing, and API serialization.
- [x] 2.2 Remove per-account form fields, payload handling, and localization.
- [x] 2.3 Remove OpenAI quota auto-pause scheduler and sticky-path enforcement.
- [x] 2.4 Update tests and preserve expiry/rate-limit/Grok behavior.

## 3. Gates

- [x] 3.1 `go test ./internal/...`
- [x] 3.2 `go vet ./internal/...`
- [x] 3.3 `staticcheck ./internal/...`
- [x] 3.4 Frontend ESLint, typecheck, and Vitest
- [x] 3.5 `openspec validate remove-openai-quota-auto-pause --strict`
