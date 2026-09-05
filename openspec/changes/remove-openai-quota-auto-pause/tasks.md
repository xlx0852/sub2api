## 1. Inventory and baseline

- [ ] 1.1 Record all global and per-account references and existing tests.
- [ ] 1.2 Run focused scheduler/settings/account/form regression tests.

## 2. Remove implementation

- [ ] 2.1 Remove global settings fields, cache/context plumbing, and API serialization.
- [ ] 2.2 Remove per-account form fields, payload handling, and localization.
- [ ] 2.3 Remove OpenAI quota auto-pause scheduler and sticky-path enforcement.
- [ ] 2.4 Update tests and preserve expiry/rate-limit/Grok behavior.

## 3. Gates

- [ ] 3.1 `go test ./internal/...`
- [ ] 3.2 `go vet ./internal/...`
- [ ] 3.3 `staticcheck ./internal/...`
- [ ] 3.4 Frontend ESLint, typecheck, and Vitest
- [ ] 3.5 `openspec validate remove-openai-quota-auto-pause --strict`
