# GameService

GameService is a self-hosted declarative game backend designed to complement
GNS.NET. The implementation follows `codex(7).md`: Nakama owns identity and
social capabilities, GameService owns domain state, PostgreSQL is the durable
coordination substrate, and Agones owns dedicated-server lifecycle.

## Current status

The repository contains the Phase 0 foundation and a tested control API
vertical slice. The API currently validates command envelopes and provides
idempotent command results in process memory. PostgreSQL persistence,
Nakama/Agones integration, and production deployment are intentionally tracked
as incomplete until their real integration tests run.

## Development

Requirements: Go 1.25+, Docker Desktop for `make dev-core`, and Kind or
Minikube plus Helm for `make dev-full`.

```text
go test ./...
go run ./cmd/control-api
```

The API listens on `:8080` by default. Health endpoints are available at
`/health/live` and `/health/ready`; commands are posted to `/v1/commands`.

Pinned candidate images are recorded in `deploy/compose/compose.yaml` and must
be resolved to immutable digests by the compatibility smoke test before a
production baseline is declared.
