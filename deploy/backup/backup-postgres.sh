#!/usr/bin/env bash
set -euo pipefail

repo_dir="${GAMESERVICE_DIR:-/opt/gameservice}"
backup_dir="${GAMESERVICE_BACKUP_DIR:-/var/backups/gameservice}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="${1:-${backup_dir}/gameservice-${stamp}.dump}"
age_recipient="${GAMESERVICE_BACKUP_AGE_RECIPIENT:-}"
require_encryption="${GAMESERVICE_BACKUP_REQUIRE_ENCRYPTION:-0}"

if [ -n "$age_recipient" ]; then
  case "$target" in
    *.age) ;;
    *) target="${target}.age" ;;
  esac
fi

case "$target" in
  "$backup_dir"/*) ;;
  *) echo "backup path must remain under $backup_dir" >&2; exit 2 ;;
esac
mkdir -p "$backup_dir"
umask 077
cd "$repo_dir"
if [ "$require_encryption" = "1" ] && [ -z "$age_recipient" ]; then
  echo "GAMESERVICE_BACKUP_AGE_RECIPIENT is required when encryption is mandatory" >&2
  exit 3
fi
if [ -n "$age_recipient" ]; then
  command -v age >/dev/null || { echo "age is required for encrypted backups" >&2; exit 4; }
  plaintext="${target}.plaintext"
  trap 'rm -f "$plaintext"' EXIT
  docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
    pg_dump -U gameservice -d gameservice --format=custom --no-owner > "$plaintext"
  age -r "$age_recipient" -o "$target" "$plaintext"
  rm -f "$plaintext"
else
  docker compose --env-file .env -f deploy/compose/compose.yaml exec -T postgres \
    pg_dump -U gameservice -d gameservice --format=custom --no-owner > "$target"
fi
test -s "$target"
sha256sum "$target" > "${target}.sha256"
if [ -n "${GAMESERVICE_BACKUP_S3_URI:-}" ]; then
  command -v aws >/dev/null || { echo "aws CLI is required for off-site backup upload" >&2; exit 5; }
  aws s3 cp "$target" "${GAMESERVICE_BACKUP_S3_URI%/}/$(basename "$target")" --sse "${GAMESERVICE_BACKUP_S3_SSE:-aws:kms}" ${GAMESERVICE_BACKUP_SSE_KMS_KEY:+--sse-kms-key-id "$GAMESERVICE_BACKUP_SSE_KMS_KEY"}
  aws s3 cp "${target}.sha256" "${GAMESERVICE_BACKUP_S3_URI%/}/$(basename "$target").sha256" --sse "${GAMESERVICE_BACKUP_S3_SSE:-aws:kms}" ${GAMESERVICE_BACKUP_SSE_KMS_KEY:+--sse-kms-key-id "$GAMESERVICE_BACKUP_SSE_KMS_KEY"}
fi
echo "$target"
