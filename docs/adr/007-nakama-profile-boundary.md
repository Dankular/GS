# ADR-007: Nakama-owned profile boundary

## Context

The Codex assigns identity and profile ownership to Nakama and forbids the
control plane from reading Nakama’s private tables. Clients still need a
stable GameService-adjacent profile operation for the GNS.NET integration.

## Decision

Implement `gameservice.profile` as a Nakama JavaScript runtime RPC. It derives
the user from `ctx.userId`, supports `get` and `patch_public_fields`, limits
patchable fields to `username`, `displayName`, `langTag`, and `avatarUrl`, and
uses Nakama’s supported `accountGetId`/`accountUpdateId` APIs. It rejects
unauthenticated calls, malformed JSON, unknown fields, and strings longer than
128 characters. It never calls PostgreSQL or the GameService command handler.

## Consequences

Nakama remains the sole owner of profile data and its permission model. The
runtime is ES5-compatible and works with the pinned Nakama 3.40.0 image. The
module load and authenticated get/patch RPCs were verified on the configured
VPS; client SDK generation and broader social RPC examples remain separate
work.
