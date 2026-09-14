# GameService SDKs

The OpenAPI document is the source of truth for the HTTP control-plane client.
The Go client in `generated/` is generated with `oapi-codegen` and must be
reproducible from the repository root:

```text
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.0 \
  -config sdk/generated/config.yaml api/openapi.yaml
gofmt -w sdk/generated/client.gen.go
```

CI checks that regeneration produces no diff. Generated clients do not carry
trusted authority: player and server credentials are supplied by the caller,
and mutation authorization remains enforced by the Control API.

Nakama-owned real-time parties use Nakama's native client socket API. They are
short-lived session objects, not GameService database records. The client
party flow is: `createParty`, `joinParty`, `leaveParty`, then
`partyMatchmakerAdd` for matchmaking. The Control API remains responsible for
the resulting ticket, match, claims, and result lifecycle.

