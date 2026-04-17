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

usage() {
  cat <<'EOF'
Usage: ./scripts/migrate-backup.sh <from> <to> [--force] [--confirm-prod prod] [--db-backup PATH] [--files-backup PATH] [--skip-db] [--skip-files] [--no-start]

Examples:
  ./scripts/migrate-backup.sh prod dev --force
  ./scripts/migrate-backup.sh dev prod --force --confirm-prod prod
  ./scripts/migrate-backup.sh dev prod --force --confirm-prod prod --db-backup /opt/espocrm-data/backups/dev/db/espocrm-dev_2026-04-03_10-00-00.sql.gz --files-backup /opt/espocrm-data/backups/dev/files/espocrm-dev_files_2026-04-03_10-00-00.tar.gz
EOF
}

if [[ $# -gt 0 ]]; then
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      usage >&2
      die "Unknown argument: $1"
      ;;
  esac
fi

load_contour_vars() {
  local contour="$1"

  # shellcheck disable=SC2034
  ESPO_ENV="$contour"
  unset ENV_FILE
  resolve_env_file
  load_env

  LOADED_ENV_FILE="$ENV_FILE"
  LOADED_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
}

collect_migration_contract_mismatch() {
  local name="$1"
  local source_value="$2"
  local target_value="$3"
  local -n mismatches_ref="$4"

  if [[ "$source_value" != "$target_value" ]]; then
    mismatches_ref+=("$name ('$source_value' vs '$target_value')")
  fi
}

require_migration_contract_compatibility() {
  local mismatches=()

  collect_migration_contract_mismatch "ESPOCRM_IMAGE" "$SOURCE_ESPOCRM_IMAGE" "$TARGET_ESPOCRM_IMAGE" mismatches
  collect_migration_contract_mismatch "MARIADB_TAG" "$SOURCE_MARIADB_TAG" "$TARGET_MARIADB_TAG" mismatches
  collect_migration_contract_mismatch "ESPO_DEFAULT_LANGUAGE" "$SOURCE_ESPO_DEFAULT_LANGUAGE" "$TARGET_ESPO_DEFAULT_LANGUAGE" mismatches
  collect_migration_contract_mismatch "ESPO_TIME_ZONE" "$SOURCE_ESPO_TIME_ZONE" "$TARGET_ESPO_TIME_ZONE" mismatches

  if [[ ${#mismatches[@]} -gt 0 ]]; then
    die "Configs '$SOURCE_CONTOUR' and '$TARGET_CONTOUR' conflict with the migration compatibility contract: ${mismatches[*]}. Align the shared settings first and run ./scripts/doctor.sh all"
  fi
}

use_go_backend() {
  [[ "${ESPO_USE_GO_BACKEND:-0}" == "1" ]]
}

assert_go_backend_ready() {
  use_go_backend || return 0
  [[ -x "$BIN" ]] || die "ESPO_USE_GO_BACKEND=1 requires a built binary: $BIN (for example: go build -o bin/espops ./cmd/espops)"
}

json_extract_string_field() {
  local field="$1"

  awk -v field="$field" '
    $0 ~ "\"" field "\"" {
      line = $0
      sub(/^[[:space:]]*"[^"]+"[[:space:]]*:[[:space:]]*"/, "", line)
      sub(/",[[:space:]]*$/, "", line)
      sub(/"[[:space:]]*$/, "", line)
      gsub(/\\"/, "\"", line)
      gsub(/\\\\/, "\\", line)
      print line
      exit
    }
  '
}

select_latest_source_backup_pair_go() {
  local output status manifest_json prefix stamp expected_manifest_json

  assert_go_backend_ready

  set +e
  output="$("$BIN" --json verify-backup --backup-root "$SOURCE_BACKUP_ROOT_ABS" 2>&1)"
  status=$?
  set -e

  if [[ $status -ne 0 ]]; then
    printf '%s\n' "$output" >&2
    return "$status"
  fi

  manifest_json="$(printf '%s\n' "$output" | json_extract_string_field manifest)"
  DB_BACKUP="$(printf '%s\n' "$output" | json_extract_string_field db_backup)"
  FILES_BACKUP="$(printf '%s\n' "$output" | json_extract_string_field files_backup)"

  [[ -n "$manifest_json" && -n "$DB_BACKUP" && -n "$FILES_BACKUP" ]] \
    || die "Go backend returned an incomplete verify-backup JSON contract for migrate-backup"
  backup_pair_is_coherent "$DB_BACKUP" "$FILES_BACKUP" \
    || die "Go backend selected an incoherent backup-file pair for migrate-backup"
  IFS='|' read -r prefix stamp < <(backup_group_from_db_file "$DB_BACKUP") \
    || die "Go backend selected a DB backup with an unsupported name: $DB_BACKUP"
  IFS='|' read -r _db _files _manifest_txt expected_manifest_json < <(
    backup_set_paths "$SOURCE_BACKUP_ROOT_ABS" "$prefix" "$stamp"
  )
  [[ "$manifest_json" == "$expected_manifest_json" ]] \
    || die "Go backend selected a manifest that does not match the backup-set name: $manifest_json"
}

select_latest_source_backup_pair() {
  local group_key prefix stamp

  if use_go_backend; then
    select_latest_source_backup_pair_go
    return
  fi

  group_key="$(latest_complete_backup_group_key "$SOURCE_BACKUP_ROOT_ABS" 1 1 0 1 || true)"
  [[ -n "$group_key" ]] || die "No complete backup set with checksums was found for source contour '$SOURCE_CONTOUR'"

  IFS='|' read -r prefix stamp <<< "$group_key"
  IFS='|' read -r DB_BACKUP FILES_BACKUP _manifest_txt _manifest_json < <(
    backup_set_paths "$SOURCE_BACKUP_ROOT_ABS" "$prefix" "$stamp"
  )
}

resolve_source_backup_selection() {
  if [[ $SKIP_DB -eq 0 && $SKIP_FILES -eq 0 ]]; then
    if [[ -z "$DB_BACKUP" && -z "$FILES_BACKUP" ]]; then
      select_latest_source_backup_pair
    elif [[ -n "$DB_BACKUP" && -z "$FILES_BACKUP" ]]; then
      FILES_BACKUP="$(matching_files_backup_for_db "$SOURCE_BACKUP_ROOT_ABS" "$DB_BACKUP" || true)"
    elif [[ -z "$DB_BACKUP" && -n "$FILES_BACKUP" ]]; then
      DB_BACKUP="$(matching_db_backup_for_files "$SOURCE_BACKUP_ROOT_ABS" "$FILES_BACKUP" || true)"
    fi

    [[ -n "$DB_BACKUP" && -f "$DB_BACKUP" ]] || die "No coherent database backup was found for source contour '$SOURCE_CONTOUR'"
    [[ -n "$FILES_BACKUP" && -f "$FILES_BACKUP" ]] || die "No coherent files backup was found for source contour '$SOURCE_CONTOUR'"
    backup_pair_is_coherent "$DB_BACKUP" "$FILES_BACKUP" || die "The database and files backups must belong to the same backup set"
    return
  fi

  if [[ $SKIP_DB -eq 0 && -z "$DB_BACKUP" ]]; then
    DB_BACKUP="$(latest_verified_backup_file "$SOURCE_BACKUP_ROOT_ABS/db" '*.sql.gz' || true)"
  fi

  if [[ $SKIP_FILES -eq 0 && -z "$FILES_BACKUP" ]]; then
    FILES_BACKUP="$(latest_verified_backup_file "$SOURCE_BACKUP_ROOT_ABS/files" '*.tar.gz' || true)"
  fi
}

if [[ $# -lt 2 ]]; then
  usage >&2
  exit 1
fi

SOURCE_CONTOUR="$1"
TARGET_CONTOUR="$2"
shift 2

case "$SOURCE_CONTOUR" in
  dev|prod) ;;
  *) die "Source contour must be dev or prod" ;;
esac

case "$TARGET_CONTOUR" in
  dev|prod) ;;
  *) die "Target contour must be dev or prod" ;;
esac

[[ "$SOURCE_CONTOUR" != "$TARGET_CONTOUR" ]] || die "Source and target contours must differ"

DB_BACKUP=""
FILES_BACKUP=""
FORCE=0
CONFIRM_PROD=""
SKIP_DB=0
SKIP_FILES=0
NO_START=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --db-backup)
      [[ $# -ge 2 ]] || die "--db-backup must be followed by a path"
      DB_BACKUP="$(caller_path "$2")"
      shift 2
      ;;
    --force)
      FORCE=1
      shift
      ;;
    --confirm-prod)
      [[ $# -ge 2 ]] || die "--confirm-prod must be followed by prod"
      [[ "$2" == "prod" ]] || die "--confirm-prod accepts only the value prod"
      CONFIRM_PROD="prod"
      shift 2
      ;;
    --files-backup)
      [[ $# -ge 2 ]] || die "--files-backup must be followed by a path"
      FILES_BACKUP="$(caller_path "$2")"
      shift 2
      ;;
    --skip-db)
      SKIP_DB=1
      shift
      ;;
    --skip-files)
      SKIP_FILES=1
      shift
      ;;
    --no-start)
      NO_START=1
      shift
      ;;
    *)
      usage >&2
      die "Unknown argument: $1"
      ;;
  esac
done

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Nothing to migrate: --skip-db and --skip-files cannot both be set"
fi

acquire_operation_lock migrate-backup
load_contour_vars "$SOURCE_CONTOUR"
SOURCE_ENV_FILE="$LOADED_ENV_FILE"
SOURCE_BACKUP_ROOT_ABS="$LOADED_BACKUP_ROOT_ABS"
SOURCE_ESPOCRM_IMAGE="$ESPOCRM_IMAGE"
SOURCE_MARIADB_TAG="$MARIADB_TAG"
SOURCE_ESPO_DEFAULT_LANGUAGE="$ESPO_DEFAULT_LANGUAGE"
SOURCE_ESPO_TIME_ZONE="$ESPO_TIME_ZONE"
resolve_source_backup_selection

if [[ $SKIP_DB -eq 0 ]]; then
  [[ -n "$DB_BACKUP" && -f "$DB_BACKUP" ]] || die "Database backup for source contour '$SOURCE_CONTOUR' was not found"
  verify_sha256_required "$DB_BACKUP"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  [[ -n "$FILES_BACKUP" && -f "$FILES_BACKUP" ]] || die "Files backup for source contour '$SOURCE_CONTOUR' was not found"
  verify_sha256_required "$FILES_BACKUP"
fi

load_contour_vars "$TARGET_CONTOUR"
TARGET_ENV_FILE="$LOADED_ENV_FILE"
TARGET_ESPOCRM_IMAGE="$ESPOCRM_IMAGE"
TARGET_MARIADB_TAG="$MARIADB_TAG"
TARGET_ESPO_DEFAULT_LANGUAGE="$ESPO_DEFAULT_LANGUAGE"
TARGET_ESPO_TIME_ZONE="$ESPO_TIME_ZONE"
require_migration_contract_compatibility
ensure_runtime_dirs
require_destructive_approval "migrate-backup" "$FORCE" "$CONFIRM_PROD"

echo "[info] Source contour: $SOURCE_CONTOUR"
echo "[info] Source env file: $SOURCE_ENV_FILE"
echo "[info] Target contour: $TARGET_CONTOUR"
echo "[info] Target env file: $TARGET_ENV_FILE"
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[info] Database backup: $DB_BACKUP"
fi
if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[info] Files backup: $FILES_BACKUP"
fi

require_compose
acquire_maintenance_lock migrate

echo "[1/4] Ensuring the target contour database container is running"
run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" up -d db

echo "[2/4] Stopping target contour application services"
run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" stop espocrm espocrm-daemon espocrm-websocket || true

STEP=3
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[$STEP/4] Restoring the database into contour $TARGET_CONTOUR"
  if [[ "$TARGET_CONTOUR" == "prod" ]]; then
    run_repo_script "$SCRIPT_DIR/restore-db.sh" "$TARGET_CONTOUR" "$DB_BACKUP" --force --confirm-prod prod --no-snapshot --no-stop --no-start
  else
    run_repo_script "$SCRIPT_DIR/restore-db.sh" "$TARGET_CONTOUR" "$DB_BACKUP" --force --no-snapshot --no-stop --no-start
  fi
  STEP=$((STEP + 1))
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[$STEP/4] Restoring files into contour $TARGET_CONTOUR"
  if [[ "$TARGET_CONTOUR" == "prod" ]]; then
    run_repo_script "$SCRIPT_DIR/restore-files.sh" "$TARGET_CONTOUR" "$FILES_BACKUP" --force --confirm-prod prod --no-snapshot --no-stop --no-start
  else
    run_repo_script "$SCRIPT_DIR/restore-files.sh" "$TARGET_CONTOUR" "$FILES_BACKUP" --force --no-snapshot --no-stop --no-start
  fi
fi

if [[ $NO_START -eq 0 ]]; then
  echo "[4/4] Starting the target contour after migration"
  run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" up -d
else
  echo "[4/4] The target contour was left stopped because of --no-start"
fi

echo "Migration completed: $SOURCE_CONTOUR -> $TARGET_CONTOUR"
