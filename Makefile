.DEFAULT_GOAL := verify

.PHONY: format lint test test-race build domain-test domain-property-test postgres-integration-test redfish-fuzz-test sqlc-generate openapi-lint openapi-generate openapi-generate-check web-typecheck web-lint web-test web-build container-build secret-scan verify

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

redfish-fuzz-test:
	go test ./internal/domain/inventory -run '^$$' -fuzz '^FuzzSelectorCanonicalRoundTrip$$' -fuzztime=10s
	go test ./internal/redfish -run '^$$' -fuzz '^FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot$$' -fuzztime=10s
	go test ./internal/redfish -run '^$$' -fuzz '^FuzzDecodeDocumentRejectsUnsafeInputWithoutPanic$$' -fuzztime=10s

sqlc-generate:
	CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

openapi-lint:
	pnpm --dir web exec spectral lint --ruleset ../api/openapi/.spectral.yaml ../api/openapi/openapi.yaml

openapi-generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 --config api/openapi/oapi-codegen.yaml api/openapi/openapi.yaml

openapi-generate-check: openapi-generate
	git diff --exit-code -- internal/delivery/controlhttp/openapi_types.gen.go

build:
	go build ./cmd/control-api ./cmd/orchestrator-worker ./cmd/redfish-simulator ./cmd/discovery-worker ./cmd/local-object-store ./cmd/database-readiness-check ./cmd/wakeup-sweeper ./cmd/wakeup-worker ./cmd/execution-worker ./cmd/precheck-worker ./cmd/observation-worker ./cmd/reconciliation-worker

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
