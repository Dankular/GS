# ADR-018: Kind for the local Agones validation cluster

## Status

Accepted

## Context

Development needs a reproducible Kubernetes environment with real Agones
allocation behavior while keeping the workstation workflow Docker-first.

## Decision

Use Kind with a pinned Kubernetes node image and pinned Agones Helm chart for
the full local profile. Docker Compose remains the core-service runtime and
the Kind node is connected to the Compose network through the documented
gateway/allocator setup. A deterministic fake allocator remains the unit/API
boundary for tests that do not need Kubernetes.

## Alternatives considered

- Minikube: viable, but the current Docker/Windows workflow and CI evidence
  are stronger for Kind.
- Compose-only Agones emulation: fast, but cannot prove CRD, SDK, Fleet, or
  Allocator compatibility.

## Consequences

The full profile requires Docker, Kind, kubectl, and Helm and is not an HA
claim. The pinned compatibility baseline must be updated when any of these
components change.

## Rollback

Replace the installer behind the same Fleet and allocator contracts after a
documented compatibility comparison; retain the fake allocator for isolated
tests.
