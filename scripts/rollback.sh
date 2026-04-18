#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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
Usage: ./scripts/rollback.sh <dev|prod> [--dry-run] [--force] [--confirm-prod prod] [--db-backup PATH --files-backup PATH] [--no-snapshot] [--no-start] [--skip-http-probe] [--timeout SEC]

Examples:
  ./scripts/rollback.sh dev --force --timeout 900
  ./scripts/rollback.sh dev --dry-run
  ./scripts/rollback.sh prod --force --confirm-prod prod
  ./scripts/rollback.sh prod --force --confirm-prod prod --db-backup /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --files-backup /opt/espocrm-data/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/rollback.sh prod --force --confirm-prod prod --no-snapshot --no-start

  --timeout SEC sets the shared service-readiness budget
  across all rollback wait steps.
EOF
}

select_latest_valid_backup_set() {
  local output status selection_fields expected_manifest_json
  local -a selection_items=()

  command_exists python3 || die "python3 is required to parse the Go verify-backup JSON contract for rollback"

  set +e
  output="$(run_espops --json verify-backup --backup-root "$BACKUP_ROOT_ABS" 2>&1)"
  status=$?
  set -e

  if [[ $status -ne 0 ]]; then
    printf '%s\n' "$output" >&2
    return "$status"
  fi

  selection_fields="$(
    printf '%s\n' "$output" | python3 -c 'import json, sys
data = json.load(sys.stdin)
artifacts = data.get("artifacts") or {}
required = ("manifest", "db_backup", "files_backup")
values = []
for key in required:
    value = artifacts.get(key)
    if not isinstance(value, str) or not value.strip():
        raise SystemExit(f"verify-backup JSON contract for rollback is missing artifacts.{key}")
    values.append(value.strip())
print("\n".join(values))'
  )" || die "Go verify-backup returned an incomplete JSON contract for rollback"

  mapfile -t selection_items <<<"$selection_fields"
  [[ ${#selection_items[@]} -eq 3 ]] || die "Go verify-backup returned an incomplete JSON contract for rollback"
  SELECTED_MANIFEST_JSON="${selection_items[0]}"
  SELECTED_DB_BACKUP="${selection_items[1]}"
  SELECTED_FILES_BACKUP="${selection_items[2]}"

  backup_pair_is_coherent "$SELECTED_DB_BACKUP" "$SELECTED_FILES_BACKUP" \
    || die "Go verify-backup selected an incoherent backup-file pair for rollback"
  IFS='|' read -r SELECTED_PREFIX SELECTED_STAMP < <(backup_group_from_db_file "$SELECTED_DB_BACKUP") \
    || die "Go verify-backup selected a DB backup with an unsupported name: $SELECTED_DB_BACKUP"
  IFS='|' read -r _selected_db _selected_files SELECTED_MANIFEST_TXT expected_manifest_json < <(
    backup_set_paths "$BACKUP_ROOT_ABS" "$SELECTED_PREFIX" "$SELECTED_STAMP"
  )
  [[ "$SELECTED_MANIFEST_JSON" == "$expected_manifest_json" ]] \
    || die "Go verify-backup selected a manifest that does not match the backup-set name: $SELECTED_MANIFEST_JSON"

  [[ -f "$SELECTED_MANIFEST_TXT" ]] || SELECTED_MANIFEST_TXT=""

  return 0
}

validate_manual_backup_pair() {
  local db_file="$1"
  local files_file="$2"

  [[ -f "$db_file" ]] || die "Database backup file not found: $db_file"
  [[ -f "$files_file" ]] || die "Files backup file not found: $files_file"
  backup_pair_is_coherent "$db_file" "$files_file" || die "The database and files backups used for rollback must belong to the same backup set"
  [[ -f "${db_file}.sha256" ]] || die "Database backup checksum file not found: ${db_file}.sha256"
  [[ -f "${files_file}.sha256" ]] || die "Files backup checksum file not found: ${files_file}.sha256"

  verify_sha256_sidecar "$db_file" "${db_file}.sha256" || die "Checksum mismatch for database backup: $db_file"
  verify_sha256_sidecar "$files_file" "${files_file}.sha256" || die "Checksum mismatch for files backup: $files_file"

  if IFS='|' read -r SELECTED_PREFIX SELECTED_STAMP < <(backup_group_from_db_file "$db_file"); then
    IFS='|' read -r _selected_db _selected_files SELECTED_MANIFEST_TXT SELECTED_MANIFEST_JSON < <(
      backup_set_paths "$BACKUP_ROOT_ABS" "$SELECTED_PREFIX" "$SELECTED_STAMP"
    )
    [[ -f "$SELECTED_MANIFEST_TXT" ]] || SELECTED_MANIFEST_TXT=""
    [[ -f "$SELECTED_MANIFEST_JSON" ]] || SELECTED_MANIFEST_JSON=""
  else
    SELECTED_PREFIX="manual"
    SELECTED_STAMP="manual"
    SELECTED_MANIFEST_TXT=""
    SELECTED_MANIFEST_JSON=""
  fi

  SELECTED_DB_BACKUP="$db_file"
  SELECTED_FILES_BACKUP="$files_file"
}

write_rollback_plan_reports() {
  cat > "$ROLLBACK_PLAN_TXT" <<EOF
EspoCRM contour rollback plan
created_at=$STAMP
contour=$ESPO_ENV
compose_project=$COMPOSE_PROJECT_NAME
env_file=$(basename "$ENV_FILE")
selection_mode=$SELECTION_MODE
selected_prefix=$SELECTED_PREFIX
selected_stamp=$SELECTED_STAMP
db_backup=$SELECTED_DB_BACKUP
files_backup=$SELECTED_FILES_BACKUP
manifest_txt=${SELECTED_MANIFEST_TXT:-}
manifest_json=${SELECTED_MANIFEST_JSON:-}
snapshot_enabled=$([[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]] && echo true || echo false)
no_start=$([[ $NO_START -eq 1 ]] && echo true || echo false)
skip_http_probe=$([[ $SKIP_HTTP_PROBE -eq 1 ]] && echo true || echo false)
timeout_seconds=$TIMEOUT_SECONDS
EOF

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$STAMP")"
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
    printf '  "selection_mode": "%s",\n' "$(json_escape "$SELECTION_MODE")"
    printf '  "selected_prefix": "%s",\n' "$(json_escape "$SELECTED_PREFIX")"
    printf '  "selected_stamp": "%s",\n' "$(json_escape "$SELECTED_STAMP")"
    printf '  "db_backup": "%s",\n' "$(json_escape "$SELECTED_DB_BACKUP")"
    printf '  "files_backup": "%s",\n' "$(json_escape "$SELECTED_FILES_BACKUP")"
    printf '  "manifest_txt": %s,\n' "$(if [[ -n "${SELECTED_MANIFEST_TXT:-}" ]]; then printf '"%s"' "$(json_escape "$SELECTED_MANIFEST_TXT")"; else printf 'null'; fi)"
    printf '  "manifest_json": %s,\n' "$(if [[ -n "${SELECTED_MANIFEST_JSON:-}" ]]; then printf '"%s"' "$(json_escape "$SELECTED_MANIFEST_JSON")"; else printf 'null'; fi)"
    printf '  "snapshot_enabled": %s,\n' "$([[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]] && echo true || echo false)"
    printf '  "no_start": %s,\n' "$([[ $NO_START -eq 1 ]] && echo true || echo false)"
    printf '  "skip_http_probe": %s,\n' "$([[ $SKIP_HTTP_PROBE -eq 1 ]] && echo true || echo false)"
    printf '  "timeout_seconds": %s\n' "$TIMEOUT_SECONDS"
    printf '}\n'
  } > "$ROLLBACK_PLAN_JSON"
}

on_error() {
  local exit_code="$?"
  trap - ERR

  if [[ ${ERROR_HANDLER_ACTIVE:-0} -eq 1 ]]; then
    exit "$exit_code"
  fi

  ERROR_HANDLER_ACTIVE=1

  if [[ ${BUNDLE_ON_FAIL:-1} -eq 1 && -n "${FAILURE_BUNDLE_PATH:-}" ]]; then
    warn "Rollback failed, collecting a support bundle"
    run_support_bundle_capture \
      "$SCRIPT_DIR" \
      "$ESPO_ENV" \
      "$ENV_FILE" \
      "$FAILURE_BUNDLE_PATH" \
      "Collected a support bundle for rollback failure analysis" \
      "Could not collect the rollback support bundle automatically"
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
DB_BACKUP_ARG=""
FILES_BACKUP_ARG=""
FORCE=0
CONFIRM_PROD=""
DRY_RUN=0
SNAPSHOT_BEFORE_ROLLBACK=1
NO_START=0
SKIP_HTTP_PROBE=0
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --force)
      FORCE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --dry-run)
      DRY_RUN=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --confirm-prod)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--confirm-prod must be followed by prod"
      [[ "${POSITIONAL_ARGS[1]}" == "prod" ]] || die "--confirm-prod accepts only the value prod"
      CONFIRM_PROD="prod"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --db-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--db-backup must be followed by a path"
      DB_BACKUP_ARG="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--files-backup must be followed by a path"
      FILES_BACKUP_ARG="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --no-snapshot)
      SNAPSHOT_BEFORE_ROLLBACK=0
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --no-start)
      NO_START=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-http-probe)
      SKIP_HTTP_PROBE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --timeout)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--timeout must be followed by a number of seconds"
      TIMEOUT_SECONDS="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
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

if [[ -n "$DB_BACKUP_ARG" || -n "$FILES_BACKUP_ARG" ]]; then
  [[ -n "$DB_BACKUP_ARG" && -n "$FILES_BACKUP_ARG" ]] || die "Manual rollback requires both --db-backup and --files-backup"
fi

if [[ $DRY_RUN -eq 1 ]]; then
  rollback_plan_args=(
    rollback-plan
    --scope "$ESPO_ENV"
    --project-dir "$ROOT_DIR"
    --compose-file "$ROOT_DIR/compose.yaml"
    --timeout "$TIMEOUT_SECONDS"
  )

  if [[ -n "${ENV_FILE:-}" ]]; then
    rollback_plan_args+=(--env-file "$ENV_FILE")
  fi
  if [[ -n "$DB_BACKUP_ARG" ]]; then
    rollback_plan_args+=(--db-backup "$DB_BACKUP_ARG" --files-backup "$FILES_BACKUP_ARG")
  fi
  if [[ $SNAPSHOT_BEFORE_ROLLBACK -eq 0 ]]; then
    rollback_plan_args+=(--no-snapshot)
  fi
  if [[ $NO_START -eq 1 ]]; then
    rollback_plan_args+=(--no-start)
  fi
  if [[ $SKIP_HTTP_PROBE -eq 1 ]]; then
    rollback_plan_args+=(--skip-http-probe)
  fi

  run_espops "${rollback_plan_args[@]}"
  exit $?
fi

acquire_operation_lock rollback
resolve_env_file
load_env
ensure_runtime_dirs
require_destructive_approval "rollback" "$FORCE" "$CONFIRM_PROD"
acquire_maintenance_lock rollback
require_compose

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
REPORT_RETENTION="${REPORT_RETENTION_DAYS:-30}"
STAMP="$(next_unique_stamp \
  "$REPORTS_DIR/${NAME_PREFIX}_pre-rollback___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_pre-rollback___STAMP__.json" \
  "$REPORTS_DIR/${NAME_PREFIX}_post-rollback___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_post-rollback___STAMP__.json" \
  "$REPORTS_DIR/${NAME_PREFIX}_rollback-plan___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_rollback-plan___STAMP__.json" \
  "$SUPPORT_DIR/${NAME_PREFIX}_rollback-failure___STAMP__.tar.gz")"
PRE_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_pre-rollback_${STAMP}.txt"
PRE_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_pre-rollback_${STAMP}.json"
POST_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_post-rollback_${STAMP}.txt"
POST_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_post-rollback_${STAMP}.json"
ROLLBACK_PLAN_TXT="$REPORTS_DIR/${NAME_PREFIX}_rollback-plan_${STAMP}.txt"
ROLLBACK_PLAN_JSON="$REPORTS_DIR/${NAME_PREFIX}_rollback-plan_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SUPPORT_DIR/${NAME_PREFIX}_rollback-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0
BUNDLE_ON_FAIL=1
# shellcheck disable=SC2034
READINESS_TIMEOUT_BUDGET="$TIMEOUT_SECONDS"

trap 'on_error' ERR

if [[ -n "$DB_BACKUP_ARG" ]]; then
  SELECTION_MODE="manual"
  validate_manual_backup_pair "$DB_BACKUP_ARG" "$FILES_BACKUP_ARG"
else
  SELECTION_MODE="auto-latest-valid"
  if ! select_latest_valid_backup_set; then
    die "No valid backup set was found for rollback in $BACKUP_ROOT_ABS"
  fi
fi

write_rollback_plan_reports
print_context

echo "[info] Backup-set selection mode: $SELECTION_MODE"
echo "[info] Selected prefix: $SELECTED_PREFIX"
echo "[info] Selected timestamp: $SELECTED_STAMP"
echo "[info] Database backup: $SELECTED_DB_BACKUP"
echo "[info] Files backup: $SELECTED_FILES_BACKUP"
if [[ -n "${SELECTED_MANIFEST_TXT:-}" ]]; then
  echo "[info] Text manifest: $SELECTED_MANIFEST_TXT"
fi
if [[ -n "${SELECTED_MANIFEST_JSON:-}" ]]; then
  echo "[info] JSON manifest: $SELECTED_MANIFEST_JSON"
fi

echo "[1/7] Capturing the current contour status"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$PRE_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$PRE_REPORT_JSON"

echo "[2/7] Starting the database container when needed"
if ! service_is_running db; then
  note "The DB container was not running, starting db temporarily for rollback"
  compose up -d db
fi
wait_for_service_ready_with_shared_timeout READINESS_TIMEOUT_BUDGET db "rollback procedure"

echo "[3/7] Stopping application services before rollback"
stop_app_services

echo "[4/7] Taking an emergency snapshot of the current state before rollback"
if [[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]]; then
  ESPO_SHELL_EXEC_CONTEXT=1 ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"
else
  echo "Emergency snapshot skipped because of --no-snapshot"
fi

echo "[5/7] Restoring the database"
if [[ "$ESPO_ENV" == "prod" ]]; then
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$SELECTED_DB_BACKUP" --force --confirm-prod prod --no-snapshot --no-stop --no-start
else
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$SELECTED_DB_BACKUP" --force --no-snapshot --no-stop --no-start
fi

echo "[6/7] Restoring files"
if [[ "$ESPO_ENV" == "prod" ]]; then
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$SELECTED_FILES_BACKUP" --force --confirm-prod prod --no-snapshot --no-stop --no-start
else
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$SELECTED_FILES_BACKUP" --force --no-snapshot --no-stop --no-start
fi

echo "[7/7] Returning the contour to working state"
if [[ $NO_START -eq 0 ]]; then
  compose up -d
  wait_for_application_stack_with_shared_timeout READINESS_TIMEOUT_BUDGET "rollback procedure"

  if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
    http_probe "$SITE_URL"
  else
    echo "HTTP probe skipped because of --skip-http-probe"
  fi
else
  echo "Contour left stopped because of --no-start"
fi

ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$POST_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$POST_REPORT_JSON"
cleanup_old_files "$REPORTS_DIR" "$REPORT_RETENTION" '*.txt' '*.json'

trap - ERR

echo "Rollback completed successfully"
echo "Rollback plan:         $ROLLBACK_PLAN_TXT"
echo "Pre-rollback report:     $PRE_REPORT_TXT"
echo "Post-rollback report:  $POST_REPORT_TXT"
