ALTER TABLE ops.outbox_events ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS outbox_events_pending_created_idx ON ops.outbox_events (created_at) WHERE published_at IS NULL AND dead_lettered_at IS NULL;
