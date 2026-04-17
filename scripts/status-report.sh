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
Usage: ./scripts/status-report.sh <dev|prod> [--json] [--output PATH]

Examples:
  ./scripts/status-report.sh prod
  ./scripts/status-report.sh dev --json
  ./scripts/status-report.sh prod --output /tmp/prod-status.txt
EOF
}

emit_output() {
  local content="$1"
  local output_path="${2:-}"
  local success_message="${3:-}"

  if [[ -n "$output_path" ]]; then
    mkdir -p "$(dirname "$output_path")"
    printf '%s\n' "$content" > "$output_path"
    if [[ -n "$success_message" ]]; then
      printf '%s\n' "$success_message"
    fi
  else
    printf '%s\n' "$content"
  fi
}

service_status_or_default() {
  local service="$1"
  local status

  if [[ ${DOCKER_DAEMON_AVAILABLE:-0} -eq 0 ]]; then
    printf 'daemon_unavailable\n'
    return
  fi

  status="$(compose_service_status "$service" 2>/dev/null || true)"
  if [[ -n "$status" ]]; then
    printf '%s\n' "$status"
  else
    printf 'not_created\n'
  fi
}

render_lock_entries_json() {
  local first_lock=1
  local lock_entry lock_name lock_pid lock_state lock_path

  printf '['
  for lock_entry in "${LOCK_ENTRIES[@]}"; do
    IFS='|' read -r lock_name lock_pid lock_state lock_path <<< "$lock_entry"
    if [[ $first_lock -eq 0 ]]; then
      printf ','
    fi
    printf '\n      {"file": "%s", "pid": %s, "state": "%s", "path": "%s"}' \
      "$(json_escape "$lock_name")" \
      "$(json_number_or_null "$lock_pid")" \
      "$(json_escape "$lock_state")" \
      "$(json_escape "$lock_path")"
    first_lock=0
  done

  if [[ ${#LOCK_ENTRIES[@]} -gt 0 ]]; then
    printf '\n    '
  fi
  printf ']'
}

render_lock_entries_text() {
  local lock_entry lock_name lock_pid lock_state lock_path

  if [[ ${#LOCK_ENTRIES[@]} -eq 0 ]]; then
    printf '  none'
    return
  fi

  for lock_entry in "${LOCK_ENTRIES[@]}"; do
    IFS='|' read -r lock_name lock_pid lock_state lock_path <<< "$lock_entry"
    printf '\n  %s: pid=%s, state=%s, path=%s' "$lock_name" "${lock_pid:-n/a}" "$lock_state" "$lock_path"
  done
}

parse_contour_arg "$@"
OUTPUT_MODE="text"
OUTPUT_PATH=""
DOCKER_DAEMON_AVAILABLE=0

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --json)
      OUTPUT_MODE="json"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--output must be followed by a path"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
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

acquire_operation_lock status-report
resolve_env_file
load_env
ensure_runtime_dirs
require_compose --skip-daemon-check

if docker info >/dev/null 2>&1; then
  DOCKER_DAEMON_AVAILABLE=1
fi

STAMP="$(date +%F_%H-%M-%S)"
DB_STORAGE_ABS="$(root_path "$DB_STORAGE_DIR")"
ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
LOCKS_DIR="$BACKUP_ROOT_ABS/locks"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"

DB_STATUS="$(service_status_or_default db)"
ESPO_STATUS="$(service_status_or_default espocrm)"
DAEMON_STATUS="$(service_status_or_default espocrm-daemon)"
WS_STATUS="$(service_status_or_default espocrm-websocket)"

LATEST_DB_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/db" '*.sql.gz' || true)"
LATEST_FILES_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/files" '*.tar.gz' || true)"
LATEST_MANIFEST_JSON="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.json' || true)"
LATEST_MANIFEST_TXT="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.txt' || true)"
LATEST_REPORT_TXT="$(latest_backup_file "$REPORTS_DIR" '*.txt' || true)"
LATEST_REPORT_JSON="$(latest_backup_file "$REPORTS_DIR" '*.json' || true)"
LATEST_SUPPORT_BUNDLE="$(latest_backup_file "$SUPPORT_DIR" '*.tar.gz' || true)"

DB_STORAGE_SIZE="$(directory_size_human "$DB_STORAGE_ABS")"
ESPO_STORAGE_SIZE="$(directory_size_human "$ESPO_STORAGE_ABS")"
BACKUP_ROOT_SIZE="$(directory_size_human "$BACKUP_ROOT_ABS")"
REPORTS_DIR_SIZE="$(directory_size_human "$REPORTS_DIR")"
SUPPORT_DIR_SIZE="$(directory_size_human "$SUPPORT_DIR")"
REPORT_RETENTION="${REPORT_RETENTION_DAYS:-30}"
SUPPORT_RETENTION="${SUPPORT_RETENTION_DAYS:-14}"

ACTIVE_LOCK_COUNT=0
LOCK_ENTRIES=()
while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  LOCK_ENTRIES+=("$entry")
  IFS='|' read -r _lock_name _lock_pid lock_state _lock_path <<< "$entry"
  if [[ "$lock_state" == "active" ]]; then
    ACTIVE_LOCK_COUNT=$((ACTIVE_LOCK_COUNT + 1))
  fi
done < <(collect_lock_entries "$LOCKS_DIR")

  if [[ "$OUTPUT_MODE" == "json" ]]; then
  REPORT_CONTENT="$(
    {
      printf '{\n'
      printf '  "canonical": false,\n'
      printf '  "contract_level": "non_canonical_shell",\n'
      printf '  "machine_contract": false,\n'
      printf '  "created_at": "%s",\n' "$(json_escape "$STAMP")"
      printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
      printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
      printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
      printf '  "site_url": "%s",\n' "$(json_escape "$SITE_URL")"
      printf '  "ws_public_url": "%s",\n' "$(json_escape "$WS_PUBLIC_URL")"
      printf '  "espocrm_image": "%s",\n' "$(json_escape "$ESPOCRM_IMAGE")"
      printf '  "retention": {\n'
      printf '    "backup_days": %s,\n' "$(json_number_or_null "${BACKUP_RETENTION_DAYS:-}")"
      printf '    "report_days": %s,\n' "$(json_number_or_null "$REPORT_RETENTION")"
      printf '    "support_days": %s\n' "$(json_number_or_null "$SUPPORT_RETENTION")"
      printf '  },\n'
      printf '  "storage": {\n'
      printf '    "db_path": "%s",\n' "$(json_escape "$DB_STORAGE_ABS")"
      printf '    "db_size": "%s",\n' "$(json_escape "$DB_STORAGE_SIZE")"
      printf '    "espo_path": "%s",\n' "$(json_escape "$ESPO_STORAGE_ABS")"
      printf '    "espo_size": "%s",\n' "$(json_escape "$ESPO_STORAGE_SIZE")"
      printf '    "backup_root": "%s",\n' "$(json_escape "$BACKUP_ROOT_ABS")"
      printf '    "backup_root_size": "%s",\n' "$(json_escape "$BACKUP_ROOT_SIZE")"
      printf '    "reports_dir": "%s",\n' "$(json_escape "$REPORTS_DIR")"
      printf '    "reports_dir_size": "%s",\n' "$(json_escape "$REPORTS_DIR_SIZE")"
      printf '    "support_dir": "%s",\n' "$(json_escape "$SUPPORT_DIR")"
      printf '    "support_dir_size": "%s"\n' "$(json_escape "$SUPPORT_DIR_SIZE")"
      printf '  },\n'
      printf '  "services": {\n'
      printf '    "db": "%s",\n' "$(json_escape "$DB_STATUS")"
      printf '    "espocrm": "%s",\n' "$(json_escape "$ESPO_STATUS")"
      printf '    "espocrm_daemon": "%s",\n' "$(json_escape "$DAEMON_STATUS")"
      printf '    "espocrm_websocket": "%s"\n' "$(json_escape "$WS_STATUS")"
      printf '  },\n'
      printf '  "maintenance": {\n'
      printf '    "locks_dir": "%s",\n' "$(json_escape "$LOCKS_DIR")"
      printf '    "active_lock_count": %s,\n' "$ACTIVE_LOCK_COUNT"
      printf '    "locks": '
      render_lock_entries_json
      printf '\n'
      printf '  },\n'
      printf '  "latest_artifacts": {\n'
      printf '    "db": %s,\n' "$(json_value_or_null "$LATEST_DB_BACKUP")"
      printf '    "files": %s,\n' "$(json_value_or_null "$LATEST_FILES_BACKUP")"
      printf '    "manifest_json": %s,\n' "$(json_value_or_null "$LATEST_MANIFEST_JSON")"
      printf '    "manifest_txt": %s,\n' "$(json_value_or_null "$LATEST_MANIFEST_TXT")"
      printf '    "report_txt": %s,\n' "$(json_value_or_null "$LATEST_REPORT_TXT")"
      printf '    "report_json": %s,\n' "$(json_value_or_null "$LATEST_REPORT_JSON")"
      printf '    "support_bundle": %s\n' "$(json_value_or_null "$LATEST_SUPPORT_BUNDLE")"
      printf '  }\n'
      printf '}\n'
    }
  )"
else
  LOCKS_TEXT="$(render_lock_entries_text)"

  REPORT_CONTENT="$(
    cat <<EOF
EspoCRM contour report
Date:            $STAMP
Contour:          $ESPO_ENV
Compose project:  $COMPOSE_PROJECT_NAME
Env file:  $ENV_FILE
Site URL:     $SITE_URL
WebSocket URL: $WS_PUBLIC_URL
EspoCRM image:   $ESPOCRM_IMAGE

Retention:
  Backup days:        ${BACKUP_RETENTION_DAYS:-n/a}
  Report days:                $REPORT_RETENTION
  Support bundle days: $SUPPORT_RETENTION

Services:
  db:                 $DB_STATUS
  espocrm:            $ESPO_STATUS
  espocrm-daemon:     $DAEMON_STATUS
  espocrm-websocket:  $WS_STATUS

Directories:
  DB storage:             $DB_STORAGE_ABS ($DB_STORAGE_SIZE)
  EspoCRM storage:        $ESPO_STORAGE_ABS ($ESPO_STORAGE_SIZE)
  Backup root:  $BACKUP_ROOT_ABS ($BACKUP_ROOT_SIZE)
  Reports directory:          $REPORTS_DIR ($REPORTS_DIR_SIZE)
  Support directory:      $SUPPORT_DIR ($SUPPORT_DIR_SIZE)

Maintenance:
  Locks directory:  $LOCKS_DIR
  Active locks: $ACTIVE_LOCK_COUNT
  Lock entries:$LOCKS_TEXT

Latest artifacts:
  Database backup:      ${LATEST_DB_BACKUP:-n/a}
  Files backup:  ${LATEST_FILES_BACKUP:-n/a}
  JSON manifest:           ${LATEST_MANIFEST_JSON:-n/a}
  Text manifest:      ${LATEST_MANIFEST_TXT:-n/a}
  Text report:         ${LATEST_REPORT_TXT:-n/a}
  JSON report:              ${LATEST_REPORT_JSON:-n/a}
  Support bundle:   ${LATEST_SUPPORT_BUNDLE:-n/a}
EOF
  )"
fi

emit_output "$REPORT_CONTENT" "$OUTPUT_PATH" "Report saved: $OUTPUT_PATH"
