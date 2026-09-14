# ADR-002: PostgreSQL outbox before a broker

## Status

Accepted

## Context

Domain mutations and their externally delivered events must be atomic and
retryable. MVP load does not yet justify operating a second durable broker.

## Decision

Write outbox events in the same PostgreSQL transaction as the owning domain
mutation. Consumers use durable per-consumer checkpoints and idempotent
delivery. Keep publisher and consumer interfaces replaceable.

## Alternatives considered

- Kafka/NATS: useful at higher fan-out, but introduces another durability and
  failure domain before measured need.
- Synchronous external calls: risks partial commits and retry ambiguity.

## Consequences

PostgreSQL carries coordination and delivery backlog. Outbox lag and failed
delivery require monitoring and reconciliation; consumers must never assume
exactly-once transport.

## Rollback

Introduce a broker publisher behind the existing outbox consumer interface;
retain the PostgreSQL outbox as the transactional source until migration and
replay evidence are complete.
