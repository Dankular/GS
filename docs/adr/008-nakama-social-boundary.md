# ADR-008: Nakama-owned social boundary

## Decision

Expose a small authenticated `gameservice.social` Nakama runtime RPC for
friend, group membership, notification-list, and channel-message operations. The RPC
derives the caller from `ctx.userId`, bounds request sizes, and calls only
documented Nakama runtime APIs. It does not read Nakama tables or forward
arbitrary method names.

## Consequences

Nakama remains the authoritative owner of social state, leaderboards, and
tournaments, as well as permission checks.
GameService clients can use a stable operation envelope without receiving
Nakama server credentials. Supported operations include `friends.*`,
`group.*`, `groups.mine`, `notifications.list`, and `chat.send`. Parties remain
native Nakama client capabilities. Tournament creation and authoritative record
delivery use the separate, server-only `gameservice.tournament_record` runtime
RPC; moderation policy hooks remain a follow-up capability.
