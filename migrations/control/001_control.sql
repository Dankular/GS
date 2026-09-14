CREATE SCHEMA IF NOT EXISTS game_control;
CREATE TABLE IF NOT EXISTS game_control.idempotency_records (
  request_id text PRIMARY KEY,
  correlation_id text NOT NULL,
  operation text NOT NULL,
  status text NOT NULL,
  result jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS game_control.outbox_events (
  event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type text NOT NULL,
  aggregate_id text NOT NULL,
  event_type text NOT NULL,
  correlation_id text NOT NULL,
  payload jsonb NOT NULL,
  published_at timestamptz
);
