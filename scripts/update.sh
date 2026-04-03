#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт безопасного обновления контура EspoCRM.
# Он объединяет стандартный регламент обслуживания:
# - фиксирует текущее состояние контура;
# - прогоняет doctor-проверку;
# - создает backup перед изменениями;
# - подтягивает образы и перезапускает стек;
# - проверяет готовность сервисов;
# - автоматически собирает support bundle при ошибке.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/update.sh [dev|prod] [--skip-doctor] [--skip-backup] [--skip-pull] [--skip-http-probe] [--timeout SEC]

Примеры:
  ./scripts/update.sh prod
  ./scripts/update.sh dev --skip-backup
  ./scripts/update.sh prod --timeout 900
EOF
}

wait_for_application_stack() {
  local timeout_seconds="$1"

  wait_for_service_ready db "$timeout_seconds"
  wait_for_service_ready espocrm "$timeout_seconds"
  wait_for_service_ready espocrm-daemon "$timeout_seconds"
  wait_for_service_ready espocrm-websocket "$timeout_seconds"
}

create_failure_bundle() {
  local bundle_path="$1"

  set +e
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/support-bundle.sh" "$ESPO_ENV" --output "$bundle_path"
  local bundle_exit=$?
  set -e

  if [[ $bundle_exit -eq 0 ]]; then
    warn "Собран support bundle для разбора сбоя: $bundle_path"
  else
    warn "Не удалось собрать support bundle автоматически"
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
    warn "Обновление завершилось ошибкой, собираю support bundle"
    create_failure_bundle "$FAILURE_BUNDLE_PATH"
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
SKIP_DOCTOR=0
SKIP_BACKUP=0
SKIP_PULL=0
SKIP_HTTP_PROBE=0
BUNDLE_ON_FAIL=1
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --skip-doctor)
      SKIP_DOCTOR=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-backup)
      SKIP_BACKUP=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-pull)
      SKIP_PULL=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-http-probe)
      SKIP_HTTP_PROBE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --timeout)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --timeout должно идти число секунд"
      TIMEOUT_SECONDS="${POSITIONAL_ARGS[1]}"
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

[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || die "Таймаут должен быть целым числом секунд"

resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock update
require_compose

STAMP="$(date +%F_%H-%M-%S)"
BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
REPORT_RETENTION="${REPORT_RETENTION_DAYS:-30}"
PRE_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_pre-update_${STAMP}.txt"
PRE_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_pre-update_${STAMP}.json"
POST_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_post-update_${STAMP}.txt"
POST_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_post-update_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SUPPORT_DIR/${NAME_PREFIX}_update-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0

trap 'on_error' ERR

print_context

echo "[1/6] Фиксация текущего статуса контура"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$PRE_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$PRE_REPORT_JSON"

echo "[2/6] Предварительная проверка окружения"
if [[ $SKIP_DOCTOR -eq 0 ]]; then
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV"
else
  echo "Doctor-проверка пропущена по флагу --skip-doctor"
fi

echo "[3/6] Создание контрольной точки перед обновлением"
if [[ $SKIP_BACKUP -eq 0 ]]; then
  if ! service_is_running db; then
    note "Контейнер БД не был запущен, временно поднимаю db для backup"
    compose up -d db
    wait_for_service_ready db "$TIMEOUT_SECONDS"
  fi

  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"
else
  echo "Backup пропущен по флагу --skip-backup"
fi

echo "[4/6] Обновление образов"
if [[ $SKIP_PULL -eq 0 ]]; then
  compose pull
else
  echo "Pull образов пропущен по флагу --skip-pull"
fi

echo "[5/6] Перезапуск стека с актуальной конфигурацией"
compose up -d

echo "[6/6] Проверка готовности после обновления"
wait_for_application_stack "$TIMEOUT_SECONDS"

if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
  http_probe "$SITE_URL"
else
  echo "HTTP-проверка пропущена по флагу --skip-http-probe"
fi

ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$POST_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$POST_REPORT_JSON"
cleanup_old_files "$REPORTS_DIR" "$REPORT_RETENTION" '*.txt' '*.json'

trap - ERR

echo "Обновление завершено успешно"
echo "Pre-update отчет:  $PRE_REPORT_TXT"
echo "Post-update отчет: $POST_REPORT_TXT"
