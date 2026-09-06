.DEFAULT_GOAL := verify

.PHONY: format lint test test-race build web-typecheck web-lint web-test web-build container-build secret-scan verify

format:
	go fmt ./...

lint:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build ./cmd/control-api ./cmd/orchestrator-worker ./cmd/redfish-simulator

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
