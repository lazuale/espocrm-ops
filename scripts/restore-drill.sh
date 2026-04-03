#!/usr/bin/env bash
set -Eeuo pipefail

# Restore-drill для проверки, что последние backup действительно поднимаются.
# Скрипт:
# - берет backup выбранного контура;
# - разворачивает их во временный изолированный compose-проект;
# - дожидается готовности сервисов;
# - проверяет HTTP-доступность;
# - сохраняет drill-отчет в reports исходного контура.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/restore-drill.sh [dev|prod] [--db-backup PATH] [--files-backup PATH] [--timeout SEC] [--app-port PORT] [--ws-port PORT] [--skip-http-probe] [--keep-artifacts]

Примеры:
  ./scripts/restore-drill.sh prod
  ./scripts/restore-drill.sh dev --timeout 900
  ./scripts/restore-drill.sh prod --db-backup /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/restore-drill.sh dev --files-backup /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/restore-drill.sh prod --app-port 28080 --ws-port 28081 --keep-artifacts
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
    die "Порт $port уже занят, restore-drill не может использовать его для $label"
  else
    port_check_status=$?
  fi

  if [[ $port_check_status -eq 2 ]]; then
    warn "Не удалось автоматически проверить занятость порта $port, продолжаю без preflight-проверки"
  fi
}

create_failure_bundle() {
  local bundle_path="$1"

  [[ -n "${DRILL_ENV_FILE:-}" && -f "${DRILL_ENV_FILE:-}" ]] || return 0

  set +e
  ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/support-bundle.sh" "$ESPO_ENV" --output "$bundle_path"
  local bundle_exit=$?
  set -e

  if [[ $bundle_exit -eq 0 ]]; then
    warn "Собран support bundle restore-drill: $bundle_path"
  else
    warn "Не удалось собрать support bundle restore-drill автоматически"
  fi
}

on_error() {
  local exit_code="$?"
  trap - ERR

  if [[ ${ERROR_HANDLER_ACTIVE:-0} -eq 1 ]]; then
    exit "$exit_code"
  fi

  ERROR_HANDLER_ACTIVE=1

  if [[ ${BUNDLE_ON_FAIL:-1} -eq 1 && -n "${FAILURE_BUNDLE_PATH:-}" ]]; then
    warn "Restore-drill завершился ошибкой, собираю support bundle"
    create_failure_bundle "$FAILURE_BUNDLE_PATH"
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
      warn "Временный restore-drill контур сохранен по флагу --keep-artifacts"
      warn "Env-файл drill-контура: $DRILL_ENV_FILE"
      if [[ -n "${DRILL_SITE_URL:-}" ]]; then
        warn "URL drill-контура: $DRILL_SITE_URL"
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
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --db-backup должен идти путь к .sql.gz"
      DB_BACKUP_ARG="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --files-backup должен идти путь к .tar.gz"
      FILES_BACKUP_ARG="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --timeout)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --timeout должно идти число секунд"
      TIMEOUT_SECONDS="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --app-port)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --app-port должен идти номер порта"
      DRILL_APP_PORT="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --ws-port)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --ws-port должен идти номер порта"
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
      die "Неизвестный аргумент: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || die "Таймаут должен быть целым числом секунд"
[[ -z "$DRILL_APP_PORT" || "$DRILL_APP_PORT" =~ ^[0-9]+$ ]] || die "Порт приложения должен быть числом"
[[ -z "$DRILL_WS_PORT" || "$DRILL_WS_PORT" =~ ^[0-9]+$ ]] || die "Порт websocket должен быть числом"

resolve_env_file
load_env
ensure_runtime_dirs
require_compose

SOURCE_ENV_FILE="$ENV_FILE"
SOURCE_COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME"
SOURCE_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
SOURCE_REPORTS_DIR="$SOURCE_BACKUP_ROOT_ABS/reports"
SOURCE_SUPPORT_DIR="$SOURCE_BACKUP_ROOT_ABS/support"
SOURCE_NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
SOURCE_APP_PORT="$APP_PORT"
SOURCE_WS_PORT="$WS_PORT"

DB_BACKUP_FILE="$(
  if [[ -n "$DB_BACKUP_ARG" ]]; then
    caller_path "$DB_BACKUP_ARG"
  else
    latest_backup_file "$SOURCE_BACKUP_ROOT_ABS/db" '*.sql.gz'
  fi
)"
FILES_BACKUP_FILE="$(
  if [[ -n "$FILES_BACKUP_ARG" ]]; then
    caller_path "$FILES_BACKUP_ARG"
  else
    latest_backup_file "$SOURCE_BACKUP_ROOT_ABS/files" '*.tar.gz'
  fi
)"

[[ -n "$DB_BACKUP_FILE" && -f "$DB_BACKUP_FILE" ]] || die "Не найден backup базы данных для restore-drill"
[[ -n "$FILES_BACKUP_FILE" && -f "$FILES_BACKUP_FILE" ]] || die "Не найден backup файлов для restore-drill"

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

(( DRILL_APP_PORT >= 1 && DRILL_APP_PORT <= 65535 )) || die "Неверный APP_PORT для restore-drill: $DRILL_APP_PORT"
(( DRILL_WS_PORT >= 1 && DRILL_WS_PORT <= 65535 )) || die "Неверный WS_PORT для restore-drill: $DRILL_WS_PORT"
[[ "$DRILL_APP_PORT" != "$DRILL_WS_PORT" ]] || die "Порты APP и WS для restore-drill должны отличаться"

ensure_port_available "$DRILL_APP_PORT" "HTTP"
ensure_port_available "$DRILL_WS_PORT" "websocket"

STAMP="$(date +%F_%H-%M-%S)"
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
DRILL_REPORT_TXT="$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill_${STAMP}.txt"
DRILL_REPORT_JSON="$SOURCE_REPORTS_DIR/${SOURCE_NAME_PREFIX}_restore-drill_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SOURCE_SUPPORT_DIR/${SOURCE_NAME_PREFIX}_restore-drill-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0
BUNDLE_ON_FAIL=1

trap 'on_error' ERR
append_trap 'cleanup $?' EXIT

# На случай незавершенного предыдущего drill-прогона сначала убираем временный стек и каталоги.
compose down >/dev/null 2>&1 || true
safe_remove_tree "$DRILL_DB_STORAGE_ABS"
safe_remove_tree "$DRILL_ESPO_STORAGE_ABS"
safe_remove_tree "$DRILL_BACKUP_ROOT_ABS"
ensure_runtime_dirs

echo "Исходный контур: $ESPO_ENV"
echo "Исходный env-файл: $SOURCE_ENV_FILE"
echo "Исходный compose-проект: $SOURCE_COMPOSE_PROJECT_NAME"
echo "Backup БД для drill: $DB_BACKUP_FILE"
echo "Backup файлов для drill: $FILES_BACKUP_FILE"
print_context

echo "[1/6] Подготовка временного restore-drill контура"
echo "  HTTP URL: $SITE_URL"
echo "  WS URL:   $WS_PUBLIC_URL"

echo "[2/6] Запуск временной базы данных"
compose up -d db
wait_for_service_ready db "$TIMEOUT_SECONDS"

echo "[3/6] Восстановление базы данных во временный контур"
ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$DB_BACKUP_FILE" --no-stop --no-start

echo "[4/6] Восстановление файлов во временный контур"
ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$FILES_BACKUP_FILE" --no-stop --no-start

echo "[5/6] Запуск полного временного стека"
compose up -d
wait_for_service_ready db "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm-daemon "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm-websocket "$TIMEOUT_SECONDS"

echo "[6/6] Проверка готовности приложения после восстановления"
if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
  http_probe "$SITE_URL"
else
  echo "HTTP-проверка пропущена по флагу --skip-http-probe"
fi

ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$DRILL_REPORT_TXT"
ENV_FILE="$DRILL_ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$DRILL_REPORT_JSON"

trap - ERR

echo "Restore-drill завершен успешно"
echo "Отчет: $DRILL_REPORT_TXT"
echo "JSON-отчет: $DRILL_REPORT_JSON"
