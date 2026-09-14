-- Forward migration: associate matched tickets with their durable match.
ALTER TABLE match.tickets
  ADD COLUMN IF NOT EXISTS match_id text REFERENCES match.matches(match_id);

CREATE INDEX IF NOT EXISTS tickets_match_id_idx ON match.tickets(match_id);
