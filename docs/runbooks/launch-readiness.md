# Launch-readiness and disaster recovery

This runbook records the initial recovery hypotheses from
[`ADR-015`](../adr/015-rpo-rto-targets.md). They are acceptance criteria for a
production deployment, not evidence that the current VPS meets them.

## Recovery objectives

| Service/data | Target | Required evidence |
| --- | --- | --- |
| GameService durable state | RPO ≤ 15 minutes | Off-site encrypted backup cadence and checksum history |
| Control plane and database | RTO ≤ 60 minutes | Timed isolated restore and application smoke test |
| Definitions, audit, and metadata | Same as database | Off-site artifact backup and digest verification |

## Regional evacuation

1. Declare the incident and freeze definition activation and high-value admin
   mutations.
2. Preserve current match, audit, outbox, and allocator evidence; do not run
   rollback migrations against the source database.
3. Promote the approved PostgreSQL standby or restore the newest verified
   backup in the target region.
4. Reconcile projections, outbox checkpoints, definitions, and active-match
   state before opening the gateway.
5. Deploy digest-pinned application and server images, then run the synthetic
   match and compare the observed outage, RPO, and RTO with ADR-015.

## Credential and certificate rotation

1. Issue replacement secrets through the approved external secret manager.
2. Add new verification keys while retaining the old key for the overlap
   window; rotate signing and allocator client credentials separately.
3. Roll workloads with readiness checks and verify claims, Nakama sessions,
   and allocator mTLS before removing the old key.
4. Record the actor, reason, key IDs, overlap window, and deployment digest in
   the audit system. Never copy private key material into logs or tickets.

## Allocator loss or capacity exhaustion

Keep tickets queued with bounded allocation retries. Alert on allocation
failures and ready-capacity exhaustion, inspect Agones allocator health and
Fleet capacity, and only fail stale allocations through reconciliation. Do
not grant rewards or mark a match complete from allocator state alone.

## Failed migration or application rollout

Stop promotion, preserve the failed image digest and migration logs, and
confirm whether the migration committed. Use a forward-compatible application
rollback or the documented database restore procedure; never run a down
migration against production data unless an approved recovery plan explicitly
requires it. Re-run migration and synthetic-match checks in isolation before
retrying promotion.

## Current evidence boundary

The configured VPS proves Docker Compose operation, pinned Kind/Agones
compatibility, backup/restore script behavior, and synthetic match behavior.
It does not prove multi-region evacuation, managed PostgreSQL PITR, external
secret-manager recovery, or the RPO/RTO targets above.
