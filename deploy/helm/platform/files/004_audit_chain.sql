CREATE EXTENSION IF NOT EXISTS pgcrypto;
ALTER TABLE ops.audit_log ADD COLUMN IF NOT EXISTS previous_hash text;
ALTER TABLE ops.audit_log ADD COLUMN IF NOT EXISTS record_hash text;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='ops.audit_log'::regclass AND tgname='audit_log_no_update') THEN
    UPDATE ops.audit_log
    SET previous_hash = COALESCE(previous_hash, ''),
        record_hash = COALESCE(record_hash, encode(digest(audit_id::text || '|' || actor_type || '|' || actor_id || '|' || action || '|' || resource_type || '|' || resource_id || '|' || correlation_id || '|' || details::text || '|' || created_at::text, 'sha256'), 'hex'))
    WHERE record_hash IS NULL;
  END IF;
END;
$$;

CREATE OR REPLACE FUNCTION ops.audit_log_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'ops.audit_log is append-only';
END;
$$;
DROP TRIGGER IF EXISTS audit_log_no_update ON ops.audit_log;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON ops.audit_log FOR EACH ROW EXECUTE FUNCTION ops.audit_log_immutable();

CREATE OR REPLACE FUNCTION ops.audit_log_hash_chain() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE prior_hash text;
BEGIN
  PERFORM pg_advisory_xact_lock(871234);
  SELECT record_hash INTO prior_hash FROM ops.audit_log WHERE record_hash IS NOT NULL ORDER BY created_at DESC, audit_id DESC LIMIT 1;
  NEW.previous_hash := COALESCE(prior_hash, '');
  NEW.record_hash := encode(digest(NEW.audit_id::text || '|' || NEW.actor_type || '|' || NEW.actor_id || '|' || NEW.action || '|' || NEW.resource_type || '|' || NEW.resource_id || '|' || NEW.correlation_id || '|' || NEW.details::text || '|' || NEW.created_at::text || '|' || NEW.previous_hash, 'sha256'), 'hex');
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS audit_log_hash_chain ON ops.audit_log;
CREATE TRIGGER audit_log_hash_chain BEFORE INSERT ON ops.audit_log FOR EACH ROW EXECUTE FUNCTION ops.audit_log_hash_chain();
