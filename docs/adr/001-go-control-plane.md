# ADR-001: Go control plane

## Status

Accepted

## Context

GameService needs one typed implementation language for the HTTP control API,
definition compiler, PostgreSQL workers, and Agones integration. These
components must remain independently testable and deployable.

## Decision

Implement the control plane and its workers in Go. Keep domain packages
transport-independent; HTTP, PostgreSQL, Nakama, and Agones are adapters at
the process boundaries.

## Alternatives considered

- TypeScript: strong Nakama ecosystem, but weaker fit for compact static
  worker binaries and Kubernetes/Agones concurrency.
- Java/.NET: capable, but adds runtime and operational diversity without a
  demonstrated requirement.

## Consequences

The repository has one compiler/toolchain baseline and small distroless
runtime images. Nakama runtime code remains JavaScript because that is the
supported embedded runtime boundary.

## Rollback

Replace an individual adapter or worker behind the same versioned API and
event contracts; do not couple the domain model to Go-specific transport
types.
