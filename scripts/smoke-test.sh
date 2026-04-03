#!/usr/bin/env bash
set -Eeuo pipefail

# Smoke-test для быстрого прогона жизненного цикла:
# - поднять стек;
# - дождаться готовности сервисов;
# - проверить HTTP-доступность;
# - создать backup;
# - проверить backup;
# - убрать временное окружение.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/smoke-test.sh [dev|prod] [--from-example] [--timeout SEC] [--keep-artifacts]

Примеры:
  ./scripts/smoke-test.sh dev --from-example
  ./scripts/smoke-test.sh prod --from-example --timeout 900
  ./scripts/smoke-test.sh dev --keep-artifacts
EOF
}

cleanup() {
  local exit_code="$1"

  if [[ -n "${SMOKE_ENV_FILE:-}" && -f "${SMOKE_ENV_FILE:-}" ]]; then
    if [[ ${KEEP_ARTIFACTS:-0} -eq 0 ]]; then
      ENV_FILE="$SMOKE_ENV_FILE" compose down >/dev/null 2>&1 || true
      rm -f "$SMOKE_ENV_FILE"
      if [[ -n "${SMOKE_DB_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_DB_STORAGE_ABS"
      fi
      if [[ -n "${SMOKE_ESPO_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_ESPO_STORAGE_ABS"
      fi
      if [[ -n "${SMOKE_BACKUP_ROOT_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_BACKUP_ROOT_ABS"
      fi
    else
      warn "Временное smoke-окружение сохранено по флагу --keep-artifacts"
      warn "Env-файл: $SMOKE_ENV_FILE"
    fi
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
USE_EXAMPLE_ENV=0
KEEP_ARTIFACTS=0
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --from-example)
      USE_EXAMPLE_ENV=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --keep-artifacts)
      KEEP_ARTIFACTS=1
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

require_compose

SMOKE_ENV_FILE=""

if [[ $USE_EXAMPLE_ENV -eq 1 ]]; then
  EXAMPLE_ENV_FILE="$ROOT_DIR/.env.${ESPO_ENV}.example"
  [[ -f "$EXAMPLE_ENV_FILE" ]] || die "Не найден шаблон env-файла: $EXAMPLE_ENV_FILE"

  SMOKE_ENV_FILE="$(mktemp "$ROOT_DIR/.env.smoke.${ESPO_ENV}.XXXXXX")"
  cp "$EXAMPLE_ENV_FILE" "$SMOKE_ENV_FILE"

  if [[ "$ESPO_ENV" == "prod" ]]; then
    SMOKE_APP_PORT=18080
    SMOKE_WS_PORT=18081
  else
    SMOKE_APP_PORT=18088
    SMOKE_WS_PORT=18089
  fi

  set_env_value "$SMOKE_ENV_FILE" "COMPOSE_PROJECT_NAME" "espo-smoke-$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "DB_STORAGE_DIR" "./storage/smoke/$ESPO_ENV/db"
  set_env_value "$SMOKE_ENV_FILE" "ESPO_STORAGE_DIR" "./storage/smoke/$ESPO_ENV/espo"
  set_env_value "$SMOKE_ENV_FILE" "BACKUP_ROOT" "./backups/smoke/$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "BACKUP_NAME_PREFIX" "espocrm-smoke-$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "APP_PORT" "$SMOKE_APP_PORT"
  set_env_value "$SMOKE_ENV_FILE" "WS_PORT" "$SMOKE_WS_PORT"
  set_env_value "$SMOKE_ENV_FILE" "SITE_URL" "http://127.0.0.1:$SMOKE_APP_PORT"
  set_env_value "$SMOKE_ENV_FILE" "WS_PUBLIC_URL" "ws://127.0.0.1:$SMOKE_WS_PORT"

  export ENV_FILE="$SMOKE_ENV_FILE"
  resolve_env_file
else
  resolve_env_file
  warn "Smoke-test будет выполняться на реальном контуре '$ESPO_ENV'"
fi

trap 'cleanup $?' EXIT

load_env
ensure_runtime_dirs

if [[ $USE_EXAMPLE_ENV -eq 1 ]]; then
  SMOKE_DB_STORAGE_ABS="$(root_path "$DB_STORAGE_DIR")"
  SMOKE_ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
  SMOKE_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

  safe_remove_tree "$SMOKE_DB_STORAGE_ABS"
  safe_remove_tree "$SMOKE_ESPO_STORAGE_ABS"
  safe_remove_tree "$SMOKE_BACKUP_ROOT_ABS"
  ensure_runtime_dirs
fi

print_context

echo "[1/5] Запуск smoke-стека"
compose up -d

echo "[2/5] Ожидание готовности сервисов"
wait_for_service_ready db "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm-daemon "$TIMEOUT_SECONDS"
wait_for_service_ready espocrm-websocket "$TIMEOUT_SECONDS"

echo "[3/5] Проверка HTTP-доступности приложения: $SITE_URL"
http_probe "$SITE_URL"

echo "[4/5] Создание резервной копии"
run_repo_script "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"

echo "[5/5] Проверка резервной копии"
run_repo_script "$SCRIPT_DIR/verify-backup.sh" "$ESPO_ENV"

echo "Smoke-test завершен успешно"
