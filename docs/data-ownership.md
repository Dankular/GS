# Data ownership

GameService and Nakama may share the VPS PostgreSQL cluster, but they do not
share undocumented tables. The control plane writes only the schemas listed
below; Nakama data is accessed through Nakama APIs or runtime contracts.

In Docker Compose, `gameservice_admin` is deployment-only and is used by the
bootstrap jobs; the Control API and workers use the non-superuser `gameservice`
role, while Nakama uses the separate non-superuser `nakama` role. Nakama's
role is denied usage on the GameService schemas, and GameService does not read
Nakama's public tables. Production Kubernetes deployments must provide the
same role separation through the external PostgreSQL/secret-management
system.

| Data | Authoritative owner | Access contract |
| --- | --- | --- |
| Identity, sessions, usernames, profile, friends, groups, parties, chat, notifications | Nakama | Nakama client APIs plus the authenticated `gameservice.profile` and `gameservice.social` runtime RPCs |
| Nakama storage objects, leaderboards, and tournaments | Nakama | Nakama APIs; trusted workers use the supported leaderboard server API and the narrow server-to-server tournament runtime RPC |
| Definitions, revisions, activation history | GameService | Control API and `platform` schema |
| Commands, idempotency results, audit records, outbox | GameService | Control API and `platform`/`ops` schemas |
| Wallets, currency ledger, inventory, entitlements, progression, rewards | GameService | Typed command handlers and `economy`/`progression` schemas |
| Privacy export/deletion workflow and tombstones | Shared boundary; Nakama identity APIs plus GameService-owned projections | `/v1/players/me/privacy/*`, `gameservice.privacy`, `platform.deleted_account_tombstones` |
| Player restrictions and admission policy records | GameService | Admin restriction commands and transactional queue/join checks |
| Tickets, matches, rosters, allocations, claims, results | GameService | Matchmaking/allocation adapters and `match` schema |
| Game-server process lifecycle and capacity | Agones/Kubernetes | Allocator API and Kubernetes/Agones APIs; never domain writes |

Cross-owner data is exchanged through authenticated APIs or versioned outbox
events. A service must not query another owner’s private tables, infer current
definition state during command execution, or expose its credentials to a
client or dedicated server.
