#!/usr/bin/env bash
set -Eeuo pipefail

# Shell owns operator guardrails and app-service orchestration.
# Go owns DB backup preflight, checksum/gzip validation and reset/import.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/restore-db.sh /path/to/manifest.json [--dry-run]
  ./scripts/restore-db.sh <dev|prod> /path/to/backup.sql.gz [--force] [--confirm-prod prod] [--dry-run] [--no-snapshot] [--snapshot-before-restore] [--no-stop] [--no-start]

Examples:
  ESPOPS_DB_CONTAINER=espocrm-prod-db-1 ESPOPS_DB_NAME=espocrm ESPOPS_DB_USER=espocrm ESPOPS_DB_PASSWORD_FILE=/run/secrets/db-password ./scripts/restore-db.sh /opt/espocrm-data/backups/prod/manifests/espocrm-prod_YYYY-MM-DD_HH-MM-SS.manifest.json --dry-run
  ./scripts/restore-db.sh dev /opt/espocrm-data/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz --force
  ./scripts/restore-db.sh prod /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --force --confirm-prod prod
  ./scripts/restore-db.sh prod /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --force --confirm-prod prod --no-snapshot
  ./scripts/restore-db.sh prod /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --force --confirm-prod prod --no-start
EOF
}

run_go_restore_db() {
  local args=(restore-db --db-container "$DB_CONTAINER")

  if [[ -n "${MANIFEST_JSON:-}" ]]; then
    args+=(--manifest "$MANIFEST_JSON")
  else
    args+=(--db-backup "$BACKUP_FILE")
  fi

  if [[ ${DRY_RUN:-0} -eq 1 ]]; then
    args+=(--dry-run)
  fi

  run_espops "${args[@]}"
}

write_restore_db_plan_reports() {
  local source_kind="direct_backup"
  local next_step="After restore, verify the contour with status or overview"

  if [[ $DRY_RUN -eq 1 ]]; then
    next_step="Run the same command without --dry-run to execute the database restore"
  elif [[ $NO_START -eq 1 ]]; then
    next_step="After restore, start the application services manually and verify the contour with status"
  fi

  cat > "$RESTORE_PLAN_TXT" <<EOF
EspoCRM database restore plan
created_at=$STAMP
command=restore-db
contour=$ESPO_ENV
compose_project=$COMPOSE_PROJECT_NAME
env_file=$(basename "$ENV_FILE")
source_kind=$source_kind
db_backup=$BACKUP_FILE
db_container=$DB_CONTAINER
db_name=$DB_NAME
db_user=$DB_USER
snapshot_enabled=$([[ $SNAPSHOT_BEFORE_RESTORE -eq 1 ]] && echo true || echo false)
no_stop=$([[ $NO_STOP -eq 1 ]] && echo true || echo false)
no_start=$([[ $NO_START -eq 1 ]] && echo true || echo false)
dry_run=$([[ $DRY_RUN -eq 1 ]] && echo true || echo false)
changes=replace target database contents from selected backup
non_changes=does not modify files storage
next_step=$next_step
EOF

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$STAMP")"
    printf '  "command": "restore-db",\n'
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
    printf '  "source_kind": "%s",\n' "$(json_escape "$source_kind")"
    printf '  "db_backup": "%s",\n' "$(json_escape "$BACKUP_FILE")"
    printf '  "target": {\n'
    printf '    "db_container": "%s",\n' "$(json_escape "$DB_CONTAINER")"
    printf '    "db_name": "%s",\n' "$(json_escape "$DB_NAME")"
    printf '    "db_user": "%s"\n' "$(json_escape "$DB_USER")"
    printf '  },\n'
    printf '  "flags": {\n'
    printf '    "snapshot_enabled": %s,\n' "$(json_bool "$SNAPSHOT_BEFORE_RESTORE")"
    printf '    "no_stop": %s,\n' "$(json_bool "$NO_STOP")"
    printf '    "no_start": %s,\n' "$(json_bool "$NO_START")"
    printf '    "dry_run": %s\n' "$(json_bool "$DRY_RUN")"
    printf '  },\n'
    printf '  "changes": [\n'
    printf '    "replace target database contents from selected backup"\n'
    printf '  ],\n'
    printf '  "non_changes": [\n'
    printf '    "does not modify files storage"\n'
    printf '  ],\n'
    printf '  "next_step": "%s"\n' "$(json_escape "$next_step")"
    printf '}\n'
  } > "$RESTORE_PLAN_JSON"
}

if [[ $# -gt 0 && "${1:-}" != "dev" && "${1:-}" != "prod" && "${1:-}" != "-h" && "${1:-}" != "--help" ]]; then
  DIRECT_DRY_RUN=0
  if [[ $# -eq 2 && "${2:-}" == "--dry-run" ]]; then
    DIRECT_DRY_RUN=1
  elif [[ $# -ne 1 ]]; then
    echo "usage: $0 MANIFEST.json [--dry-run]" >&2
    exit 2
  fi

  MANIFEST_JSON="$(caller_path "$1")"
  DRY_RUN="$DIRECT_DRY_RUN"
  DB_CONTAINER="${ESPOPS_DB_CONTAINER:-}"

  [[ -f "$MANIFEST_JSON" ]] || die "JSON manifest not found: $MANIFEST_JSON"
  run_go_restore_db
  exit $?
fi

parse_contour_arg "$@"

FORCE=0
CONFIRM_PROD=""
NO_STOP=0
NO_START=0
SNAPSHOT_BEFORE_RESTORE=1
BACKUP_ARG=""
BACKUP_FILE=""
MANIFEST_JSON=""
DRY_RUN=0

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --force)
      FORCE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --confirm-prod)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--confirm-prod must be followed by prod"
      [[ "${POSITIONAL_ARGS[1]}" == "prod" ]] || die "--confirm-prod accepts only the value prod"
      CONFIRM_PROD="prod"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --no-snapshot)
      SNAPSHOT_BEFORE_RESTORE=0
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --snapshot-before-restore)
      SNAPSHOT_BEFORE_RESTORE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --dry-run)
      DRY_RUN=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --no-stop)
      NO_STOP=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --no-start)
      NO_START=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
    *)
      if [[ -n "$BACKUP_ARG" ]]; then
        usage >&2
        die "More than one backup-file path was provided"
      fi
      BACKUP_ARG="${POSITIONAL_ARGS[0]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
  esac
done

require_explicit_contour

if [[ -z "$BACKUP_ARG" ]]; then
  usage >&2
  exit 1
fi

BACKUP_FILE="$(caller_path "$BACKUP_ARG")"
[[ -f "$BACKUP_FILE" ]] || die "Backup file not found: $BACKUP_FILE"

acquire_operation_lock restore-db
resolve_env_file
load_env
ensure_runtime_dirs

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
STAMP="$(next_unique_stamp \
  "$REPORTS_DIR/${NAME_PREFIX}_restore-db-plan___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_restore-db-plan___STAMP__.json")"
RESTORE_PLAN_TXT="$REPORTS_DIR/${NAME_PREFIX}_restore-db-plan_${STAMP}.txt"
RESTORE_PLAN_JSON="$REPORTS_DIR/${NAME_PREFIX}_restore-db-plan_${STAMP}.json"

if [[ $DRY_RUN -eq 1 ]]; then
  require_compose
  require_service_running db
  wait_for_service_ready db
  DB_CONTAINER="$(compose_service_container_id db)"
  [[ -n "$DB_CONTAINER" ]] || die "Could not resolve the db container"

  write_restore_db_plan_reports
  print_context
  echo "DB backup for dry-run database restore: $BACKUP_FILE"
  echo "Database restore plan: $RESTORE_PLAN_TXT"
  echo "JSON database restore plan: $RESTORE_PLAN_JSON"
  echo "Dry-run database restore via the Go backend"
  run_go_restore_db || die "Dry-run database restore via the Go backend failed"
  exit 0
fi

require_destructive_approval "restore-db" "$FORCE" "$CONFIRM_PROD"
acquire_maintenance_lock restore-db
require_compose
require_service_running db
wait_for_service_ready db
DB_CONTAINER="$(compose_service_container_id db)"
[[ -n "$DB_CONTAINER" ]] || die "Could not resolve the db container"

write_restore_db_plan_reports

SHOULD_RESTART_APP_SERVICES=0
APP_SERVICES_WERE_RUNNING=0
if [[ $NO_STOP -eq 0 ]]; then
  if app_services_running; then
    APP_SERVICES_WERE_RUNNING=1
    echo "Stopping application services before the database restore"
    stop_app_services
    if [[ $NO_START -eq 0 ]]; then
      SHOULD_RESTART_APP_SERVICES=1
    fi
  else
    echo "Application services are not running, nothing to stop"
  fi
fi

if [[ $NO_STOP -eq 1 ]] && app_services_running; then
  APP_SERVICES_WERE_RUNNING=1
fi

print_context
echo "Database restore plan: $RESTORE_PLAN_TXT"
echo "JSON database restore plan: $RESTORE_PLAN_JSON"

if [[ $SNAPSHOT_BEFORE_RESTORE -eq 1 ]]; then
  if [[ $NO_STOP -eq 1 && $APP_SERVICES_WERE_RUNNING -eq 1 ]]; then
    warn "The pre-restore emergency snapshot will be created without stopping application services"
  fi
  echo "Creating an emergency snapshot before the database restore"
  SNAPSHOT_ARGS=("$ESPO_ENV" "--skip-files")
  if [[ $NO_STOP -eq 1 ]]; then
    SNAPSHOT_ARGS+=("--no-stop")
  fi
  run_repo_script "$SCRIPT_DIR/backup.sh" "${SNAPSHOT_ARGS[@]}"
fi

echo "Database restore via the Go backend"
run_go_restore_db || die "Could not restore the database via the Go backend"

if [[ $SHOULD_RESTART_APP_SERVICES -eq 1 ]]; then
  echo "Starting application services after the database restore"
  start_app_services
elif [[ $NO_START -eq 1 ]]; then
  echo "Application services were left stopped because of --no-start"
fi

echo "Database restore completed"
