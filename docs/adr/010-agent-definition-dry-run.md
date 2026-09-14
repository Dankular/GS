# ADR-010: Agent definition dry-run boundary

## Decision

GameService exposes `POST /v1/admin/definitions/dry-run` as the agent-facing
authoring boundary. It accepts the same declarative definition source as
validation, compiles it through the canonical compiler, and may compare it to
an existing revision using `gameId` and `revision` query parameters.

The response includes the candidate digest, a digest-level diff when a source
revision is selected, active environment revisions, and non-mutating impact
counts for running matches and queued or matching tickets. The endpoint only
requires `definition:validate`; it cannot publish, activate, rollback, or
access secrets.

## Rationale

An agent needs a bounded feedback loop before it can propose a playable mode,
but validation must not be confused with authorization to change live state.
Keeping dry-run, publication, and activation as separate routes preserves the
capability boundary in the design contract and makes the proposed change
auditable before a human or separately issued capability performs activation.

## Verification

- OpenAPI surface and generated Go client are drift-tested in CI.
- The integration test `TestDryRunImpactReportsActiveAndInFlightState` verifies
  revision comparison and live impact counts against PostgreSQL.
- The endpoint performs only `SELECT` queries after compiler validation.
