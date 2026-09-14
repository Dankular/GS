# ADR-017: OpenAPI and schemas as the API contract

## Status

Accepted

## Context

The HTTP API and generated client must not drift, while command and event
payloads need independently versioned JSON Schema contracts.

## Decision

Keep `api/openapi.yaml` and `api/schemas/` source-controlled as the external
contract. Generate the Go client from OpenAPI, validate schema fixtures and
runtime decoding in tests, and make CI fail when generation changes tracked
files. Async events are versioned in `api/asyncapi.yaml` and event schemas.

## Alternatives considered

- Code-only contract: convenient, but clients cannot review a stable source
  document and drift detection is weaker.
- Hand-maintained generated client: permits silent incompatibility.

## Consequences

An externally visible route or payload change requires synchronized contract,
implementation, generated client, fixtures, and tests. Generated files are
artifacts, not an independent design source.

## Rollback

Revert to the prior contract revision and regenerate clients; preserve
versioned routes and schemas for already deployed clients.
