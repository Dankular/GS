# ADR-011: Two-actor production definition approval

## Decision

Production definition activation and rollback require an approval record whose
game, environment, revision, and digest match the requested operation. The
approval endpoint is scoped separately as `definition:approve`. The first
actor creates a pending approval; a different actor approves it. Activation
and rollback accept that approval ID through `X-Approval-Id` and reject
self-approval, stale approvals, and digest mismatches.

Development and staging environments do not require this production-only
step-up. All approval requests, approvals, and subsequent activation actions
are written to the append-only audit chain.

## Rationale

Agent and admin credentials can validate and propose content without gaining a
single-actor path to production. Binding approval to the immutable revision
digest prevents approval reuse after a definition changes.
