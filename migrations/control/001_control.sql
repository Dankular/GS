CREATE SCHEMA IF NOT EXISTS platform;
CREATE SCHEMA IF NOT EXISTS ops;
CREATE TABLE IF NOT EXISTS platform.command_requests (
  request_id text PRIMARY KEY,
  correlation_id text NOT NULL,
  game_id text NOT NULL,
  environment text NOT NULL,
  definition_revision bigint NOT NULL CHECK (definition_revision > 0),
  actor_type text NOT NULL,
  actor_id text NOT NULL,
  operation text NOT NULL,
  status text NOT NULL,
  result jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ops.outbox_events (
  event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type text NOT NULL,
  aggregate_id text NOT NULL,
  event_type text NOT NULL,
  correlation_id text NOT NULL,
  payload jsonb NOT NULL,
  published_at timestamptz
);
