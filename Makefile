.DEFAULT_GOAL := verify

.PHONY: format lint test test-race build domain-test domain-property-test postgres-integration-test p02-fuzz-test sqlc-generate web-typecheck web-lint web-test web-build container-build secret-scan verify

format:
	go fmt ./...

lint:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

domain-test:
	go test -count=1 ./internal/domain/... ./internal/architecturetest/...

domain-property-test:
	go test -count=1 ./test/domainmodel/... -args --rapid.checks=200

postgres-integration-test:
	go test -tags=postgresintegration -count=1 ./test/integration/postgres/...

p02-fuzz-test:
	go test ./internal/domain/inventory -run '^$$' -fuzz '^FuzzSelectorCanonicalRoundTrip$$' -fuzztime=10s
	go test ./internal/redfish -run '^$$' -fuzz '^FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot$$' -fuzztime=10s
	go test ./internal/redfish -run '^$$' -fuzz '^FuzzDecodeDocumentRejectsUnsafeInputWithoutPanic$$' -fuzztime=10s

sqlc-generate:
	CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

build:
	go build ./cmd/control-api ./cmd/orchestrator-worker ./cmd/redfish-simulator ./cmd/discovery-worker ./cmd/local-object-store ./cmd/v1-p02-check

web-typecheck:
	pnpm --dir web typecheck

web-lint:
	pnpm --dir web lint

web-test:
	pnpm --dir web test

web-build:
	pnpm --dir web build

container-build:
	docker build --file deploy/docker/Dockerfile.api --tag fleetforge/control-api:local .
	docker build --file deploy/docker/Dockerfile.worker --tag fleetforge/orchestrator-worker:local .
	docker build --file deploy/docker/Dockerfile.simulator --tag fleetforge/redfish-simulator:local .

secret-scan:
	@echo "Run the pinned CI secret-scan job or install gitleaks locally."

verify: lint test test-race build web-typecheck web-lint web-test web-build
