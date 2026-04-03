#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт формирует краткий статус-отчет по контуру:
# - конфигурация;
# - статусы сервисов;
# - размеры каталогов;
# - последние бэкапы.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/status-report.sh [dev|prod] [--json] [--output PATH]

Примеры:
  ./scripts/status-report.sh prod
  ./scripts/status-report.sh dev --json
  ./scripts/status-report.sh prod --output /tmp/prod-status.txt
EOF
}

json_value_or_null() {
  local value="${1:-}"

  if [[ -n "$value" ]]; then
    printf '"%s"' "$(json_escape "$value")"
  else
    printf 'null'
  fi
}

json_number_or_null() {
  local value="${1:-}"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
  else
    printf 'null'
  fi
}

service_status_or_default() {
  local service="$1"
  local status

  status="$(compose_service_status "$service" 2>/dev/null || true)"
  if [[ -n "$status" ]]; then
    printf '%s\n' "$status"
  else
    printf 'not_created\n'
  fi
}

collect_lock_entries() {
  local locks_dir="$1"
  local lock_file pid state

  while IFS= read -r lock_file; do
    [[ -n "$lock_file" ]] || continue
    pid="$(lock_file_owner_pid "$lock_file" || true)"
    state="$(lock_file_state "$lock_file")"
    printf '%s|%s|%s|%s\n' "$(basename "$lock_file")" "$pid" "$state" "$lock_file"
  done < <(list_lock_files "$locks_dir")
}

parse_contour_arg "$@"
OUTPUT_MODE="text"
OUTPUT_PATH=""

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --json)
      OUTPUT_MODE="json"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --output должен быть путь"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Неизвестный аргумент: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

resolve_env_file
load_env
ensure_runtime_dirs
require_compose

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
      printf '    "locks": ['
      first_lock=1
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
      printf ']\n'
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
  if [[ ${#LOCK_ENTRIES[@]} -eq 0 ]]; then
    LOCKS_TEXT='  none'
  else
    LOCKS_TEXT=''
    for lock_entry in "${LOCK_ENTRIES[@]}"; do
      IFS='|' read -r lock_name lock_pid lock_state lock_path <<< "$lock_entry"
      LOCKS_TEXT+=$'\n'"  $lock_name: pid=${lock_pid:-n/a}, state=$lock_state, path=$lock_path"
    done
  fi

  REPORT_CONTENT="$(
    cat <<EOF
Отчет по контуру EspoCRM
Дата:            $STAMP
Контур:          $ESPO_ENV
Compose-проект:  $COMPOSE_PROJECT_NAME
Env-файл:        $ENV_FILE
Site URL:        $SITE_URL
WS URL:          $WS_PUBLIC_URL
Образ EspoCRM:   $ESPOCRM_IMAGE

Retention:
  Backup days:      ${BACKUP_RETENTION_DAYS:-n/a}
  Report days:      $REPORT_RETENTION
  Support days:     $SUPPORT_RETENTION

Сервисы:
  db:                 $DB_STATUS
  espocrm:            $ESPO_STATUS
  espocrm-daemon:     $DAEMON_STATUS
  espocrm-websocket:  $WS_STATUS

Каталоги:
  DB storage:    $DB_STORAGE_ABS ($DB_STORAGE_SIZE)
  ESPO storage:  $ESPO_STORAGE_ABS ($ESPO_STORAGE_SIZE)
  Backup root:   $BACKUP_ROOT_ABS ($BACKUP_ROOT_SIZE)
  Reports dir:   $REPORTS_DIR ($REPORTS_DIR_SIZE)
  Support dir:   $SUPPORT_DIR ($SUPPORT_DIR_SIZE)

Обслуживание:
  Locks dir:        $LOCKS_DIR
  Active locks:     $ACTIVE_LOCK_COUNT
  Lock entries:$LOCKS_TEXT

Последние артефакты:
  DB backup:       ${LATEST_DB_BACKUP:-n/a}
  Files backup:    ${LATEST_FILES_BACKUP:-n/a}
  Manifest JSON:   ${LATEST_MANIFEST_JSON:-n/a}
  Manifest TXT:    ${LATEST_MANIFEST_TXT:-n/a}
  Report TXT:      ${LATEST_REPORT_TXT:-n/a}
  Report JSON:     ${LATEST_REPORT_JSON:-n/a}
  Support bundle:  ${LATEST_SUPPORT_BUNDLE:-n/a}
EOF
  )"
fi

if [[ -n "$OUTPUT_PATH" ]]; then
  mkdir -p "$(dirname "$OUTPUT_PATH")"
  printf '%s\n' "$REPORT_CONTENT" > "$OUTPUT_PATH"
  echo "Отчет сохранен: $OUTPUT_PATH"
else
  printf '%s\n' "$REPORT_CONTENT"
fi
