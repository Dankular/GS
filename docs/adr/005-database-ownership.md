# ADR-005: Shared PostgreSQL cluster with isolated ownership

## Status

Accepted

## Context

A single PostgreSQL cluster is economical for the initial deployment, while
Nakama and GameService require independent write ownership and migration
lifecycles.

## Decision

GameService uses dedicated schemas, roles, migrations, and tables. Nakama
uses its supported database APIs and own role/schema. GameService never reads
or writes undocumented Nakama tables; cross-owner data uses supported APIs or
durable events.

## Alternatives considered

- Separate database clusters immediately: stronger isolation, but higher
  operational cost before scale requires it.
- Shared undocumented tables: lower short-term effort, but creates unsafe
  coupling to Nakama internals.

## Consequences

The initial cluster must still enforce role grants and schema boundaries.
Production can split the databases without changing the public contracts.

## Rollback

Move either owner to a separate database and update its connection secret;
retain event/API boundaries and do not introduce direct cross-database SQL.
