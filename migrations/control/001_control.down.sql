-- Explicit development/test rollback for the single control-plane migration.
-- This is destructive by design and must never be run against production data.
DROP SCHEMA IF EXISTS ops CASCADE;
DROP SCHEMA IF EXISTS match CASCADE;
DROP SCHEMA IF EXISTS progression CASCADE;
DROP SCHEMA IF EXISTS economy CASCADE;
DROP SCHEMA IF EXISTS platform CASCADE;
