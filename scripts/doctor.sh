#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт быстрой проверки сервера и конфигурации перед первым запуском
# или перед публикацией изменений в прод.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/doctor.sh [dev|prod|all] [--json]

Примеры:
  ./scripts/doctor.sh
  ./scripts/doctor.sh all
  ./scripts/doctor.sh prod
  ./scripts/doctor.sh dev --json
EOF
}

OK_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0
JSON_MODE=0
TARGET_SCOPE="all"
MIN_DOCKER_VERSION="24.0.0"
MIN_DOCKER_COMPOSE_VERSION="2.20.0"
declare -A LOADED_APP_PORTS=()
declare -A LOADED_WS_PORTS=()
declare -A LOADED_PROJECTS=()
declare -A LOADED_DB_STORAGE_DIRS=()
declare -A LOADED_ESPO_STORAGE_DIRS=()
declare -A LOADED_BACKUP_ROOTS=()
declare -A LOADED_IMAGES=()
declare -a FINDING_LEVELS=()
declare -a FINDING_MESSAGES=()

record_finding() {
  local level="$1"
  shift
  FINDING_LEVELS+=("$level")
  FINDING_MESSAGES+=("$*")
}

section() {
  [[ $JSON_MODE -eq 0 ]] || return 0
  echo
  echo "== $* =="
}

ok() {
  record_finding ok "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[ok] $*"
  OK_COUNT=$((OK_COUNT + 1))
}

warn() {
  record_finding warn "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[warn] $*"
  WARN_COUNT=$((WARN_COUNT + 1))
}

fail() {
  record_finding fail "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[fail] $*"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

json_findings() {
  local first=1
  local index

  printf '['
  for index in "${!FINDING_LEVELS[@]}"; do
    if [[ $first -eq 0 ]]; then
      printf ','
    fi
    printf '\n    {"level": "%s", "message": "%s"}' \
      "$(json_escape "${FINDING_LEVELS[$index]}")" \
      "$(json_escape "${FINDING_MESSAGES[$index]}")"
    first=0
  done

  if [[ ${#FINDING_LEVELS[@]} -gt 0 ]]; then
    printf '\n  '
  fi
  printf ']'
}

render_json_report() {
  local created_at
  created_at="$(date +%F_%H-%M-%S)"

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$created_at")"
    printf '  "target_scope": "%s",\n' "$(json_escape "$TARGET_SCOPE")"
    printf '  "success": %s,\n' "$([[ $FAIL_COUNT -eq 0 ]] && echo true || echo false)"
    printf '  "summary": {\n'
    printf '    "ok": %s,\n' "$OK_COUNT"
    printf '    "warn": %s,\n' "$WARN_COUNT"
    printf '    "fail": %s\n' "$FAIL_COUNT"
    printf '  },\n'
    printf '  "findings": '
    json_findings
    printf '\n}\n'
  }
}

check_sha256_tool() {
  if command_exists sha256sum; then
    ok "Доступен sha256sum для проверки целостности бэкапов"
  elif command_exists shasum; then
    ok "Доступен shasum для проверки целостности бэкапов"
  elif command_exists openssl; then
    ok "Доступен openssl для проверки целостности бэкапов"
  else
    fail "Не найден инструмент SHA-256: требуется sha256sum, shasum или openssl"
  fi
}

check_url() {
  local contour="$1"
  local name="$2"
  local value="$3"
  local pattern="$4"

  if [[ -z "$value" ]]; then
    fail "[$contour] Переменная $name не задана"
    return
  fi

  case "$value" in
    *change_me*|*YOUR_SERVER_IP*)
      fail "[$contour] Переменная $name содержит шаблонное значение: $value"
      return
      ;;
  esac

  if [[ "$value" =~ $pattern ]]; then
    ok "[$contour] Переменная $name выглядит корректно"
  else
    fail "[$contour] Переменная $name имеет неподходящий формат: $value"
  fi
}

check_secret_value() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ -z "$value" ]]; then
    fail "[$contour] Переменная $name не задана"
  elif [[ "$value" == *change_me* ]]; then
    fail "[$contour] Переменная $name содержит шаблонное значение"
  else
    ok "[$contour] Переменная $name заполнена"
  fi
}

check_integer_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    ok "[$contour] Переменная $name содержит целое число"
  else
    fail "[$contour] Переменная $name должна быть целым числом: $value"
  fi
}

check_decimal_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    ok "[$contour] Переменная $name содержит число"
  else
    fail "[$contour] Переменная $name должна быть числом: $value"
  fi
}

check_mem_limit_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?[bkmgBKMG]$ ]]; then
    ok "[$contour] Переменная $name выглядит как лимит памяти"
  else
    fail "[$contour] Переменная $name должна быть в формате вроде 512m или 1g: $value"
  fi
}

check_log_size_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?[bkmgBKMG]$ ]]; then
    ok "[$contour] Переменная $name выглядит как размер лога"
  else
    fail "[$contour] Переменная $name должна быть в формате вроде 10m: $value"
  fi
}

version_ge() {
  local current="$1"
  local minimum="$2"
  [[ "$(printf '%s\n%s\n' "$minimum" "$current" | sort -V | head -n 1)" == "$minimum" ]]
}

check_recommended_version() {
  local label="$1"
  local current="$2"
  local minimum="$3"

  if [[ -z "$current" ]]; then
    warn "Не удалось определить версию для $label"
    return
  fi

  if version_ge "$current" "$minimum"; then
    ok "$label версии $current соответствует рекомендуемому минимуму $minimum"
  else
    warn "$label версии $current ниже рекомендуемого минимума $minimum"
  fi
}

filesystem_free_mb() {
  local path="$1"
  local existing_path
  existing_path="$(nearest_existing_parent "$path")"

  df -Pm "$existing_path" | awk 'NR == 2 { print $4 }'
}

check_free_space() {
  local contour="$1"
  local name="$2"
  local path_value="$3"
  local min_free_mb="$4"
  local absolute_path existing_path free_mb

  absolute_path="$(root_path "$path_value")"
  existing_path="$(nearest_existing_parent "$absolute_path")"

  if ! command_exists df; then
    warn "[$contour] Проверка свободного места для $name пропущена: команда df недоступна"
    return
  fi

  free_mb="$(filesystem_free_mb "$absolute_path" 2>/dev/null || true)"
  if [[ ! "$free_mb" =~ ^[0-9]+$ ]]; then
    warn "[$contour] Не удалось определить свободное место для $name"
    return
  fi

  if (( free_mb >= min_free_mb )); then
    ok "[$contour] Для $name доступно $free_mb MB, это не меньше порога $min_free_mb MB"
  else
    warn "[$contour] Для $name доступно только $free_mb MB, это ниже рекомендуемого порога $min_free_mb MB"
  fi
}

check_port_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    fail "[$contour] Порт $name должен быть числом: $value"
    return
  fi

  if (( value < 1 || value > 65535 )); then
    fail "[$contour] Порт $name вне допустимого диапазона: $value"
    return
  fi

  local port_state
  if is_tcp_port_busy "$value"; then
    fail "[$contour] Порт $name=$value уже занят"
    return
  else
    port_state=$?
  fi

  case "$port_state" in
    1)
      ok "[$contour] Порт $name=$value свободен"
      ;;
    2)
      warn "[$contour] Не удалось проверить занятость порта $name=$value: нет ss/lsof/netstat"
      ;;
    *)
      fail "[$contour] Неожиданный результат проверки порта $name=$value"
      ;;
  esac
}

check_path_setting() {
  local contour="$1"
  local name="$2"
  local path_value="$3"
  local absolute_path
  absolute_path="$(root_path "$path_value")"

  if path_is_writable_or_creatable "$absolute_path"; then
    ok "[$contour] Путь $name доступен для записи или создания: $absolute_path"
  else
    fail "[$contour] Путь $name недоступен для записи или создания: $absolute_path"
  fi
}

check_required_var() {
  local contour="$1"
  local var_name="$2"
  local value="${!var_name:-}"

  if [[ -z "$value" ]]; then
    fail "[$contour] Не задана обязательная переменная $var_name"
    return 1
  fi

  return 0
}

check_compose_config_for_contour() {
  local contour="$1"

  if compose config >/dev/null 2>&1; then
    ok "[$contour] docker compose config проходит успешно"
  else
    fail "[$contour] docker compose config завершился ошибкой"
  fi
}

check_cross_contour_conflicts() {
  if [[ -z "${LOADED_PROJECTS[prod]:-}" || -z "${LOADED_PROJECTS[dev]:-}" ]]; then
    return
  fi

  section "Перекрестная проверка dev/prod"

  if [[ "${LOADED_PROJECTS[prod]}" == "${LOADED_PROJECTS[dev]}" ]]; then
    fail "[cross] COMPOSE_PROJECT_NAME совпадает у prod и dev: ${LOADED_PROJECTS[prod]}"
  else
    ok "[cross] COMPOSE_PROJECT_NAME различается у prod и dev"
  fi

  if [[ "${LOADED_APP_PORTS[prod]}" == "${LOADED_APP_PORTS[dev]}" ]]; then
    fail "[cross] APP_PORT совпадает у prod и dev: ${LOADED_APP_PORTS[prod]}"
  else
    ok "[cross] APP_PORT различается у prod и dev"
  fi

  if [[ "${LOADED_WS_PORTS[prod]}" == "${LOADED_WS_PORTS[dev]}" ]]; then
    fail "[cross] WS_PORT совпадает у prod и dev: ${LOADED_WS_PORTS[prod]}"
  else
    ok "[cross] WS_PORT различается у prod и dev"
  fi

  if [[ "${LOADED_DB_STORAGE_DIRS[prod]}" == "${LOADED_DB_STORAGE_DIRS[dev]}" ]]; then
    fail "[cross] DB_STORAGE_DIR совпадает у prod и dev: ${LOADED_DB_STORAGE_DIRS[prod]}"
  else
    ok "[cross] DB_STORAGE_DIR различается у prod и dev"
  fi

  if [[ "${LOADED_ESPO_STORAGE_DIRS[prod]}" == "${LOADED_ESPO_STORAGE_DIRS[dev]}" ]]; then
    fail "[cross] ESPO_STORAGE_DIR совпадает у prod и dev: ${LOADED_ESPO_STORAGE_DIRS[prod]}"
  else
    ok "[cross] ESPO_STORAGE_DIR различается у prod и dev"
  fi

  if [[ "${LOADED_BACKUP_ROOTS[prod]}" == "${LOADED_BACKUP_ROOTS[dev]}" ]]; then
    fail "[cross] BACKUP_ROOT совпадает у prod и dev: ${LOADED_BACKUP_ROOTS[prod]}"
  else
    ok "[cross] BACKUP_ROOT различается у prod и dev"
  fi

  if [[ "${LOADED_IMAGES[prod]}" != "${LOADED_IMAGES[dev]}" ]]; then
    warn "[cross] ESPOCRM_IMAGE различается между prod и dev: '${LOADED_IMAGES[prod]}' vs '${LOADED_IMAGES[dev]}'"
  else
    ok "[cross] ESPOCRM_IMAGE совпадает у prod и dev"
  fi
}

check_contour() {
  local contour="$1"
  local required_vars=(
    COMPOSE_PROJECT_NAME
    ESPOCRM_IMAGE
    MARIADB_TAG
    DB_STORAGE_DIR
    ESPO_STORAGE_DIR
    BACKUP_ROOT
    BACKUP_NAME_PREFIX
    BACKUP_RETENTION_DAYS
    BACKUP_MAX_DB_AGE_HOURS
    BACKUP_MAX_FILES_AGE_HOURS
    REPORT_RETENTION_DAYS
    SUPPORT_RETENTION_DAYS
    MIN_FREE_DISK_MB
    DOCKER_LOG_MAX_SIZE
    DOCKER_LOG_MAX_FILE
    DB_MEM_LIMIT
    DB_CPUS
    DB_PIDS_LIMIT
    ESPO_MEM_LIMIT
    ESPO_CPUS
    ESPO_PIDS_LIMIT
    DAEMON_MEM_LIMIT
    DAEMON_CPUS
    DAEMON_PIDS_LIMIT
    WS_MEM_LIMIT
    WS_CPUS
    WS_PIDS_LIMIT
    APP_PORT
    WS_PORT
    SITE_URL
    WS_PUBLIC_URL
    DB_ROOT_PASSWORD
    DB_NAME
    DB_USER
    DB_PASSWORD
    ADMIN_USERNAME
    ADMIN_PASSWORD
    ESPO_DEFAULT_LANGUAGE
    ESPO_TIME_ZONE
    ESPO_LOGGER_LEVEL
  )

  section "Проверка контура: $contour"

  # shellcheck disable=SC2034
  ESPO_ENV="$contour"
  if ! resolve_env_file 2>/dev/null; then
    fail "[$contour] Env-файл для контура не найден"
    return
  fi
  ok "[$contour] Найден env-файл: $ENV_FILE"

  if ! load_env 2>/dev/null; then
    fail "[$contour] Не удалось загрузить env-файл: $ENV_FILE"
    return
  fi
  ok "[$contour] Env-файл успешно загружен"

  local missing_required=0
  local var_name
  for var_name in "${required_vars[@]}"; do
    if ! check_required_var "$contour" "$var_name"; then
      missing_required=1
    fi
  done

  if [[ $missing_required -ne 0 ]]; then
    return
  fi

  LOADED_PROJECTS["$contour"]="$COMPOSE_PROJECT_NAME"
  LOADED_APP_PORTS["$contour"]="$APP_PORT"
  LOADED_WS_PORTS["$contour"]="$WS_PORT"
  LOADED_DB_STORAGE_DIRS["$contour"]="$DB_STORAGE_DIR"
  LOADED_ESPO_STORAGE_DIRS["$contour"]="$ESPO_STORAGE_DIR"
  LOADED_BACKUP_ROOTS["$contour"]="$BACKUP_ROOT"
  LOADED_IMAGES["$contour"]="$ESPOCRM_IMAGE"

  check_secret_value "$contour" "DB_ROOT_PASSWORD" "$DB_ROOT_PASSWORD"
  check_secret_value "$contour" "DB_PASSWORD" "$DB_PASSWORD"
  check_secret_value "$contour" "ADMIN_PASSWORD" "$ADMIN_PASSWORD"
  check_url "$contour" "SITE_URL" "$SITE_URL" '^https?://'
  check_url "$contour" "WS_PUBLIC_URL" "$WS_PUBLIC_URL" '^wss?://'
  check_integer_setting "$contour" "BACKUP_RETENTION_DAYS" "$BACKUP_RETENTION_DAYS"
  check_integer_setting "$contour" "BACKUP_MAX_DB_AGE_HOURS" "$BACKUP_MAX_DB_AGE_HOURS"
  check_integer_setting "$contour" "BACKUP_MAX_FILES_AGE_HOURS" "$BACKUP_MAX_FILES_AGE_HOURS"
  check_integer_setting "$contour" "REPORT_RETENTION_DAYS" "$REPORT_RETENTION_DAYS"
  check_integer_setting "$contour" "SUPPORT_RETENTION_DAYS" "$SUPPORT_RETENTION_DAYS"
  check_integer_setting "$contour" "MIN_FREE_DISK_MB" "$MIN_FREE_DISK_MB"
  check_integer_setting "$contour" "DOCKER_LOG_MAX_FILE" "$DOCKER_LOG_MAX_FILE"
  check_log_size_setting "$contour" "DOCKER_LOG_MAX_SIZE" "$DOCKER_LOG_MAX_SIZE"
  check_mem_limit_setting "$contour" "DB_MEM_LIMIT" "$DB_MEM_LIMIT"
  check_decimal_setting "$contour" "DB_CPUS" "$DB_CPUS"
  check_integer_setting "$contour" "DB_PIDS_LIMIT" "$DB_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "ESPO_MEM_LIMIT" "$ESPO_MEM_LIMIT"
  check_decimal_setting "$contour" "ESPO_CPUS" "$ESPO_CPUS"
  check_integer_setting "$contour" "ESPO_PIDS_LIMIT" "$ESPO_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "DAEMON_MEM_LIMIT" "$DAEMON_MEM_LIMIT"
  check_decimal_setting "$contour" "DAEMON_CPUS" "$DAEMON_CPUS"
  check_integer_setting "$contour" "DAEMON_PIDS_LIMIT" "$DAEMON_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "WS_MEM_LIMIT" "$WS_MEM_LIMIT"
  check_decimal_setting "$contour" "WS_CPUS" "$WS_CPUS"
  check_integer_setting "$contour" "WS_PIDS_LIMIT" "$WS_PIDS_LIMIT"
  check_port_setting "$contour" "APP_PORT" "$APP_PORT"
  check_port_setting "$contour" "WS_PORT" "$WS_PORT"

  if [[ "$APP_PORT" == "$WS_PORT" ]]; then
    fail "[$contour] APP_PORT и WS_PORT не должны совпадать"
  else
    ok "[$contour] APP_PORT и WS_PORT не конфликтуют между собой"
  fi

  check_path_setting "$contour" "DB_STORAGE_DIR" "$DB_STORAGE_DIR"
  check_path_setting "$contour" "ESPO_STORAGE_DIR" "$ESPO_STORAGE_DIR"
  check_path_setting "$contour" "BACKUP_ROOT" "$BACKUP_ROOT"
  check_free_space "$contour" "DB_STORAGE_DIR" "$DB_STORAGE_DIR" "$MIN_FREE_DISK_MB"
  check_free_space "$contour" "ESPO_STORAGE_DIR" "$ESPO_STORAGE_DIR" "$MIN_FREE_DISK_MB"
  check_free_space "$contour" "BACKUP_ROOT" "$BACKUP_ROOT" "$MIN_FREE_DISK_MB"

  if command_exists docker && docker compose version >/dev/null 2>&1; then
    check_compose_config_for_contour "$contour"
  else
    warn "[$contour] Проверка docker compose config пропущена: docker compose недоступен"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    all|prod|dev)
      TARGET_SCOPE="$1"
      shift
      ;;
    --json)
      JSON_MODE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Неизвестный режим проверки: $1"
      ;;
  esac
done

case "$TARGET_SCOPE" in
  all)
    CONTOURS=(prod dev)
    ;;
  prod|dev)
    CONTOURS=("$TARGET_SCOPE")
    ;;
  *)
    usage >&2
    die "Неизвестный режим проверки: $TARGET_SCOPE"
    ;;
esac

section "Базовые проверки окружения"
if command_exists bash; then
  ok "Доступен bash"
else
  fail "Команда bash не найдена"
fi

if command_exists docker; then
  ok "Доступен docker"
else
  fail "Команда docker не найдена"
fi

if command_exists docker && docker info >/dev/null 2>&1; then
  ok "Docker daemon доступен"
else
  fail "Docker daemon недоступен или не запущен"
fi

if command_exists docker && docker compose version >/dev/null 2>&1; then
  ok "Доступен docker compose"
else
  fail "docker compose недоступен"
fi

if command_exists docker && docker info >/dev/null 2>&1; then
  check_recommended_version "Docker Engine" "$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)" "$MIN_DOCKER_VERSION"
fi

if command_exists docker && docker compose version >/dev/null 2>&1; then
  check_recommended_version "Docker Compose" "$(docker compose version --short 2>/dev/null || true)" "$MIN_DOCKER_COMPOSE_VERSION"
fi

check_sha256_tool

for contour in "${CONTOURS[@]}"; do
  check_contour "$contour"
done

if [[ "$TARGET_SCOPE" == "all" ]]; then
  check_cross_contour_conflicts
fi

if [[ $JSON_MODE -eq 0 ]]; then
  echo
  echo "Итог проверки: ok=$OK_COUNT warn=$WARN_COUNT fail=$FAIL_COUNT"
else
  render_json_report
fi

if [[ $FAIL_COUNT -ne 0 ]]; then
  exit 1
fi
