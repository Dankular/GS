#!/bin/sh
set -eu

ENV_FILE="${ENV_FILE:-.env}"
COMPOSE_FILE="${COMPOSE_FILE:-deploy/compose/compose.yaml}"

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

echo "applying control-plane migrations"
compose run --rm migrations

echo "rolling back the development migration boundary"
compose run --rm migrations sh -ec \
  'psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f /migrations/001_control.down.sql'

echo "reapplying control-plane migrations"
compose run --rm migrations
echo "migration up/down/up verification passed"
