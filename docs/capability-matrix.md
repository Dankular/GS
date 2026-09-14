# Capability matrix

This matrix records the supported GameService boundary and its owner. Nakama
features remain accessed through Nakama client APIs or the narrow runtime RPC;
the Control API never reads Nakama tables.

| Capability | Owner | Supported surface | Verification |
|---|---|---|---|
| Authentication and sessions | Nakama | Device/session JWT; Control API verifies the session | VPS authenticated profile command |
| Profile | Nakama | `gameservice.profile`; `profile.get`, `profile.patch_public_fields` | Runtime smoke and RPC unit test |
| Friends, groups, notifications, chat | Nakama | `gameservice.social` bounded RPC (`friends.*`, `group.*`, `groups.mine`, `notifications.list`, `chat.send`); native Nakama APIs | VPS `friends.list` smoke; runtime registration test |
| Economy and progression | GameService | Wallet, ledger, inventory, entitlement, progression, reward commands | Go unit/integration suites |
| Definitions | GameService | Validate, diff, publish, activate, rollback | VPS definition integration suite |
| Matchmaking and admission | GameService + Agones | Queue/status/cancel, allocator worker, join claims, restrictions | Go integration suite; Kind/Agones smoke |
| Results and leaderboards | GameService + Nakama | Authoritative result finalization and outbox leaderboard delivery | VPS result and leaderboard integration coverage |
| Economic reconciliation | GameService | Read-only projection-vs-ledger worker | VPS one-shot reconciliation: passed |
| Backup and restore | Operations | Custom-format dump, checksum, isolated restore verifier | VPS restore: 21 application tables |

Parties remain native Nakama client features documented in `sdk/examples/party.md`;
GameService does not duplicate party membership. Tournaments, moderation policy
enforcement, and production HA services remain integration work rather than
being claimed as completed by this matrix. The generated Go Control API client
is implemented and drift-checked in CI; Nakama's native client SDKs remain the
client integration surface for identity and social features.
