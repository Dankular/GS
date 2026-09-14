# ADR-013: Player matchmaking membership boundary

## Status

Accepted

## Decision

The public player HTTP endpoint and player command path accept only a solo
ticket whose sole member is the authenticated actor. Multi-player tickets are
reserved for a trusted Nakama/service adapter that proves party membership.

## Context

Ticket records support multiple members for matchmaking and future party
integration, but a client-provided array of Nakama IDs is not an authorization
proof. Accepting it directly would let one player claim another player as a
roster member and create admission/notification side effects for that player.

## Consequences

Solo matchmaking remains available immediately and existing two-player tests
still create two independent solo tickets. Party matchmaking requires the
explicit Nakama adapter boundary before it can be enabled; it cannot be
implemented safely by trusting an additional request field.
