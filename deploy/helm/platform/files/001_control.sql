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
CREATE TABLE IF NOT EXISTS platform.games (
  game_id text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS platform.environments (
  game_id text NOT NULL REFERENCES platform.games(game_id) ON DELETE CASCADE,
  environment text NOT NULL,
  PRIMARY KEY (game_id, environment)
);
CREATE TABLE IF NOT EXISTS platform.definition_revisions (
  game_id text NOT NULL REFERENCES platform.games(game_id) ON DELETE CASCADE,
  revision bigint NOT NULL CHECK (revision > 0),
  digest text NOT NULL,
  source_yaml text NOT NULL,
  canonical jsonb NOT NULL,
  compiled jsonb NOT NULL,
  validation_report jsonb NOT NULL,
  actor_id text NOT NULL,
  status text NOT NULL CHECK (status IN ('validated','published','activated','superseded')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, revision),
  UNIQUE (game_id, digest)
);
CREATE TABLE IF NOT EXISTS platform.definition_activations (
  game_id text NOT NULL,
  environment text NOT NULL,
  revision bigint NOT NULL,
  activated_by text NOT NULL,
  activated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, environment),
  FOREIGN KEY (game_id, revision) REFERENCES platform.definition_revisions(game_id, revision)
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
CREATE TABLE IF NOT EXISTS economy.entitlements (
  player_id text NOT NULL,
  entitlement_id text NOT NULL,
  expires_at timestamptz,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (player_id, entitlement_id)
);
CREATE SCHEMA IF NOT EXISTS progression;
CREATE TABLE IF NOT EXISTS progression.player_progress (
  player_id text NOT NULL,
  track_id text NOT NULL,
  xp bigint NOT NULL CHECK (xp >= 0),
  level bigint NOT NULL CHECK (level >= 1),
  version bigint NOT NULL CHECK (version > 0),
  PRIMARY KEY (player_id, track_id)
);
CREATE TABLE IF NOT EXISTS progression.objective_completions (
  player_id text NOT NULL,
  objective_id text NOT NULL,
  source_id text NOT NULL,
  completed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, objective_id, source_id)
);
CREATE TABLE IF NOT EXISTS economy.reward_claims (
  player_id text NOT NULL,
  reward_id text NOT NULL,
  source_id text NOT NULL,
  request_id text NOT NULL,
  claimed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, reward_id, source_id),
  UNIQUE (request_id)
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
ALTER TABLE match.tickets ADD COLUMN IF NOT EXISTS allocation_attempts integer NOT NULL DEFAULT 0 CHECK (allocation_attempts >= 0);
DO $$
BEGIN
  ALTER TABLE match.tickets DROP CONSTRAINT IF EXISTS tickets_status_check;
  ALTER TABLE match.tickets ADD CONSTRAINT tickets_status_check CHECK (status IN ('queued','matching','matched','cancelled','expired'));
END $$;
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
CREATE TABLE IF NOT EXISTS match.result_conflicts (
  conflict_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  match_id text NOT NULL REFERENCES match.matches(match_id) ON DELETE CASCADE,
  result_sequence bigint NOT NULL CHECK (result_sequence > 0),
  accepted_digest text NOT NULL,
  conflicting_digest text NOT NULL,
  correlation_id text NOT NULL,
  detected_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS result_conflicts_match_idx ON match.result_conflicts(match_id, result_sequence);

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

CREATE TABLE IF NOT EXISTS platform.player_restrictions (
  player_id text PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('ban', 'queue', 'admission')),
  reason text NOT NULL,
  expires_at timestamptz,
  actor_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
