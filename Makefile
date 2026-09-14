SHELL := sh

.PHONY: bootstrap generate lint test test-integration test-e2e dev-core dev-full down migrate-up migrate-down-one seed definition-validate load-smoke backup verify-restore

bootstrap:
	@go version
	@command -v docker >/dev/null || (echo "docker is required" >&2; exit 1)
	@command -v docker-compose >/dev/null 2>&1 || docker compose version >/dev/null

generate:
	@go test ./api ./internal/compiler ./internal/commands
	@test -f api/openapi.yaml
	@test -f api/asyncapi.yaml

lint:
	@test -z "$$(gofmt -l .)"
	go vet ./...

test:
	go test -race ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	go test -tags=e2e ./...

dev-core:
	docker compose -f deploy/compose/compose.yaml up --build

dev-full:
	./deploy/kind/install-agones.sh

down:
	docker compose -f deploy/compose/compose.yaml down

migrate-up:
	docker compose -f deploy/compose/compose.yaml run --rm migrations

migrate-down-one:
	docker compose --env-file $${ENV_FILE:-.env} -f deploy/compose/compose.yaml run --rm migrations sh -c 'psql "$$DATABASE_URL" -f /migrations/001_control.down.sql'

seed:
	go run ./cmd/definition-compiler --file definitions/examples/arena.yaml

definition-validate:
	go run ./cmd/definition-compiler --file $(FILE)

load-smoke:
	go test ./...

simulator-build:
	docker compose -f deploy/compose/compose.yaml --profile simulator build simulator-server

backup:
	./deploy/backup/backup-postgres.sh

verify-restore:
	./deploy/backup/verify-restore.sh $(FILE)
