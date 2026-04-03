#!/usr/bin/env bash
set -Eeuo pipefail

# Абсолютный путь до корня проекта.
# Все остальные относительные пути мы приводим именно к нему.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Запоминаем каталог, из которого пользователь вызвал скрипт.
# Это нужно, чтобы корректно интерпретировать относительные пути к бэкапам.
CALLER_DIR="$(pwd)"

# Общий массив позиционных аргументов.
# Он заполняется функцией `parse_contour_arg` и затем используется
# в вызывающих скриптах после отделения имени контура от остальных аргументов.
# shellcheck disable=SC2034
POSITIONAL_ARGS=()

# Единообразный аварийный выход с сообщением об ошибке.
die() {
  echo "Ошибка: $*" >&2
  exit 1
}

# Единообразный информационный вывод.
note() {
  echo "[info] $*"
}

# Единообразный вывод предупреждений.
warn() {
  echo "[warn] $*" >&2
}

# Аккуратно добавляем команду в уже существующий trap, не затирая его целиком.
# Это полезно для сценариев, где одному скрипту нужны и свои cleanup-операции,
# и общий release lock.
append_trap() {
  local new_command="$1"
  shift

  local signal existing_command
  for signal in "$@"; do
    existing_command="$(trap -p "$signal" | awk -F"'" 'NR == 1 { print $2 }')"
    if [[ -n "$existing_command" ]]; then
      # shellcheck disable=SC2064
      trap "$existing_command"$'\n'"$new_command" "$signal"
    else
      # shellcheck disable=SC2064
      trap "$new_command" "$signal"
    fi
  done
}

# Проверяем, доступна ли команда в PATH.
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Запускаем дочерний shell-скрипт явно через bash.
# Это избавляет от зависимости на execute-bit в git checkout,
# что особенно важно для Linux CI после коммита с Windows-машины.
run_repo_script() {
  local script_path="$1"
  shift

  [[ -f "$script_path" ]] || die "Не найден дочерний скрипт: $script_path"
  bash "$script_path" "$@"
}

# Проверяем, жив ли процесс-владелец lock-файла.
lock_owner_is_alive() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null
}

# Читаем PID владельца lock-файла.
lock_file_owner_pid() {
  local lock_file="$1"
  [[ -f "$lock_file" ]] || return 1
  head -n 1 "$lock_file" 2>/dev/null || true
}

# Возвращаем состояние lock-файла: active или stale.
lock_file_state() {
  local lock_file="$1"
  local pid

  pid="$(lock_file_owner_pid "$lock_file" || true)"
  if lock_owner_is_alive "$pid"; then
    printf 'active\n'
  else
    printf 'stale\n'
  fi
}

# Перечисляем lock-файлы контура в стабильном порядке.
list_lock_files() {
  local directory="$1"
  [[ -d "$directory" ]] || return 0
  find "$directory" -maxdepth 1 -type f -name '*.lock' | sort
}

# Получаем SHA-256 для файла через первый доступный инструмент.
# Это позволяет не завязываться только на одну утилиту конкретного дистрибутива.
sha256_file() {
  local file="$1"
  [[ -f "$file" ]] || die "Файл для расчета SHA-256 не найден: $file"

  if command_exists sha256sum; then
    sha256sum "$file" | awk '{print $1}'
  elif command_exists shasum; then
    shasum -a 256 "$file" | awk '{print $1}'
  elif command_exists openssl; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  else
    die "Не найден инструмент для расчета SHA-256 (sha256sum, shasum или openssl)"
  fi
}

# Сохраняем checksum-файл рядом с целевым артефактом.
write_sha256_sidecar() {
  local file="$1"
  local checksum="$2"
  local sidecar="${3:-$file.sha256}"

  printf '%s  %s\n' "$checksum" "$(basename "$file")" > "$sidecar"
}

# Читаем checksum из sidecar-файла.
read_sha256_sidecar() {
  local sidecar="$1"
  [[ -f "$sidecar" ]] || die "Checksum-файл не найден: $sidecar"

  awk 'NR == 1 { print $1 }' "$sidecar"
}

# Проверяем соответствие checksum-файла и реального содержимого.
# Возвращает:
# - 0, если checksum совпадает;
# - 1, если checksum не совпадает;
# - 2, если sidecar-файл отсутствует.
verify_sha256_sidecar() {
  local file="$1"
  local sidecar="${2:-$file.sha256}"

  if [[ ! -f "$sidecar" ]]; then
    return 2
  fi

  local expected actual
  expected="$(read_sha256_sidecar "$sidecar")"
  actual="$(sha256_file "$file")"

  [[ "$actual" == "$expected" ]]
}

# Проверяем целостность бэкапа, если рядом есть checksum-файл.
# Это сохраняет обратную совместимость со старыми архивами без sidecar.
verify_sha256_or_warn() {
  local file="$1"
  local sidecar="${2:-$file.sha256}"

  if [[ ! -f "$sidecar" ]]; then
    note "Checksum-файл не найден, проверка целостности пропущена: $sidecar"
    return 0
  fi

  if verify_sha256_sidecar "$file" "$sidecar"; then
    note "Контрольная сумма подтверждена: $sidecar"
  else
    die "Контрольная сумма не совпадает для файла: $file"
  fi
}

# Возвращаем размер файла в байтах.
file_size_bytes() {
  local file="$1"
  [[ -f "$file" ]] || die "Файл не найден: $file"

  wc -c < "$file" | tr -d '[:space:]'
}

# Возвращаем время последней модификации файла в Unix epoch.
file_mtime_epoch() {
  local file="$1"
  [[ -f "$file" ]] || die "Файл не найден: $file"

  if stat -c %Y "$file" >/dev/null 2>&1; then
    stat -c %Y "$file"
  elif stat -f %m "$file" >/dev/null 2>&1; then
    stat -f %m "$file"
  else
    die "Не удалось определить время модификации файла: $file"
  fi
}

# Экранируем строку для безопасной вставки в JSON.
json_escape() {
  local value="$1"

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

# Разбор первого аргумента как имени контура.
# Поддерживаем только `dev` и `prod`.
# Если пользователь не передал контур явно, по умолчанию считаем, что это `prod`.
parse_contour_arg() {
  ESPO_ENV="${ESPO_ENV:-prod}"

  if [[ $# -gt 0 ]]; then
    case "$1" in
      dev|prod)
        ESPO_ENV="$1"
        shift
        ;;
    esac
  fi

  export ESPO_ENV
  # Сохраняем остаток аргументов для вызывающего скрипта.
  # shellcheck disable=SC2034
  POSITIONAL_ARGS=("$@")
}

# Определяем, какой env-файл должен использоваться для текущего контура.
# Поддерживаем только явные env-файлы `.env.prod` и `.env.dev`,
# чтобы в проекте не оставалось двусмысленной legacy-схемы.
resolve_env_file() {
  if [[ -n "${ENV_FILE:-}" ]]; then
    [[ -f "$ENV_FILE" ]] || die "Не найден переопределенный env-файл: $ENV_FILE"
    export ENV_FILE
    return
  fi

  case "$ESPO_ENV" in
    prod)
      ENV_FILE="$ROOT_DIR/.env.prod"
      [[ -f "$ENV_FILE" ]] || die "Не найден $ENV_FILE"
      ;;
    dev)
      ENV_FILE="$ROOT_DIR/.env.dev"
      [[ -f "$ENV_FILE" ]] || die "Не найден $ENV_FILE"
      ;;
    *)
      die "Неподдерживаемый контур '$ESPO_ENV'. Используйте dev или prod."
      ;;
  esac

  export ENV_FILE
}

# Загружаем переменные окружения из выбранного env-файла.
# Используем `set -a`, чтобы значения автоматически экспортировались
# и были доступны `docker compose` и дочерним процессам.
load_env() {
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a

  # Минимальная проверка обязательных переменных.
  : "${COMPOSE_PROJECT_NAME:?Не задан COMPOSE_PROJECT_NAME в $ENV_FILE}"
  : "${DB_STORAGE_DIR:?Не задан DB_STORAGE_DIR в $ENV_FILE}"
  : "${ESPO_STORAGE_DIR:?Не задан ESPO_STORAGE_DIR в $ENV_FILE}"
  : "${BACKUP_ROOT:?Не задан BACKUP_ROOT в $ENV_FILE}"
}

# Проверяем наличие Docker CLI и Compose plugin.
require_compose() {
  command -v docker >/dev/null 2>&1 || die "docker не установлен или не найден в PATH"
  docker compose version >/dev/null 2>&1 || die "плагин docker compose недоступен"
}

# Унифицированный вызов `docker compose` для текущего проекта.
# Все скрипты используют именно эту обертку, чтобы:
# - работать из любого каталога;
# - гарантированно подхватывать нужный env-файл;
# - всегда ссылаться на нужный compose.yaml.
compose() {
  docker compose \
    --project-directory "$ROOT_DIR" \
    -f "$ROOT_DIR/compose.yaml" \
    --env-file "$ENV_FILE" \
    "$@"
}

# Преобразуем путь к виду "относительно корня проекта".
# Если путь уже абсолютный, возвращаем его как есть.
root_path() {
  local path="$1"

  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$ROOT_DIR/${path#./}"
  fi
}

# Преобразуем путь относительно каталога, из которого пользователь вызвал скрипт.
# Это удобно для аргументов вида `./dump.sql.gz`.
caller_path() {
  local path="$1"

  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$CALLER_DIR/${path#./}"
  fi
}

# Обновляем или добавляем ключ в env-файле без ручного редактирования.
# Это удобно для временных тестовых окружений и restore-drill сценариев.
set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"

  if grep -q "^${key}=" "$file"; then
    sed -i "s|^${key}=.*|${key}=${value}|" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$file"
  fi
}

# Пытаемся удалить дерево каталога через временный Docker-контейнер.
# Это нужно для bind-mount каталогов, внутри которых контейнер оставил root-owned файлы,
# а обычный пользователь хоста не может их убрать через rm -rf.
docker_remove_tree() {
  local target="$1"
  local parent base
  local cleanup_image="${ESPOCRM_IMAGE:-alpine:3.20}"

  parent="$(dirname "$target")"
  base="$(basename "$target")"

  command_exists docker || return 1
  [[ -d "$parent" ]] || return 1

  docker run --rm \
    --entrypoint sh \
    -v "$parent:/cleanup-parent" \
    -e CLEANUP_BASENAME="$base" \
    "$cleanup_image" \
    sh -euc 'rm -rf -- "/cleanup-parent/$CLEANUP_BASENAME"' \
    >/dev/null
}

# Пытаемся очистить содержимое каталога через временный Docker-контейнер.
# Сам каталог сохраняется, удаляется только его содержимое.
docker_empty_dir() {
  local target="$1"
  local cleanup_image="${ESPOCRM_IMAGE:-alpine:3.20}"

  command_exists docker || return 1
  [[ -d "$target" ]] || return 1

  docker run --rm \
    --entrypoint sh \
    -v "$target:/cleanup-target" \
    "$cleanup_image" \
    sh -euc 'find /cleanup-target -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +' \
    >/dev/null
}

# Полностью удаляем каталог внутри проекта.
# Используем это только для временных тестовых контуров и drill-артефактов.
safe_remove_tree() {
  local target="$1"
  [[ -n "$target" ]] || die "Не передан путь для удаления"
  [[ -d "$target" ]] || return 0

  local resolved
  resolved="$(cd "$target" && pwd)"
  [[ "$resolved" == "$ROOT_DIR"/* ]] || die "Отказ в удалении пути вне проекта: $resolved"

  if rm -rf -- "$resolved" 2>/dev/null; then
    return 0
  fi

  warn "Обычное удаление не удалось, пробую Docker fallback: $resolved"
  docker_remove_tree "$resolved" || die "Не удалось удалить каталог даже через Docker fallback: $resolved"
}

# Создаем все рабочие каталоги для текущего контура.
# Вызывается как на первом запуске, так и перед бэкапами/restore.
ensure_runtime_dirs() {
  mkdir -p \
    "$(root_path "$DB_STORAGE_DIR")" \
    "$(root_path "$ESPO_STORAGE_DIR")" \
    "$(root_path "$BACKUP_ROOT")/db" \
    "$(root_path "$BACKUP_ROOT")/files" \
    "$(root_path "$BACKUP_ROOT")/locks" \
    "$(root_path "$BACKUP_ROOT")/manifests" \
    "$(root_path "$BACKUP_ROOT")/reports" \
    "$(root_path "$BACKUP_ROOT")/support"
}

# Освобождаем maintenance-lock, если он был захвачен текущим процессом.
release_maintenance_lock() {
  if [[ ${MAINTENANCE_LOCK_HELD:-0} -eq 1 && -n "${MAINTENANCE_LOCK_FILE:-}" ]]; then
    rm -f -- "$MAINTENANCE_LOCK_FILE"
    MAINTENANCE_LOCK_HELD=0
  fi
}

# Захватываем блокировку на операции обслуживания контура.
# Это защищает от одновременного запуска backup/restore/update/migrate,
# которые могут конфликтовать между собой по данным и сервисам.
#
# Вложенные дочерние вызовы не пытаются захватывать lock повторно:
# верхнеуровневый скрипт экспортирует `ESPO_MAINTENANCE_LOCK=1`,
# а дочерние скрипты просто используют уже существующую блокировку.
acquire_maintenance_lock() {
  local scope="${1:-maintenance}"
  local existing_pid=""

  if [[ "${ESPO_MAINTENANCE_LOCK:-0}" == "1" ]]; then
    note "Используется унаследованная блокировка обслуживания для контура '$ESPO_ENV'"
    return 0
  fi

  MAINTENANCE_LOCK_FILE="$(root_path "$BACKUP_ROOT")/locks/${scope}.lock"

  if [[ -f "$MAINTENANCE_LOCK_FILE" ]]; then
    existing_pid="$(head -n 1 "$MAINTENANCE_LOCK_FILE" 2>/dev/null || true)"
    if lock_owner_is_alive "$existing_pid"; then
      die "Для контура '$ESPO_ENV' уже выполняется другая операция обслуживания (PID $existing_pid): $MAINTENANCE_LOCK_FILE"
    fi

    warn "Найден устаревший lock-файл, удаляю: $MAINTENANCE_LOCK_FILE"
    rm -f -- "$MAINTENANCE_LOCK_FILE"
  fi

  if ! ( set -o noclobber; printf '%s\n' "$$" > "$MAINTENANCE_LOCK_FILE" ) 2>/dev/null; then
    die "Не удалось создать lock-файл обслуживания: $MAINTENANCE_LOCK_FILE"
  fi

  export ESPO_MAINTENANCE_LOCK=1
  MAINTENANCE_LOCK_HELD=1
  append_trap 'release_maintenance_lock' EXIT
  note "Захвачена блокировка обслуживания: $MAINTENANCE_LOCK_FILE"
}

# Безопасно очищаем каталог внутри проекта.
# Функция специально защищает от случайного удаления чего-то вне репозитория.
safe_empty_dir() {
  local target_dir="$1"
  mkdir -p "$target_dir"

  local resolved
  resolved="$(cd "$target_dir" && pwd)"

  [[ -n "$resolved" ]] || die "Не удалось вычислить путь каталога: $target_dir"
  [[ "$resolved" == "$ROOT_DIR"/* ]] || die "Отказ в очистке пути вне проекта: $resolved"
  [[ "$resolved" != "$ROOT_DIR" ]] || die "Отказ в очистке корня проекта"

  if find "$resolved" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null; then
    return 0
  fi

  warn "Обычная очистка не удалась, пробую Docker fallback: $resolved"
  docker_empty_dir "$resolved" || die "Не удалось очистить каталог даже через Docker fallback: $resolved"
}

# Проверяем, что конкретный сервис действительно запущен.
# Это позволяет завершаться понятной ошибкой до начала бэкапа/restore.
require_service_running() {
  local service="$1"

  if ! compose ps --status running --services | grep -qx "$service"; then
    die "Сервис '$service' не запущен для контура '$ESPO_ENV'"
  fi
}

# Возвращаем успешный код, если сервис уже работает.
# Это удобно для сценариев обновления, когда БД нужно при необходимости
# временно поднять перед созданием контрольного backup.
service_is_running() {
  local service="$1"
  compose ps --status running --services | grep -qx "$service"
}

# Проверяем, запущен ли хотя бы один прикладной сервис EspoCRM.
# Это нужно для безопасного restore: если сервисы были активны до начала операции,
# мы их остановим и затем поднимем обратно после успешного завершения.
app_services_running() {
  local running_services
  running_services="$(compose ps --status running --services || true)"

  grep -qx 'espocrm' <<<"$running_services" \
    || grep -qx 'espocrm-daemon' <<<"$running_services" \
    || grep -qx 'espocrm-websocket' <<<"$running_services"
}

# Останавливаем прикладные сервисы, не трогая контейнер БД.
# Если сервисы еще не созданы, это не считается ошибкой.
stop_app_services() {
  compose stop espocrm espocrm-daemon espocrm-websocket >/dev/null 2>&1 || true
}

# Запускаем обратно прикладные сервисы после обслуживания.
# Базу данных не трогаем: предполагается, что она уже доступна.
start_app_services() {
  compose up -d espocrm espocrm-daemon espocrm-websocket
}

# Возвращаем идентификатор контейнера для compose-сервиса.
compose_service_container_id() {
  local service="$1"
  compose ps -q "$service"
}

# Возвращаем текущее состояние сервиса.
# Если у контейнера есть healthcheck, используем его статус.
compose_service_status() {
  local service="$1"
  local container_id
  container_id="$(compose_service_container_id "$service")"

  [[ -n "$container_id" ]] || return 1

  docker inspect \
    --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
    "$container_id"
}

# Ждем, пока сервис не станет готовым.
# Для сервисов с healthcheck ожидаем статус `healthy`,
# для остальных допускаем `running`.
wait_for_service_ready() {
  local service="$1"
  local timeout_seconds="${2:-300}"
  local start_time now elapsed status

  start_time="$(date +%s)"

  while true; do
    status="$(compose_service_status "$service" 2>/dev/null || true)"
    case "$status" in
      healthy|running)
        return 0
        ;;
      exited|dead)
        die "Сервис '$service' завершился аварийно во время ожидания готовности"
        ;;
    esac

    now="$(date +%s)"
    elapsed=$((now - start_time))
    if (( elapsed >= timeout_seconds )); then
      die "Таймаут ожидания готовности сервиса '$service' (${timeout_seconds} сек.)"
    fi

    sleep 5
  done
}

# Проверяем доступность HTTP-URL через curl или wget.
http_probe() {
  local url="$1"

  if command_exists curl; then
    curl -fsSL --max-time 10 -o /dev/null "$url"
  elif command_exists wget; then
    wget -q -T 10 -O /dev/null "$url"
  else
    die "Не найден инструмент HTTP-проверки (curl или wget)"
  fi
}

# Ищем самый свежий backup-файл по шаблону в указанном каталоге.
latest_backup_file() {
  local directory="$1"
  local pattern="$2"

  [[ -d "$directory" ]] || return 1

  find "$directory" -maxdepth 1 -type f -name "$pattern" -printf '%T@ %p\n' \
    | sort -nr \
    | head -n 1 \
    | cut -d' ' -f2-
}

# Удаляем старые служебные артефакты по набору шаблонов и сроку хранения.
cleanup_old_files() {
  local directory="$1"
  local retention_days="$2"
  shift 2

  [[ -d "$directory" ]] || return 0
  [[ $# -gt 0 ]] || return 0

  local find_args=("$directory" -type f '(')
  local first=1
  local pattern

  for pattern in "$@"; do
    if [[ $first -eq 0 ]]; then
      find_args+=(-o)
    fi
    find_args+=(-name "$pattern")
    first=0
  done

  find_args+=(')' -mtime +"$retention_days" -delete)
  find "${find_args[@]}"
}

# Возвращаем ближайший существующий родительский путь.
nearest_existing_parent() {
  local path="$1"

  while [[ ! -e "$path" && "$path" != "/" ]]; do
    path="$(dirname "$path")"
  done

  printf '%s\n' "$path"
}

# Проверяем, что путь либо уже доступен на запись,
# либо может быть создан текущим пользователем.
path_is_writable_or_creatable() {
  local path="$1"

  if [[ -e "$path" ]]; then
    [[ -w "$path" ]]
    return
  fi

  local parent
  parent="$(nearest_existing_parent "$path")"
  [[ -d "$parent" && -w "$parent" ]]
}

# Возвращаем размер каталога в человекочитаемом виде.
# Если каталог отсутствует, считаем размер нулевым.
directory_size_human() {
  local path="$1"

  if [[ ! -e "$path" ]]; then
    printf '0\n'
    return
  fi

  if command_exists du; then
    du -sh "$path" 2>/dev/null | awk '{print $1}'
  else
    printf 'n/a\n'
  fi
}

# Проверяем, занят ли TCP-порт.
# Если в системе нет подходящей утилиты, возвращаем 2.
is_tcp_port_busy() {
  local port="$1"

  if command_exists ss; then
    ss -ltn "( sport = :$port )" | tail -n +2 | grep -q .
  elif command_exists lsof; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  elif command_exists netstat; then
    netstat -ltn 2>/dev/null | awk 'NR > 2 { print $4 }' | grep -Eq "(^|:)$port$"
  else
    return 2
  fi
}

# Печатаем краткий контекст выполнения.
print_context() {
  note "Контур: $ESPO_ENV"
  note "Env-файл: $ENV_FILE"
  note "Compose-проект: $COMPOSE_PROJECT_NAME"
}
