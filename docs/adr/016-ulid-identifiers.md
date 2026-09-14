# ADR-016: ULIDs for externally referenced identifiers

## Status

Accepted

## Context

Requests, matches, tickets, allocations, and audit records need identifiers
that are safe to generate at the edge, sortable for operational inspection,
and non-sequential to clients.

## Decision

Use canonical ULIDs for caller-generated request IDs and time-ordered domain
IDs where the public contract requires an opaque identifier. Validate length
and syntax at the boundary; use PostgreSQL UUIDs for database-generated keys
where the identifier is not part of the client contract.

## Alternatives considered

- UUIDv4 everywhere: collision-safe but less useful for ordered operational
  scans and index locality.
- Integer sequences: ordered but expose volume and create a centralized
  allocation dependency.

## Consequences

Idempotency keys can be generated before a request is sent, and recent rows
are easy to inspect by creation order without exposing database sequences.
Indexes and logs must still treat IDs as bounded strings and never use them as
metric labels.

## Rollback

Accept UUIDs at a versioned API boundary and migrate individual tables with a
dual-read/dual-write period; never reinterpret an existing identifier.
