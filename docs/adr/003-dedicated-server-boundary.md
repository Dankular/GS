# ADR-003: Dedicated servers external to Nakama matches

## Status

Accepted

## Context

Realtime authoritative simulation has different CPU, network, lifecycle, and
scaling requirements from identity, social, and persistence services.

## Decision

Nakama owns identity and supported social/matchmaking capabilities; Agones
allocates dedicated server processes; GameService owns durable match state,
claims, results, and rewards. Servers report facts and authoritative results
but never apply durable economy mutations.

## Alternatives considered

- Nakama authoritative matches: simpler initial wiring, but couples realtime
  fleet lifecycle and product domain state.
- Control-plane simulation: violates the separation of request handling and
  moment-to-moment game simulation.

## Consequences

The match ID and result contract survive pod replacement. Allocation and
server admission require explicit scoped credentials and build binding.

## Rollback

Swap the Agones adapter or server runtime while preserving match, claim, and
result contracts; do not move durable ownership into the server process.
