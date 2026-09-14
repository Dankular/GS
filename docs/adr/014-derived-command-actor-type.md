# ADR-014: Derive command actor type from scope

## Status

Accepted

## Decision

The HTTP command endpoint requires the submitted actor ID to match the
authenticated Nakama identity and requires the actor type to match the
operation class. Player-scoped operations use `player`; definition and admin
operations use `admin`. Dedicated-server mutations use their separate
server-claim endpoints and cannot be reached by relabeling a player command.

## Context

Actor type is part of the command envelope for auditing and dispatch, but it
is not authentication evidence. Trusting the caller's type would allow a
player token to label itself as a service or admin actor and bypass target
binding in domain handlers.

## Consequences

The public command path has one unambiguous identity boundary. Server and
Nakama adapter flows must use their dedicated authenticated endpoints or a
future explicitly scoped service gateway; adding a new actor class requires a
new authentication path and tests.
