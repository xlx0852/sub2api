## 1. Baseline

- [ ] 1.1 Run existing provider stage-order and forwarding regression tests.
- [ ] 1.2 Record current stage count and account/lease invariants.

## 2. Ablation

- [ ] 2.1 Remove the redundant no-op select stage from the selected provider pipelines.
- [ ] 2.2 Update characterization tests and compare behavior/metrics.

## 3. Gates

- [ ] 3.1 `go test ./internal/...`
- [ ] 3.2 `go vet ./internal/...`
- [ ] 3.3 `staticcheck ./internal/...` with existing compatibility warning accounted for.
