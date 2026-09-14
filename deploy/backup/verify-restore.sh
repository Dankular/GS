#!/usr/bin/env bash
set -euo pipefail

repo_dir="${GAMESERVICE_DIR:-/opt/gameservice}"
dump="${1:?usage: verify-restore.sh /var/backups/gameservice/file.dump}"
database_user="${GAMESERVICE_BACKUP_DATABASE_USER:-gameservice_admin}"
test -f "$dump"
if [[ "$dump" == *.age ]]; then
  identity="${GAMESERVICE_BACKUP_AGE_IDENTITY:-}"
  test -n "$identity" || { echo "GAMESERVICE_BACKUP_AGE_IDENTITY is required for encrypted restore" >&2; exit 3; }
  command -v age >/dev/null || { echo "age is required for encrypted restore" >&2; exit 4; }
fi
cd "$repo_dir"
db="gameservice_restore_$$"
cleanup() {
  docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
    dropdb -U "$database_user" --if-exists "$db" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
  createdb -U "$database_user" "$db"
if [[ "$dump" == *.age ]]; then
  age -d -i "$identity" "$dump" | docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
    pg_restore -U "$database_user" -d "$db" --no-owner
else
  docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
    pg_restore -U "$database_user" -d "$db" --no-owner < "$dump"
fi
tables="$(docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
  psql -U "$database_user" -d "$db" -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema IN ('platform','economy','match','ops');")"
test "${tables//[[:space:]]/}" -gt 0
drift="$(docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
  psql -U "$database_user" -d "$db" -Atc "WITH ledger AS (SELECT player_id,currency,SUM(amount)::bigint AS balance FROM economy.ledger_entries WHERE player_id <> '__system__' GROUP BY player_id,currency) SELECT count(*) FROM economy.wallet_accounts w FULL OUTER JOIN ledger l ON l.player_id=w.player_id AND l.currency=w.currency WHERE COALESCE(w.balance,0) <> COALESCE(l.balance,0);")"
test "${drift//[[:space:]]/}" -eq 0
echo "restore verified: $tables application tables; reconciliation drift: 0"
