# Outbox delivery

Use this runbook when the outbox backlog is old or dead-lettered events are
present.

1. Inspect `/metrics` for `gameservice_outbox_backlog_depth`,
   `gameservice_outbox_oldest_age_seconds`, `gameservice_outbox_attempts`, and
   `gameservice_outbox_dead_letters`.
2. Check the relevant worker logs and database connectivity. Restart only the
   affected worker after preserving its logs and current event IDs.
3. For dead letters, inspect `ops.outbox_events.last_error` and payload schema,
   fix the consumer or external dependency, then replay only through an
   approved operator procedure. Never delete dead-lettered events to clear an
   alert.
4. Verify consumer checkpoints and downstream idempotency before replaying.
5. Run a synthetic match and confirm the backlog returns to normal.

Event payloads and identifiers must not be copied into public tickets or logs
when they contain player-related data.
