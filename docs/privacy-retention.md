# Privacy, export, and deletion

GameService exposes self-service endpoints for an authenticated Nakama
account: `POST /v1/players/me/privacy/export` and
`POST /v1/players/me/privacy/delete`. Both require an `Idempotency-Key`.

Export returns the Nakama account export and the GameService wallet/inventory
snapshot. The Nakama portion is obtained through the server-only
`gameservice.privacy` runtime RPC; GameService never reads Nakama-owned tables.

Deletion is an auditable workflow. Nakama deletes the identity through its
supported `accountDeleteId` API, then GameService transactionally removes
player projections, active match membership and restrictions, scrubs player
IDs from retained result payloads, pseudonymizes immutable ledger/audit actor
references, and writes a stable hash tombstone. A retry can finalize the
GameService transaction after an interruption because an already-absent
Nakama account is treated as an idempotent delete.

| Data class | Policy | Owner |
| --- | --- | --- |
| Authentication, profile, social content | Nakama account lifecycle | Nakama |
| Wallets, inventory, progression, restrictions | Removed during deletion | GameService |
| Economy ledger and privileged audit evidence | Retained for accounting/fraud obligations; identity replaced with a tombstone | GameService |
| Match results | Retained for reconciliation; deleted player IDs removed from result player arrays | GameService |
| Deletion tombstones | Retained for deduplication and fraud prevention; hash and request metadata only | GameService |
| Export request records | Retained for idempotent replay; jurisdiction-specific expiry is required at launch | GameService |

The workflow is implemented and auditable. Production launch still requires
an approved jurisdiction-specific expiry job and legal retention schedule for
export records.
