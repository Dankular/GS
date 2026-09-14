CREATE TABLE IF NOT EXISTS platform.definition_approvals (
  approval_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id text NOT NULL,
  environment text NOT NULL,
  revision bigint NOT NULL CHECK (revision > 0),
  digest text NOT NULL,
  requested_by text NOT NULL,
  approved_by text,
  status text NOT NULL CHECK (status IN ('pending','approved','rejected')),
  reason text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  approved_at timestamptz,
  FOREIGN KEY (game_id, revision) REFERENCES platform.definition_revisions(game_id, revision)
);
CREATE UNIQUE INDEX IF NOT EXISTS definition_approvals_pending_key
  ON platform.definition_approvals(game_id, environment, revision)
  WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS definition_approvals_lookup
  ON platform.definition_approvals(game_id, environment, revision, status);
