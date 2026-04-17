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
Usage: ./scripts/restore-drill.sh <dev|prod> [--db-backup PATH] [--files-backup PATH] [--timeout SEC] [--app-port PORT] [--ws-port PORT] [--skip-http-probe] [--keep-artifacts]

Examples:
  ./scripts/restore-drill.sh prod
  ./scripts/restore-drill.sh dev --timeout 900
  ./scripts/restore-drill.sh prod --db-backup /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/restore-drill.sh dev --files-backup /opt/espocrm-data/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/restore-drill.sh prod --app-port 28080 --ws-port 28081 --keep-artifacts

  --timeout SEC sets the shared service-readiness budget
  across all wait steps in the restore drill.
EOF
}

derive_drill_port() {
  local source_port="$1"
  local fallback_port="$2"

  if [[ "$source_port" =~ ^[0-9]+$ ]] && (( source_port + 20000 <= 65535 )); then
    printf '%s\n' "$((source_port + 20000))"
  else
    printf '%s\n' "$fallback_port"
  fi
}

ensure_port_available() {
  local port="$1"
  local label="$2"
  local port_check_status=0

  if is_tcp_port_busy "$port"; then
    die "Port $port is already in use, restore-drill cannot use it for $label"
  else
    port_check_status=$?
  fi

  if [[ $port_check_status -eq 2 ]]; then
    warn "Could not automatically check whether port $port, is busy, continuing without the preflight check"
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

select_latest_drill_backup_pair_go() {
  local output status

  assert_go_backend_ready

  set +e
  output="$("$BIN" --json verify-backup --backup-root "$SOURCE_BACKUP_ROOT_ABS" 2>&1)"
  status=$?
  set -e

  if [[ $status -ne 0 ]]; then
    printf '%s\n' "$output" >&2
    return "$status"
  fi

  DB_BACKUP_FILE="$(printf '%s\n' "$output" | json_extract_string_field db_backup)"
  FILES_BACKUP_FILE="$(printf '%s\n' "$output" | json_extract_string_field files_backup)"

  [[ -n "$DB_BACKUP_FILE" && -n "$FILES_BACKUP_FILE" ]] \
    || die "Go backend returned an incomplete verify-backup JSON contract for restore-drill"
}

select_latest_drill_backup_pair() {
  local group_key prefix stamp

  if use_go_backend; then
    select_latest_drill_backup_pair_go
    return
  fi

  group_key="$(latest_complete_backup_group_key "$SOURCE_BACKUP_ROOT_ABS" 1 1 1 1 || true)"
  [[ -n "$group_key" ]] || die "No complete backup set with checksums and manifests was found for restore-drill"

  IFS='|' read -r prefix stamp <<< "$group_key"
  IFS='|' read -r DB_BACKUP_FILE FILES_BACKUP_FILE _manifest_txt _manifest_json < <(
    backup_set_paths "$SOURCE_BACKUP_ROOT_ABS" "$prefix" "$stamp"
  )
}

resolve_drill_backup_selection() {
  if [[ -z "$DB_BACKUP_ARG" && -z "$FILES_BACKUP_ARG" ]]; then
    select_latest_drill_backup_pair
    return
  fi

  if [[ -n "$DB_BACKUP_ARG" ]]; then
    DB_BACKUP_FILE="$(caller_path "$DB_BACKUP_ARG")"
  fi

  if [[ -n "$FILES_BACKUP_ARG" ]]; then
    FILES_BACKUP_FILE="$(caller_path "$FILES_BACKUP_ARG")"
  fi

  if [[ -n "$DB_BACKUP_FILE" && -z "$FILES_BACKUP_FILE" ]]; then
    FILES_BACKUP_FILE="$(matching_files_backup_for_db "$SOURCE_BACKUP_ROOT_ABS" "$DB_BACKUP_FILE" || true)"
  elif [[ -z "$DB_BACKUP_FILE" && -n "$FILES_BACKUP_FILE" ]]; then
    DB_BACKUP_FILE="$(matching_db_backup_for_files "$SOURCE_BACKUP_ROOT_ABS" "$FILES_BACKUP_FILE" || true)"
  fi

  [[ -n "$DB_BACKUP_FILE" && -f "$DB_BACKUP_FILE" ]] || die "No coherent database backup was found for restore-drill; pass --db-backup explicitly if needed"
  [[ -n "$FILES_BACKUP_FILE" && -f "$FILES_BACKUP_FILE" ]] || die "No coherent files backup was found for restore-drill; pass --files-backup explicitly if needed"
  backup_pair_is_coherent "$DB_BACKUP_FILE" "$FILES_BACKUP_FILE" || die "The database and files backups used for restore-drill must belong to the same backup set"
}

on_error() {
  local exit_code="$?"
  trap - ERR

  if [[ ${ERROR_HANDLER_ACTIVE:-0} -eq 1 ]]; then
    exit "$exit_code"
  fi

  ERROR_HANDLER_ACTIVE=1

  if [[ ${BUNDLE_ON_FAIL:-1} -eq 1 && -n "${FAILURE_BUNDLE_PATH:-}" ]]; then
    warn "Restore-drill failed, collecting a support bundle"
    if [[ -n "${DRILL_ENV_FILE:-}" && -f "${DRILL_ENV_FILE:-}" ]]; then
      run_support_bundle_capture \
        "$SCRIPT_DIR" \
        "$ESPO_ENV" \
        "$DRILL_ENV_FILE" \
        "$FAILURE_BUNDLE_PATH" \
        "Collected a restore-drill support bundle" \
        "Could not collect a restore-drill support bundle automatically"
    fi
  fi

  exit "$exit_code"
}

cleanup() {
  local exit_code="$1"

  if [[ -n "${DRILL_ENV_FILE:-}" && -f "${DRILL_ENV_FILE:-}" ]]; then
    if [[ ${KEEP_ARTIFACTS:-0} -eq 0 ]]; then
      ENV_FILE="$DRILL_ENV_FILE" compose down >/dev/null 2>&1 || true
      rm -f "$DRILL_ENV_FILE"

      if [[ -n "${DRILL_DB_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$DRILL_DB_STORAGE_ABS"
      fi
      if [[ -n "${DRILL_ESPO_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$DRILL_ESPO_STORAGE_ABS"
      fi
      if [[ -n "${DRILL_BACKUP_ROOT_ABS:-}" ]]; then
        safe_remove_tree "$DRILL_BACKUP_ROOT_ABS"
      fi
    else
      warn "Temporary restore-drill contour preserved because of --keep-artifacts"
      warn "Drill contour env file: $DRILL_ENV_FILE"
      if [[ -n "${DRILL_SITE_URL:-}" ]]; then
        warn "Drill contour URL: $DRILL_SITE_URL"
      fi
    fi
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
DB_BACKUP_ARG=""
FILES_BACKUP_ARG=""
KEEP_ARTIFACTS=0
SKIP_HTTP_PROBE=0
TIMEOUT_SECONDS=600
DRILL_APP_PORT=""
DRILL_WS_PORT=""

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --db-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--db-backup must be followed by a .sql.gz path"
      DB_BACKUP_ARG="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--files-backup must be followed by a .tar.gz path"
      FILES_BACKUP_ARG="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --timeout)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--timeout must be followed by a number of seconds"
      TIMEOUT_SECONDS="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --app-port)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--app-port must be followed by a port number"
      DRILL_APP_PORT="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --ws-port)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--ws-port must be followed by a port number"
      DRILL_WS_PORT="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --skip-http-probe)
      SKIP_HTTP_PROBE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --keep-artifacts)
      KEEP_ARTIFACTS=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

require_explicit_contour

[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || die "Timeout must be an integer number of seconds"
[[ -z "$DRILL_APP_PORT" || "$DRILL_APP_PORT" =~ ^[0-9]+$ ]] || die "Application port must be numeric"
[[ -z "$DRILL_WS_PORT" || "$DRILL_WS_PORT" =~ ^[0-9]+$ ]] || die "WebSocket port must be numeric"

acquire_operation_lock restore-drill
resolve_env_file
load_env
ensure_runtime_dirs

SOURCE_ENV_FILE="$ENV_FILE"
SOURCE_COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME"
SOURCE_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
SOURCE_REPORTS_DIR="$SOURCE_BACKUP_ROOT_ABS/reports"
SOURCE_SUPPORT_DIR="$SOURCE_BACKUP_ROOT_ABS/support"
SOURCE_NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
SOURCE_APP_PORT="$APP_PORT"
SOURCE_WS_PORT="$WS_PORT"
DB_BACKUP_FILE=""
FILES_BACKUP_FILE=""
resolve_drill_backup_selection

if [[ -z "$DRILL_APP_PORT" ]]; then
  if [[ "$ESPO_ENV" == "prod" ]]; then
    DRILL_APP_PORT="$(derive_drill_port "$SOURCE_APP_PORT" 28080)"
  else
    DRILL_APP_PORT="$(derive_drill_port "$SOURCE_APP_PORT" 28088)"
  fi
fi

if [[ -z "$DRILL_WS_PORT" ]]; then
  if [[ "$ESPO_ENV" == "prod" ]]; then
    DRILL_WS_PORT="$(derive_drill_port "$SOURCE_WS_PORT" 28081)"
  else
    DRILL_WS_PORT="$(derive_drill_port "$SOURCE_WS_PORT" 28089)"
  fi
fi

(( DRILL_APP_PORT >= 1 && DRILL_APP_PORT <= 65535 )) || die "Invalid APP_PORT for restore-drill: $DRILL_APP_PORT"
(( DRILL_WS_PORT >= 1 && DRILL_WS_PORT <= 65535 )) || die "Invalid WS_PORT for restore-drill: $DRILL_WS_PORT"
[[ "$DRILL_APP_PORT" != "$DRILL_WS_PORT" ]] || die "APP and WS ports for restore-drill must differ"

ensure_port_available "$DRILL_APP_PORT" "HTTP"
ensure_port_available "$DRILL_WS_PORT" "websocket"

echo "Source contour: $ESPO_ENV"
echo "Source env file: $SOURCE_ENV_FILE"
echo "Source compose project: $SOURCE_COMPOSE_PROJECT_NAME"
echo "Database backup for the restore drill: $DB_BACKUP_FILE"
echo "Files backup for the restore drill: $FILES_BACKUP_FILE"

require_compose

DRILL_ENV_FILE="$(mktemp "$ROOT_DIR/.env.restore-drill.${ESPO_ENV}.XXXXXX")"
cp "$SOURCE_ENV_FILE" "$DRILL_ENV_FILE"

set_env_value "$DRILL_ENV_FILE" "COMPOSE_PROJECT_NAME" "espo-restore-drill-$ESPO_ENV"
set_env_value "$DRILL_ENV_FILE" "DB_STORAGE_DIR" "./storage/restore-drill/$ESPO_ENV/db"
set_env_value "$DRILL_ENV_FILE" "ESPO_STORAGE_DIR" "./storage/restore-drill/$ESPO_ENV/espo"
set_env_value "$DRILL_ENV_FILE" "BACKUP_ROOT" "./backups/restore-drill/$ESPO_ENV"
set_env_value "$DRILL_ENV_FILE" "BACKUP_NAME_PREFIX" "espocrm-restore-drill-$ESPO_ENV"
set_env_value "$DRILL_ENV_FILE" "APP_PORT" "$DRILL_APP_PORT"
set_env_value "$DRILL_ENV_FILE" "WS_PORT" "$DRILL_WS_PORT"
set_env_value "$DRILL_ENV_FILE" "SITE_URL" "http://127.0.0.1:$DRILL_APP_PORT"
set_env_value "$DRILL_ENV_FILE" "WS_PUBLIC_URL" "ws://127.0.0.1:$DRILL_WS_PORT"
set_env_value "$DRILL_ENV_FILE" "DB_NAME" "espocrm_restore_drill_${ESPO_ENV}"
set_env_value "$DRILL_ENV_FILE" "DB_ROOT_PASSWORD" "restore_drill_${ESPO_ENV}_root"
set_env_value "$DRILL_ENV_FILE" "DB_PASSWORD" "restore_drill_${ESPO_ENV}_db"
set_env_value "$DRILL_ENV_FILE" "ADMIN_PASSWORD" "restore_drill_${ESPO_ENV}_admin"

export ENV_FILE="$DRILL_ENV_FILE"
resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock restore-drill

DRILL_DB_STORAGE_ABS="$(root_path "$DB_STORAGE_DIR")"
DRILL_ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
DRILL_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
DRILL_SITE_URL="$SITE_URL"
STAMP="$(next_unique_stamp \
  "$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill___STAMP__.txt" \
  "$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill___STAMP__.json" \
  "$SOURCE_SUPPORT_DIR/${SOURCE_NAME_PREFIX}_restore-drill-failure___STAMP__.tar.gz")"
DRILL_REPORT_TXT="$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill_${STAMP}.txt"
DRILL_REPORT_JSON="$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SOURCE_SUPPORT_DIR/${SOURCE_NAME_PREFIX}_restore-drill-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0
BUNDLE_ON_FAIL=1
# shellcheck disable=SC2034
READINESS_TIMEOUT_BUDGET="$TIMEOUT_SECONDS"

trap 'on_error' ERR
append_trap 'cleanup $?' EXIT

# If a previous drill run was left unfinished, remove the temporary stack and directories first.
compose down >/dev/null 2>&1 || true
safe_remove_tree "$DRILL_DB_STORAGE_ABS"
safe_remove_tree "$DRILL_ESPO_STORAGE_ABS"
safe_remove_tree "$DRILL_BACKUP_ROOT_ABS"
ensure_runtime_dirs

print_context

echo "[1/6] Preparing the temporary restore-drill contour"
echo "  HTTP URL:      $SITE_URL"
echo "  WebSocket URL: $WS_PUBLIC_URL"

echo "[2/6] Starting the temporary database"
compose up -d db
wait_for_service_ready_with_shared_timeout READINESS_TIMEOUT_BUDGET db "restore drill"

echo "[3/6] Restoring the database into the temporary contour"
if [[ "$ESPO_ENV" == "prod" ]]; then
  ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$DB_BACKUP_FILE" --force --confirm-prod prod --no-snapshot --no-stop --no-start
else
  ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$DB_BACKUP_FILE" --force --no-snapshot --no-stop --no-start
fi

echo "[4/6] Restoring files into the temporary contour"
if [[ "$ESPO_ENV" == "prod" ]]; then
  ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$FILES_BACKUP_FILE" --force --confirm-prod prod --no-snapshot --no-stop --no-start
else
  ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$FILES_BACKUP_FILE" --force --no-snapshot --no-stop --no-start
fi

echo "[5/6] Starting the full temporary stack"
compose up -d
wait_for_application_stack_with_shared_timeout READINESS_TIMEOUT_BUDGET "restore drill"

echo "[6/6] Checking application readiness after restore"
if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
  http_probe "$SITE_URL"
else
  echo "HTTP probe skipped because of --skip-http-probe"
fi

ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$DRILL_REPORT_TXT"
ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$DRILL_REPORT_JSON"

trap - ERR

echo "Restore drill completed successfully"
echo "Report: $DRILL_REPORT_TXT"
echo "JSON report: $DRILL_REPORT_JSON"
