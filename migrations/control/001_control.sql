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

CREATE SCHEMA IF NOT EXISTS economy;
CREATE TABLE IF NOT EXISTS economy.wallet_accounts (
  player_id text NOT NULL,
  currency text NOT NULL,
  balance bigint NOT NULL CHECK (balance >= 0),
  PRIMARY KEY (player_id, currency)
);
CREATE TABLE IF NOT EXISTS economy.ledger_transactions (
  request_id text PRIMARY KEY,
  reason text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS economy.ledger_entries (
  entry_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  request_id text NOT NULL REFERENCES economy.ledger_transactions(request_id),
  player_id text NOT NULL,
  currency text NOT NULL,
  amount bigint NOT NULL CHECK (amount <> 0)
);
CREATE TABLE IF NOT EXISTS economy.inventory_stacks (
  player_id text NOT NULL,
  item_id text NOT NULL,
  quantity bigint NOT NULL CHECK (quantity >= 0),
  version bigint NOT NULL CHECK (version > 0),
  PRIMARY KEY (player_id, item_id)
);
