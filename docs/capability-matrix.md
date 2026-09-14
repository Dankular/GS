# Capability matrix

This matrix records the supported GameService boundary and its owner. Nakama
features remain accessed through Nakama client APIs or the narrow runtime RPC;
the Control API never reads Nakama tables.

| Capability | Owner | Supported surface | Verification |
|---|---|---|---|
| Authentication and sessions | Nakama | Device/session JWT; Control API verifies the session | VPS authenticated profile command |
| Profile | Nakama | `gameservice.profile`; `profile.get`, `profile.patch_public_fields` | Runtime smoke and RPC unit test |
| Friends, groups, notifications, chat/moderation | Nakama | `gameservice.social` bounded RPC plus `ChannelMessageSend` before-hook; optional `GAMESERVICE_MODERATION_BLOCKLIST` | Runtime registration test; deterministic moderation policy path |
| Economy and progression | GameService | Wallet, ledger, inventory, entitlement, progression, reward commands | Go unit/integration suites |
| Definitions | GameService | Validate, non-mutating dry-run/impact, diff, publish, two-actor production approval, activate, rollback | VPS definition integration suite; dry-run and approval PostgreSQL integration tests |
| Matchmaking and admission | GameService + Agones | Queue/status/cancel, bounded allocation retries, stale-allocation recovery, join claims, restrictions | Go integration suite; Kind/Agones smoke |
| Results, leaderboards, and tournaments | GameService + Nakama | Authoritative result finalization; outbox leaderboard delivery; declarative authoritative tournament delivery through Nakama runtime | VPS result/leaderboard coverage; tournament runtime smoke |
| Economic reconciliation | GameService | Read-only projection-vs-ledger worker | VPS one-shot reconciliation: passed |
| Backup and restore | Operations | Custom-format dump, checksum, isolated restore verifier | VPS restore: 21 application tables |

Parties remain native Nakama client features documented in `sdk/examples/party.md`;
GameService does not duplicate party membership. Moderation policy enforcement
and production HA services remain integration work rather than being claimed as
completed by this matrix. The generated Go Control API client
is implemented and drift-checked in CI; Nakama's native client SDKs remain the
client integration surface for identity and social features.
