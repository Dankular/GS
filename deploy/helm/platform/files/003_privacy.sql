CREATE TABLE IF NOT EXISTS platform.account_privacy_requests (
  request_id text PRIMARY KEY,
  player_id text NOT NULL,
  operation text NOT NULL CHECK (operation IN ('export','delete')),
  status text NOT NULL CHECK (status IN ('completed','failed')),
  result jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS platform.deleted_account_tombstones (
  player_id_hash text PRIMARY KEY,
  deleted_at timestamptz NOT NULL DEFAULT now(),
  request_id text NOT NULL REFERENCES platform.account_privacy_requests(request_id),
  reason text NOT NULL DEFAULT 'account deletion'
);
