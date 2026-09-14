# ADR-012: Bounded command inputs

## Status

Accepted

## Decision

All decoded command envelopes enforce a maximum argument nesting depth of 8,
maximum string/key length of 4096 bytes, and maximum object/array size of 128
entries. Numbers must be finite. Economy quantities, currency amounts, and XP
increments are additionally capped at 1,000,000,000 per command.

The bounds are enforced before dispatch, independently of the selected command
handler. The command envelope remains strict about unknown top-level JSON
fields and rejects trailing JSON data. Operation handlers retain their own
semantic validation and definition-specific limits.

## Context

The contract requires explicit limits on depth, strings, collections, and
quantities. Without a shared boundary, a newly added operation could accept a
large or pathological payload before its handler had an opportunity to apply a
limit.

## Consequences

This limits parser/resource-exhaustion risk and makes the public rejection
behavior consistent across operations. Clients must split unusually large
payloads into bounded requests. The limits are source-controlled and covered
by unit tests; changing them is a compatibility/security decision.
