# ADR-009: Agones allocation bootstrap metadata

## Context

A ready dedicated server is selected before a match exists. The allocator must
therefore carry the match ID, allocation ID, server build, and roster into the
server without giving the server Kubernetes API credentials or trusting client
input.

## Decision

The matchmaking worker generates the match and allocation IDs, sends the
assignment as Agones allocation metadata annotations, and records the same
allocation ID in the match record. The simulator reads those annotations
through the Agones SDK and applies them after reporting Ready. Join claims still
require the control-plane signature, match ID, allocation ID, build, subject,
and slot, so annotations alone never authorize a player.

The allocator adapter supports both gRPC and REST metadata and preserves the
same contract for the fake allocator used by unit tests.

## Consequences

Game-server images remain free of Kubernetes API credentials and the control
plane remains the authority for claims and results. A server can be allocated
before its match-specific configuration is known. The current Kind simulator
uses this dynamic path; production images must implement the same bootstrap
contract before production rollout.
