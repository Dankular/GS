#!/usr/bin/env bash
set -euo pipefail

repo_dir="${GAMESERVICE_DIR:-/opt/gameservice}"
backup_dir="${GAMESERVICE_BACKUP_DIR:-/var/backups/gameservice}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="${1:-${backup_dir}/gameservice-${stamp}.dump}"

case "$target" in
  "$backup_dir"/*) ;;
  *) echo "backup path must remain under $backup_dir" >&2; exit 2 ;;
esac
mkdir -p "$backup_dir"
umask 077
cd "$repo_dir"
docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
  pg_dump -U gameservice -d gameservice --format=custom --no-owner > "$target"
test -s "$target"
sha256sum "$target" > "${target}.sha256"
echo "$target"
