#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$ROOT_DIR/bin/espops"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/fs.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/backup.sh <dev|prod> [--skip-db] [--skip-files] [--no-stop]

Examples:
  ./scripts/backup.sh prod
  ./scripts/backup.sh dev
  ./scripts/backup.sh prod --skip-files
  ./scripts/backup.sh dev --skip-db
  ./scripts/backup.sh prod --no-stop
EOF
}

use_go_backend() {
  [[ "${ESPO_USE_GO_BACKEND:-0}" == "1" ]]
}

backup_metadata_uses_go() {
  use_go_backend && [[ $SKIP_DB -eq 0 && $SKIP_FILES -eq 0 ]]
}

assert_go_backup_metadata_ready() {
  backup_metadata_uses_go || return 0
  [[ -x "$BIN" ]] || die "ESPO_USE_GO_BACKEND=1 requires a built binary: $BIN (for example: go build -o bin/espops ./cmd/espops)"
}

write_backup_metadata_with_go() {
  "$BIN" backup \
    --scope "$ESPO_ENV" \
    --created-at "$MANIFEST_CREATED_AT" \
    --db-backup "$DB_FILE" \
    --files-backup "$FILES_FILE" \
    --manifest "$MANIFEST_JSON_FILE_TMP" \
    --db-checksum "$DB_SUM_FILE_TMP" \
    --files-checksum "$FILES_SUM_FILE_TMP" \
    >/dev/null
}

restart_app_services_on_exit() {
  if [[ ${RESTART_APP_SERVICES_ON_EXIT:-0} -eq 1 ]]; then
    warn "Backup failed unexpectedly, restoring application services"
    start_app_services || warn "Could not automatically restart application services after backup failure"
    RESTART_APP_SERVICES_ON_EXIT=0
  fi
}

cleanup_temp_backup_artifacts() {
  rm -f -- \
    "${DB_FILE_TMP:-}" \
    "${FILES_FILE_TMP:-}" \
    "${DB_SUM_FILE_TMP:-}" \
    "${FILES_SUM_FILE_TMP:-}" \
    "${MANIFEST_FILE_TMP:-}" \
    "${MANIFEST_JSON_FILE_TMP:-}"
}

parse_contour_arg "$@"
SKIP_DB=0
SKIP_FILES=0
NO_STOP=0

for arg in "${POSITIONAL_ARGS[@]}"; do
  case "$arg" in
    --skip-db)
      SKIP_DB=1
      ;;
    --skip-files)
      SKIP_FILES=1
      ;;
    --no-stop)
      NO_STOP=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: $arg"
      ;;
  esac
done

require_explicit_contour

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Nothing to back up: --skip-db and --skip-files cannot both be set"
fi

assert_go_backup_metadata_ready

acquire_operation_lock backup
resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock backup

if [[ $SKIP_DB -eq 0 || $NO_STOP -eq 0 ]]; then
  require_compose
fi

if [[ $SKIP_DB -eq 0 ]]; then
  require_service_running db
fi

APP_SERVICES_WERE_RUNNING=0
SHOULD_RESTART_APP_SERVICES=0
RESTART_APP_SERVICES_ON_EXIT=0

if [[ $NO_STOP -eq 0 ]]; then
  if app_services_running; then
    APP_SERVICES_WERE_RUNNING=1
    SHOULD_RESTART_APP_SERVICES=1
    RESTART_APP_SERVICES_ON_EXIT=1
    append_trap 'restart_app_services_on_exit' EXIT
    echo "Stopping application services for a consistent backup"
    stop_app_services
  else
    echo "Application services are already stopped, no extra stop is required"
  fi
else
  warn "Backup is being created without stopping application services: strict consistency is not guaranteed"
fi

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
DB_DIR="$BACKUP_ROOT_ABS/db"
FILES_DIR="$BACKUP_ROOT_ABS/files"
MANIFESTS_DIR="$BACKUP_ROOT_ABS/manifests"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
STAMP="$(next_unique_stamp \
  "$DB_DIR/${NAME_PREFIX}___STAMP__.sql.gz" \
  "$FILES_DIR/${NAME_PREFIX}_files___STAMP__.tar.gz" \
  "$MANIFESTS_DIR/${NAME_PREFIX}___STAMP__.manifest.txt" \
  "$MANIFESTS_DIR/${NAME_PREFIX}___STAMP__.manifest.json")"

DB_FILE="$DB_DIR/${NAME_PREFIX}_${STAMP}.sql.gz"
FILES_FILE="$FILES_DIR/${NAME_PREFIX}_files_${STAMP}.tar.gz"
DB_SUM_FILE="${DB_FILE}.sha256"
FILES_SUM_FILE="${FILES_FILE}.sha256"
MANIFEST_FILE="$MANIFESTS_DIR/${NAME_PREFIX}_${STAMP}.manifest.txt"
MANIFEST_JSON_FILE="$MANIFESTS_DIR/${NAME_PREFIX}_${STAMP}.manifest.json"

DB_FILE_TMP="${DB_FILE}.tmp"
FILES_FILE_TMP="${FILES_FILE}.tmp"
DB_SUM_FILE_TMP="${DB_SUM_FILE}.tmp"
FILES_SUM_FILE_TMP="${FILES_SUM_FILE}.tmp"
MANIFEST_FILE_TMP="${MANIFEST_FILE}.tmp"
MANIFEST_JSON_FILE_TMP="${MANIFEST_JSON_FILE}.tmp"

append_trap 'cleanup_temp_backup_artifacts' EXIT

print_context

STEP=1
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[$STEP/4] Creating database dump: $DB_FILE"
  # shellcheck disable=SC2016
  compose exec -T db sh -lc 'exec mariadb-dump -u"$MARIADB_USER" -p"$MARIADB_PASSWORD" "$MARIADB_DATABASE" --single-transaction --quick --routines --triggers --events' \
    | gzip -9 > "$DB_FILE_TMP"
  mv "$DB_FILE_TMP" "$DB_FILE"
else
  echo "[$STEP/4] Database backup skipped because of --skip-db"
fi
STEP=$((STEP + 1))

if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[$STEP/4] Archiving application files: $FILES_FILE"
  create_tar_archive "$ESPO_STORAGE_ABS" "$FILES_FILE_TMP" \
    || die "Could not archive application files: $ESPO_STORAGE_ABS"
  mv "$FILES_FILE_TMP" "$FILES_FILE"
else
  echo "[$STEP/4] Files backup skipped because of --skip-files"
fi
STEP=$((STEP + 1))

echo "[$STEP/4] Generating checksums and manifest files"

MANIFEST_CREATED_AT_DATE="${STAMP%%_*}"
MANIFEST_CREATED_AT_TIME="${STAMP#*_}"
MANIFEST_CREATED_AT_TIME="${MANIFEST_CREATED_AT_TIME//-/:}"
MANIFEST_CREATED_AT="${MANIFEST_CREATED_AT_DATE}T${MANIFEST_CREATED_AT_TIME}Z"

DB_ARTIFACT_NAME=""
FILES_ARTIFACT_NAME=""
DB_MANIFEST_SHA256=""
FILES_MANIFEST_SHA256=""
GO_METADATA=0

if backup_metadata_uses_go; then
  GO_METADATA=1
  note "Generating checksum sidecars and the JSON manifest through the Go backend"
  write_backup_metadata_with_go

  DB_SHA256="$(read_sha256_sidecar "$DB_SUM_FILE_TMP")"
  FILES_SHA256="$(read_sha256_sidecar "$FILES_SUM_FILE_TMP")"
  DB_SIZE_BYTES="$(file_size_bytes "$DB_FILE")"
  FILES_SIZE_BYTES="$(file_size_bytes "$FILES_FILE")"
  DB_ARTIFACT_NAME="$(basename "$DB_FILE")"
  FILES_ARTIFACT_NAME="$(basename "$FILES_FILE")"
  DB_MANIFEST_SHA256="$DB_SHA256"
  FILES_MANIFEST_SHA256="$FILES_SHA256"
  mv "$DB_SUM_FILE_TMP" "$DB_SUM_FILE"
  mv "$FILES_SUM_FILE_TMP" "$FILES_SUM_FILE"
elif [[ $SKIP_DB -eq 0 ]]; then
  DB_SHA256="$(sha256_file "$DB_FILE")"
  DB_SIZE_BYTES="$(file_size_bytes "$DB_FILE")"
  write_sha256_sidecar "$DB_FILE" "$DB_SHA256" "$DB_SUM_FILE_TMP"
  mv "$DB_SUM_FILE_TMP" "$DB_SUM_FILE"
  DB_ARTIFACT_NAME="$(basename "$DB_FILE")"
  DB_MANIFEST_SHA256="$DB_SHA256"
fi

if [[ $GO_METADATA -eq 0 && $SKIP_FILES -eq 0 ]]; then
  FILES_SHA256="$(sha256_file "$FILES_FILE")"
  FILES_SIZE_BYTES="$(file_size_bytes "$FILES_FILE")"
  write_sha256_sidecar "$FILES_FILE" "$FILES_SHA256" "$FILES_SUM_FILE_TMP"
  mv "$FILES_SUM_FILE_TMP" "$FILES_SUM_FILE"
  FILES_ARTIFACT_NAME="$(basename "$FILES_FILE")"
  FILES_MANIFEST_SHA256="$FILES_SHA256"
fi

cat > "$MANIFEST_FILE_TMP" <<EOF
created_at=$STAMP
contour=$ESPO_ENV
compose_project=$COMPOSE_PROJECT_NAME
env_file=$(basename "$ENV_FILE")
espocrm_image=${ESPOCRM_IMAGE:-}
mariadb_tag=${MARIADB_TAG:-}
retention_days=$RETENTION_DAYS
db_backup_created=$((1 - SKIP_DB))
files_backup_created=$((1 - SKIP_FILES))
consistent_snapshot=$((1 - NO_STOP))
app_services_were_running=$APP_SERVICES_WERE_RUNNING
EOF

if [[ $SKIP_DB -eq 0 ]]; then
  cat >> "$MANIFEST_FILE_TMP" <<EOF
db_backup_file=$(basename "$DB_FILE")
db_backup_checksum_file=$(basename "$DB_SUM_FILE")
db_backup_sha256=$DB_SHA256
db_backup_size_bytes=$DB_SIZE_BYTES
EOF
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  cat >> "$MANIFEST_FILE_TMP" <<EOF
files_backup_file=$(basename "$FILES_FILE")
files_backup_checksum_file=$(basename "$FILES_SUM_FILE")
files_backup_sha256=$FILES_SHA256
files_backup_size_bytes=$FILES_SIZE_BYTES
EOF
fi

mv "$MANIFEST_FILE_TMP" "$MANIFEST_FILE"

if [[ $GO_METADATA -eq 1 ]]; then
  mv "$MANIFEST_JSON_FILE_TMP" "$MANIFEST_JSON_FILE"
else
  {
    printf '{\n'
    printf '  "version": 1,\n'
    printf '  "scope": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "created_at": "%s",\n' "$(json_escape "$MANIFEST_CREATED_AT")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "artifacts": {\n'
    printf '    "db_backup": "%s",\n' "$(json_escape "$DB_ARTIFACT_NAME")"
    printf '    "files_backup": "%s"\n' "$(json_escape "$FILES_ARTIFACT_NAME")"
    printf '  },\n'
    printf '  "checksums": {\n'
    printf '    "db_backup": "%s",\n' "$(json_escape "$DB_MANIFEST_SHA256")"
    printf '    "files_backup": "%s"\n' "$(json_escape "$FILES_MANIFEST_SHA256")"
    printf '  },\n'
    printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
    printf '  "espocrm_image": "%s",\n' "$(json_escape "${ESPOCRM_IMAGE:-}")"
    printf '  "mariadb_tag": "%s",\n' "$(json_escape "${MARIADB_TAG:-}")"
    printf '  "retention_days": %s,\n' "$RETENTION_DAYS"
    printf '  "consistent_snapshot": %s,\n' "$([[ $NO_STOP -eq 0 ]] && echo true || echo false)"
    printf '  "app_services_were_running": %s,\n' "$([[ $APP_SERVICES_WERE_RUNNING -eq 1 ]] && echo true || echo false)"
    printf '  "db_backup_created": %s,\n' "$([[ $SKIP_DB -eq 0 ]] && echo true || echo false)"
    printf '  "files_backup_created": %s\n' "$([[ $SKIP_FILES -eq 0 ]] && echo true || echo false)"
    printf '\n}\n'
  } > "$MANIFEST_JSON_FILE_TMP"
  mv "$MANIFEST_JSON_FILE_TMP" "$MANIFEST_JSON_FILE"
fi
STEP=$((STEP + 1))

echo "[$STEP/4] Removing backups older than $RETENTION_DAYS days"
cleanup_old_files "$DB_DIR" "$RETENTION_DAYS" '*.sql.gz' '*.sql.gz.sha256'
cleanup_old_files "$FILES_DIR" "$RETENTION_DAYS" '*.tar.gz' '*.tar.gz.sha256'
cleanup_old_files "$MANIFESTS_DIR" "$RETENTION_DAYS" '*.manifest.txt' '*.manifest.json'

if [[ $SHOULD_RESTART_APP_SERVICES -eq 1 ]]; then
  echo "Starting application services after a successful backup"
  start_app_services
  RESTART_APP_SERVICES_ON_EXIT=0
fi

echo "Backup completed:"
if [[ $SKIP_DB -eq 0 ]]; then
  echo "  Database: $DB_FILE"
  echo "  Database checksum: $DB_SUM_FILE"
fi
if [[ $SKIP_FILES -eq 0 ]]; then
  echo "  Files:       $FILES_FILE"
  echo "  Files checksum: $FILES_SUM_FILE"
fi
echo "  Text manifest: $MANIFEST_FILE"
echo "  JSON manifest:      $MANIFEST_JSON_FILE"
