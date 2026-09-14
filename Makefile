SHELL := sh

.PHONY: bootstrap generate lint test test-integration test-e2e dev-core dev-full down migrate-up migrate-down-one seed definition-validate load-smoke

bootstrap:
	go version

generate:
	go test ./...

lint:
	gofmt -l .

test:
	go test ./...

test-integration:
	go test -tags=integration ./tests/integration/...

test-e2e:
	go test -tags=e2e ./tests/e2e/...

dev-core:
	docker compose -f deploy/compose/compose.yaml up --build

dev-full:
	./deploy/kind/install-agones.sh

down:
	docker compose -f deploy/compose/compose.yaml down

migrate-up:
	docker compose -f deploy/compose/compose.yaml run --rm migrations

migrate-down-one:
	@echo "Down migrations require an explicit migration tool and are not enabled yet."

seed:
	go run ./cmd/definition-compiler --file definitions/examples/arena.yaml

definition-validate:
	go run ./cmd/definition-compiler --file $(FILE)

load-smoke:
	go test ./tests/load/...
