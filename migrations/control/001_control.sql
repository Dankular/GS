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
  published_at timestamptz,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  leased_until timestamptz,
  last_error text,
  dead_lettered_at timestamptz
);
ALTER TABLE ops.outbox_events ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE ops.outbox_events ADD COLUMN IF NOT EXISTS leased_until timestamptz;
ALTER TABLE ops.outbox_events ADD COLUMN IF NOT EXISTS last_error text;
ALTER TABLE ops.outbox_events ADD COLUMN IF NOT EXISTS dead_lettered_at timestamptz;

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

CREATE SCHEMA IF NOT EXISTS match;
CREATE TABLE IF NOT EXISTS match.tickets (
  ticket_id text PRIMARY KEY,
  game_id text NOT NULL,
  environment text NOT NULL,
  mode_id text NOT NULL,
  definition_revision bigint NOT NULL CHECK (definition_revision > 0),
  build text NOT NULL,
  region text NOT NULL,
  capacity integer NOT NULL CHECK (capacity > 0),
  status text NOT NULL CHECK (status IN ('queued','matched','cancelled','expired')),
  properties jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);
ALTER TABLE match.tickets ADD COLUMN IF NOT EXISTS build text NOT NULL DEFAULT 'unknown';
ALTER TABLE match.tickets ADD COLUMN IF NOT EXISTS region text NOT NULL DEFAULT 'unknown';
ALTER TABLE match.tickets ADD COLUMN IF NOT EXISTS capacity integer NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS match.ticket_members (
  ticket_id text NOT NULL REFERENCES match.tickets(ticket_id) ON DELETE CASCADE,
  player_id text NOT NULL,
  rating bigint,
  PRIMARY KEY (ticket_id, player_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS ticket_members_active_player_mode
  ON match.ticket_members(player_id, ticket_id)
  WHERE player_id <> '';
CREATE TABLE IF NOT EXISTS match.matches (
  match_id text PRIMARY KEY,
  game_id text NOT NULL,
  environment text NOT NULL,
  mode_id text NOT NULL,
  definition_revision bigint NOT NULL CHECK (definition_revision > 0),
  state text NOT NULL,
  server_build text NOT NULL,
  allocation_id text,
  server_address text,
  server_ports jsonb,
  state_version bigint NOT NULL DEFAULT 1 CHECK (state_version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS match.roster_members (
  match_id text NOT NULL REFERENCES match.matches(match_id) ON DELETE CASCADE,
  player_id text NOT NULL,
  slot integer NOT NULL CHECK (slot >= 0),
  team text,
  PRIMARY KEY (match_id, player_id),
  UNIQUE (match_id, slot)
);
CREATE TABLE IF NOT EXISTS match.allocation_attempts (
  attempt_id text PRIMARY KEY,
  match_id text NOT NULL REFERENCES match.matches(match_id) ON DELETE CASCADE,
  status text NOT NULL,
  allocator_request jsonb NOT NULL,
  allocator_response jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS match.join_claims (
  jti text PRIMARY KEY,
  match_id text NOT NULL REFERENCES match.matches(match_id) ON DELETE CASCADE,
  player_id text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz
);
CREATE TABLE IF NOT EXISTS match.results (
  match_id text NOT NULL REFERENCES match.matches(match_id) ON DELETE CASCADE,
  result_sequence bigint NOT NULL CHECK (result_sequence > 0),
  payload_digest text NOT NULL,
  payload jsonb NOT NULL,
  accepted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (match_id, result_sequence)
);

CREATE TABLE IF NOT EXISTS ops.delivery_checkpoints (
  consumer_name text NOT NULL,
  event_id uuid NOT NULL REFERENCES ops.outbox_events(event_id),
  delivered_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_name, event_id)
);
CREATE TABLE IF NOT EXISTS ops.audit_log (
  audit_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_type text NOT NULL,
  actor_id text NOT NULL,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  correlation_id text NOT NULL,
  details jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
