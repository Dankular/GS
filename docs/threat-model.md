# Threat model

## Scope and assets

The model covers the public client/admin edge, Nakama, the GameService control
plane and workers, Agones-hosted dedicated servers, PostgreSQL, backups, and
the VPS Docker host. Protected assets are player identity, session material,
wallet/inventory state, authoritative results, definition history, signing
keys, database credentials, and audit records.

## Trust boundaries

- Clients, LLM proposals, and public admin traffic are untrusted.
- The edge may route and rate-limit but has no domain-write authority.
- Nakama owns identity/social state; GameService owns domain state.
- Dedicated servers are trusted only for their short-lived assigned match and
  build; they cannot grant value or write arbitrary matches.
- Agones/Kubernetes and PostgreSQL are critical infrastructure boundaries.

## Primary threats and controls

| Threat | Control | Verification |
| --- | --- | --- |
| Forged/expired player session | Verify Nakama HS256 signature, issuer/audience, expiry, token type; derive actor from claims | `internal/auth` tests |
| Actor substitution or cross-player reads | Reject body actor mismatch and scope command-result reads to owner/admin scope | Control API and commandstore integration tests |
| Replay or duplicate mutation | ULID request idempotency, transactional result/outbox, unique reward/request keys | Command/economy integration tests |
| Forged server result or join claim | Ed25519 signature, match/allocation/build/time binding, key-ring rotation | `internal/matches` tests |
| Economy inflation or unbalanced transfer | PostgreSQL transaction, bounds/stack checks, balanced ledger entries, advisory locks for once-only rewards | Economy unit and VPS PostgreSQL tests |
| Malicious definition/LLM proposal | Strict schema, semantic compiler, closed operation registry, no scripts/network references | Compiler/schema tests |
| Worker crash or duplicate external delivery | Leases, bounded retries, consumer checkpoints, dead-letter state | Outbox tests |
| Database loss or secret exposure | Digest-pinned containers, restricted `.env`, backup checksum/isolated restore procedure | VPS backup verification and deployment audit |
| Game-server compromise | Match-scoped claims, non-root/read-only containers, no database/Kubernetes credentials | Kind simulator manifest and claim tests |

Remaining production controls include an external secret manager, HA/PITR
PostgreSQL, ingress/WAF policy, vulnerability/SBOM/signature gates, alerting,
and recurring chaos/load exercises. The single-host VPS is not a production
security boundary until those controls are deployed and evidenced.
