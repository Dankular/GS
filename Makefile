SHELL := sh

.PHONY: bootstrap generate lint test test-integration test-e2e test-load test-chaos fuzz-smoke dev-core dev-full down migrate-up migrate-down-one migrate-cycle seed definition-validate load-smoke backup verify-restore

bootstrap:
	@go version
	@command -v docker >/dev/null || (echo "docker is required" >&2; exit 1)
	@command -v docker-compose >/dev/null 2>&1 || docker compose version >/dev/null

generate:
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.0 -config sdk/generated/config.yaml api/openapi.yaml
	@gofmt -w sdk/generated/client.gen.go
	@git diff --exit-code -- api/openapi.yaml sdk/generated/client.gen.go
	@go test ./api ./internal/compiler ./internal/commands ./sdk/generated
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

migrate-cycle:
	./deploy/compose/verify-migrations.sh

seed:
	docker compose --env-file $${ENV_FILE:-.env} -f deploy/compose/compose.yaml --profile seed run --rm seed

definition-validate:
	go run ./cmd/definition-compiler --file $(FILE)

test-load:
	go test -tags=load ./tests/load

test-chaos:
	go test -tags=chaos ./tests/chaos

fuzz-smoke:
	go test ./internal/commands -fuzz=FuzzDecodeStrictNeverPanics -fuzztime=3s

load-smoke: test-load

simulator-build:
	docker compose -f deploy/compose/compose.yaml --profile simulator build simulator-server

backup:
	./deploy/backup/backup-postgres.sh

verify-restore:
	./deploy/backup/verify-restore.sh $(FILE)
