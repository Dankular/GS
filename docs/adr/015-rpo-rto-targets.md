# ADR-015: Initial recovery objectives

## Status

Accepted for launch planning; production approval pending evidence

## Context

The platform needs measurable recovery objectives before selecting backup
frequency, retention, alerting, and evacuation procedures. The current VPS is
a single-host validation environment and cannot demonstrate these objectives.

## Decision

Use the following initial launch hypotheses for the production service:

- RPO: at most 15 minutes for durable GameService state.
- RTO: at most 60 minutes for restoration of the control plane and database.
- Definition source, compiled artifacts, audit records, and backup checksums
  are retained off-site with the database backup.

These targets must be revised from measured restore and evacuation drills
before launch approval.

## Alternatives considered

- No declared target: avoids commitment but prevents backup frequency and
  incident acceptance criteria from being evaluated.
- Near-zero RPO/RTO: requires managed HA PostgreSQL, synchronous replication,
  and multi-region operations not justified by the current deployment.

## Consequences

Encrypted off-site backups must run more frequently than the RPO target, and
restore drills must complete within the RTO target. A single Compose volume,
local backup, or single Kind node cannot satisfy this ADR by itself.

## Rollback

Revise the targets through a new ADR after measured workload, retention, and
incident-response evidence; keep historical decisions intact.
