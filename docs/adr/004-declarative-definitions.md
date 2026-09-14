# ADR-004: Definitions contain no executable scripts

## Status

Accepted

## Context

Humans and LLM agents need to author game behavior without gaining SQL,
shell, network, or deployment authority.

## Decision

Definitions are schema-validated data. The compiler resolves local references,
checks semantics and policy, and emits a deterministic plan over a static,
versioned command registry. Published revisions are immutable and addressed
by digest.

## Alternatives considered

- Embedded JavaScript/Lua: expressive, but creates an unbounded execution and
  review boundary.
- Arbitrary webhooks: difficult to authorize, reproduce, and make idempotent.

## Consequences

Some game-specific behavior must be implemented as a reviewed command or
condition before it can be selected. Preview and impact analysis are safe
because compilation has no side effects.

## Rollback

Deactivate a published revision and activate a previously approved digest;
never edit a published revision in place.
