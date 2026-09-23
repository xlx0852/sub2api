.PHONY: release-bundle build generate test test-unit test-unit-race test-integration test-settlement test-e2e

VERSION ?= $(shell ./scripts/resolve-version.sh)
MIN_FRONTEND_VERSION ?= $(shell tr -d '\r\n' < cmd/server/MIN_FRONTEND_VERSION)
LDFLAGS ?= -s -w -X main.Version=$(VERSION) -X main.MinFrontendVersion=$(MIN_FRONTEND_VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/server ./cmd/server

generate:
	go generate ./ent
	go generate ./cmd/server

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

# Race coverage for the money-path packages. Expand to handler/service only after
# those packages are race-clean; a red first CI run would hide real regressions.
test-unit-race:
	go test -tags=unit -race -count=1 -timeout 8m ./internal/app/billing ./internal/repository

# Real-Postgres settlement tests. Requires SUB2API_SETTLEMENT_TEST_POSTGRES_DSN
# pointing at a loopback database. CI starts a dedicated service for this.
test-settlement:
	go test -count=1 -timeout 8m -p 1 ./internal/repository ./internal/app/billing -run 'UsageSettlement|DurableSettlementPostgres'

test-integration:
	go test -tags=integration ./...

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...

# For hand-run/systemd/bind-mounted backend deployments, ship resources beside
# the binary. Never export catalog.fallback_pricing as a LiteLLM pricing file:
# alias_of entries must not silently disappear during parsing.
RELEASE_ARCHIVE ?= bin/sub2api-$(VERSION).tar.gz
release-bundle: build
	python3 scripts/package-release.py --binary bin/server --output "$(RELEASE_ARCHIVE)"
