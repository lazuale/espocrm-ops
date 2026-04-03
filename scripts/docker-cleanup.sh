#!/usr/bin/env bash
set -Eeuo pipefail

# Безопасная очистка Docker-хоста от старых остановленных контейнеров,
# dangling-образов, неиспользуемых сетей и build cache.
# По умолчанию работает только в режиме плана и ничего не удаляет.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/docker-cleanup.sh [--apply] [--include-unused-images] [--skip-build-cache] [--container-age AGE] [--image-age AGE] [--unused-image-age AGE] [--network-age AGE] [--builder-age AGE]

Примеры:
  ./scripts/docker-cleanup.sh
  ./scripts/docker-cleanup.sh --apply
  ./scripts/docker-cleanup.sh --apply --include-unused-images
  ./scripts/docker-cleanup.sh --apply --container-age 168h --image-age 168h --network-age 168h --builder-age 168h

Параметры:
  --apply                  Реально удалить найденные кандидаты. Без этого флага выполняется только dry-run.
  --include-unused-images  Дополнительно удалять старые неиспользуемые tag-образы, а не только dangling.
  --skip-build-cache       Не очищать build cache.
  --container-age AGE      Возраст остановленных контейнеров, начиная с которого они считаются мусором. По умолчанию 168h.
  --image-age AGE          Возраст dangling-образов. По умолчанию 168h.
  --unused-image-age AGE   Возраст tag-образов без контейнеров. По умолчанию 336h.
  --network-age AGE        Возраст неиспользуемых пользовательских сетей. По умолчанию 168h.
  --builder-age AGE        Возраст build cache для docker builder prune. По умолчанию 168h.

Формат AGE:
  Поддерживаются значения вида 30m, 12h, 7d, 2w.

Важно:
  Скрипт намеренно не делает volume prune, чтобы не задеть чужие Docker-тома с данными.
EOF
}

APPLY=0
INCLUDE_UNUSED_IMAGES=0
SKIP_BUILD_CACHE=0
CONTAINER_AGE="168h"
IMAGE_AGE="168h"
UNUSED_IMAGE_AGE="336h"
NETWORK_AGE="168h"
BUILDER_AGE="168h"
STAMP="$(date +%F_%H-%M-%S)"
REPORT_DIR="$ROOT_DIR/backups/host/reports"
REPORT_RETENTION_DAYS=30
REPORT_FILE=""
declare -a LOCK_DIRS=()
declare -a BARRIER_LOCK_FILES=()
declare -a ACTIVE_LOCKS=()
declare -a STALE_LOCKS=()
declare -a CONTAINER_IDS=()
declare -a CONTAINER_LINES=()
declare -a IMAGE_IDS=()
declare -a IMAGE_LINES=()
declare -a NETWORK_IDS=()
declare -a NETWORK_LINES=()
declare -a PROTECTED_SCOPES=(
  backup
  restore-db
  restore-files
  migrate
  restore-drill
  rollback
  update
  docker-cleanup
)
declare -A REFERENCED_IMAGE_IDS=()

duration_to_seconds() {
  local value="$1"
  local number suffix multiplier

  [[ "$value" =~ ^([0-9]+)([smhdw])$ ]] || return 1
  number="${BASH_REMATCH[1]}"
  suffix="${BASH_REMATCH[2]}"

  case "$suffix" in
    s) multiplier=1 ;;
    m) multiplier=60 ;;
    h) multiplier=3600 ;;
    d) multiplier=86400 ;;
    w) multiplier=604800 ;;
    *) return 1 ;;
  esac

  printf '%s\n' "$((number * multiplier))"
}

validate_duration() {
  local name="$1"
  local value="$2"
  duration_to_seconds "$value" >/dev/null || die "Параметр $name должен быть в формате вида 30m, 12h, 7d или 2w: $value"
}

cutoff_epoch_for_duration() {
  local value="$1"
  local now seconds

  now="$(date +%s)"
  seconds="$(duration_to_seconds "$value")"
  printf '%s\n' "$((now - seconds))"
}

docker_time_to_epoch() {
  local value="$1"
  date -d "$value" +%s
}

short_id() {
  local value="$1"
  value="${value#sha256:}"
  printf '%s\n' "${value:0:12}"
}

extract_env_value() {
  local file="$1"
  local key="$2"

  [[ -f "$file" ]] || return 1
  awk -F= -v key="$key" '$1 == key { print substr($0, index($0, "=") + 1) }' "$file" | tail -n 1
}

setup_report_stream() {
  mkdir -p "$REPORT_DIR"
  cleanup_old_files "$REPORT_DIR" "$REPORT_RETENTION_DAYS" '*.txt'
  REPORT_FILE="$REPORT_DIR/docker-cleanup_$([[ $APPLY -eq 1 ]] && echo apply || echo plan)_${STAMP}.txt"

  if command_exists tee; then
    # Пишем один и тот же диагностический поток и в консоль, и в файл отчета.
    exec > >(tee "$REPORT_FILE") 2>&1
  fi
}

discover_lock_dirs() {
  local contour env_file backup_root backup_root_abs lock_dir
  declare -A seen=()

  for contour in prod dev; do
    env_file="$ROOT_DIR/.env.$contour"
    backup_root=""

    if [[ -f "$env_file" ]]; then
      backup_root="$(extract_env_value "$env_file" BACKUP_ROOT || true)"
    elif [[ -d "$ROOT_DIR/backups/$contour" ]]; then
      backup_root="./backups/$contour"
    fi

    [[ -n "$backup_root" ]] || continue
    backup_root_abs="$(root_path "$backup_root")"
    lock_dir="$backup_root_abs/locks"

    if [[ -z "${seen[$lock_dir]+x}" ]]; then
      LOCK_DIRS+=("$lock_dir")
      seen["$lock_dir"]=1
    fi
  done
}

scan_lock_dirs() {
  local lock_dir lock_file pid

  ACTIVE_LOCKS=()
  STALE_LOCKS=()

  for lock_dir in "${LOCK_DIRS[@]}"; do
    [[ -d "$lock_dir" ]] || continue

    while IFS= read -r lock_file; do
      [[ -n "$lock_file" ]] || continue
      pid="$(lock_file_owner_pid "$lock_file" || true)"

      if lock_owner_is_alive "$pid"; then
        ACTIVE_LOCKS+=("$lock_file (PID $pid)")
      else
        STALE_LOCKS+=("$lock_file")
      fi
    done < <(list_lock_files "$lock_dir")
  done
}

purge_stale_locks() {
  local lock_file

  for lock_file in "${STALE_LOCKS[@]}"; do
    warn "Удаляю устаревший lock-файл: $lock_file"
    rm -f -- "$lock_file"
  done
}

release_barrier_locks() {
  local lock_file

  for lock_file in "${BARRIER_LOCK_FILES[@]}"; do
    rm -f -- "$lock_file"
  done
}

acquire_barrier_locks() {
  local lock_dir scope lock_file pid

  for lock_dir in "${LOCK_DIRS[@]}"; do
    mkdir -p "$lock_dir"

    for scope in "${PROTECTED_SCOPES[@]}"; do
      lock_file="$lock_dir/${scope}.lock"

      if [[ -f "$lock_file" ]]; then
        pid="$(lock_file_owner_pid "$lock_file" || true)"
        if lock_owner_is_alive "$pid"; then
          die "Найден активный lock-файл, очистка небезопасна: $lock_file (PID $pid)"
        fi

        warn "Удаляю устаревший lock-файл перед захватом барьера: $lock_file"
        rm -f -- "$lock_file"
      fi

      if ! ( set -o noclobber; printf '%s\n' "$$" > "$lock_file" ) 2>/dev/null; then
        die "Не удалось создать lock-файл барьера: $lock_file"
      fi

      BARRIER_LOCK_FILES+=("$lock_file")
    done
  done

  append_trap 'release_barrier_locks' EXIT
}

require_docker() {
  command_exists docker || die "docker не найден в PATH"
  docker info >/dev/null 2>&1 || die "Docker daemon недоступен"
}

build_referenced_image_index() {
  local container_ids=()
  local image_id

  mapfile -t container_ids < <(docker container ls -aq)
  [[ ${#container_ids[@]} -gt 0 ]] || return 0

  while IFS= read -r image_id; do
    [[ -n "$image_id" ]] || continue
    REFERENCED_IMAGE_IDS["$image_id"]=1
  done < <(docker inspect --format '{{.Image}}' "${container_ids[@]}" | sort -u)
}

gather_stopped_container_candidates() {
  local cutoff_epoch container_ids=()
  local id name created status image created_epoch

  cutoff_epoch="$(cutoff_epoch_for_duration "$CONTAINER_AGE")"
  mapfile -t container_ids < <(docker container ls -aq)
  [[ ${#container_ids[@]} -gt 0 ]] || return 0

  while IFS='|' read -r id name created status image; do
    [[ -n "$id" ]] || continue
    [[ "$status" == "running" ]] && continue
    created_epoch="$(docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    (( created_epoch <= cutoff_epoch )) || continue

    CONTAINER_IDS+=("$id")
    CONTAINER_LINES+=("$(short_id "$id")  ${name#/}  status=$status  image=$image  created=$created")
  done < <(docker inspect --format '{{.Id}}|{{.Name}}|{{.Created}}|{{.State.Status}}|{{.Config.Image}}' "${container_ids[@]}")
}

gather_image_candidates() {
  local image_ids=()
  local dangling_ids=()
  local cutoff_dangling cutoff_unused
  local id tag created created_epoch reason
  declare -A dangling_index=()
  declare -A seen=()

  cutoff_dangling="$(cutoff_epoch_for_duration "$IMAGE_AGE")"
  cutoff_unused="$(cutoff_epoch_for_duration "$UNUSED_IMAGE_AGE")"

  mapfile -t image_ids < <(docker image ls -qa --no-trunc | sort -u)
  [[ ${#image_ids[@]} -gt 0 ]] || return 0

  mapfile -t dangling_ids < <(docker image ls -q --no-trunc --filter dangling=true | sort -u)
  for id in "${dangling_ids[@]}"; do
    [[ -n "$id" ]] || continue
    dangling_index["$id"]=1
  done

  while IFS='|' read -r id tag created; do
    [[ -n "$id" ]] || continue
    created_epoch="$(docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    reason=""

    if [[ -n "${dangling_index[$id]+x}" ]] && (( created_epoch <= cutoff_dangling )); then
      reason="dangling"
    elif [[ $INCLUDE_UNUSED_IMAGES -eq 1 && -z "${REFERENCED_IMAGE_IDS[$id]+x}" ]] && (( created_epoch <= cutoff_unused )); then
      reason="unused"
    fi

    [[ -n "$reason" ]] || continue
    [[ -z "${seen[$id]+x}" ]] || continue

    IMAGE_IDS+=("$id")
    IMAGE_LINES+=("$(short_id "$id")  $tag  reason=$reason  created=$created")
    seen["$id"]=1
  done < <(docker image inspect --format '{{.Id}}|{{if .RepoTags}}{{index .RepoTags 0}}{{else}}<none>:<none>{{end}}|{{.Created}}' "${image_ids[@]}")
}

gather_network_candidates() {
  local cutoff_epoch network_ids=()
  local id name created container_count created_epoch

  cutoff_epoch="$(cutoff_epoch_for_duration "$NETWORK_AGE")"
  mapfile -t network_ids < <(docker network ls -q)
  [[ ${#network_ids[@]} -gt 0 ]] || return 0

  while IFS='|' read -r id name created container_count; do
    [[ -n "$id" ]] || continue

    case "$name" in
      bridge|host|none)
        continue
        ;;
    esac

    [[ "$container_count" == "0" ]] || continue
    created_epoch="$(docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    (( created_epoch <= cutoff_epoch )) || continue

    NETWORK_IDS+=("$id")
    NETWORK_LINES+=("$(short_id "$id")  $name  created=$created")
  done < <(docker network inspect --format '{{.Id}}|{{.Name}}|{{.Created}}|{{len .Containers}}' "${network_ids[@]}")
}

print_candidate_block() {
  local title="$1"
  shift
  local lines=("$@")
  local line

  echo
  echo "== $title =="
  if [[ ${#lines[@]} -eq 0 ]]; then
    echo "Ничего не найдено."
    return
  fi

  for line in "${lines[@]}"; do
    echo "$line"
  done
}

remove_stopped_containers() {
  local id removed=0 failed=0

  [[ ${#CONTAINER_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Удаление остановленных контейнеров..."

  for id in "${CONTAINER_IDS[@]}"; do
    if docker rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Не удалось удалить контейнер: $id"
      failed=$((failed + 1))
    fi
  done

  note "Остановленные контейнеры: удалено=$removed, ошибок=$failed"
  [[ $failed -eq 0 ]]
}

remove_image_candidates() {
  local id removed=0 failed=0

  [[ ${#IMAGE_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Удаление образов..."

  for id in "${IMAGE_IDS[@]}"; do
    if docker image rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Не удалось удалить образ: $id"
      failed=$((failed + 1))
    fi
  done

  note "Образы: удалено=$removed, ошибок=$failed"
  [[ $failed -eq 0 ]]
}

remove_network_candidates() {
  local id removed=0 failed=0

  [[ ${#NETWORK_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Удаление неиспользуемых сетей..."

  for id in "${NETWORK_IDS[@]}"; do
    if docker network rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Не удалось удалить сеть: $id"
      failed=$((failed + 1))
    fi
  done

  note "Сети: удалено=$removed, ошибок=$failed"
  [[ $failed -eq 0 ]]
}

prune_builder_cache() {
  [[ $SKIP_BUILD_CACHE -eq 1 ]] && return 0

  echo
  echo "Очистка build cache старше $BUILDER_AGE..."
  docker builder prune -f --filter "until=$BUILDER_AGE"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      APPLY=1
      shift
      ;;
    --include-unused-images)
      INCLUDE_UNUSED_IMAGES=1
      shift
      ;;
    --skip-build-cache)
      SKIP_BUILD_CACHE=1
      shift
      ;;
    --container-age)
      [[ $# -ge 2 ]] || die "После --container-age нужно указать значение"
      CONTAINER_AGE="$2"
      shift 2
      ;;
    --image-age)
      [[ $# -ge 2 ]] || die "После --image-age нужно указать значение"
      IMAGE_AGE="$2"
      shift 2
      ;;
    --unused-image-age)
      [[ $# -ge 2 ]] || die "После --unused-image-age нужно указать значение"
      UNUSED_IMAGE_AGE="$2"
      shift 2
      ;;
    --network-age)
      [[ $# -ge 2 ]] || die "После --network-age нужно указать значение"
      NETWORK_AGE="$2"
      shift 2
      ;;
    --builder-age)
      [[ $# -ge 2 ]] || die "После --builder-age нужно указать значение"
      BUILDER_AGE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Неизвестный аргумент: $1"
      ;;
  esac
done

validate_duration --container-age "$CONTAINER_AGE"
validate_duration --image-age "$IMAGE_AGE"
validate_duration --unused-image-age "$UNUSED_IMAGE_AGE"
validate_duration --network-age "$NETWORK_AGE"
validate_duration --builder-age "$BUILDER_AGE"

setup_report_stream
require_docker
discover_lock_dirs
scan_lock_dirs

echo "Режим: $([[ $APPLY -eq 1 ]] && echo apply || echo plan)"
echo "Отчет: $REPORT_FILE"
echo "Volume prune: отключен намеренно"
echo "Порог контейнеров: $CONTAINER_AGE"
echo "Порог dangling-образов: $IMAGE_AGE"
echo "Порог unused tag-образов: $UNUSED_IMAGE_AGE"
echo "Порог сетей: $NETWORK_AGE"
echo "Порог build cache: $BUILDER_AGE"
echo "Удалять старые unused tag-образы: $([[ $INCLUDE_UNUSED_IMAGES -eq 1 ]] && echo yes || echo no)"
echo "Очищать build cache: $([[ $SKIP_BUILD_CACHE -eq 1 ]] && echo no || echo yes)"

if [[ ${#ACTIVE_LOCKS[@]} -gt 0 ]]; then
  echo
  echo "Активные maintenance-lock файлы:"
  printf '%s\n' "${ACTIVE_LOCKS[@]}"

  if [[ $APPLY -eq 1 ]]; then
    die "На хосте уже идет операция обслуживания. Повторите cleanup позже."
  fi

  warn "Показан только dry-run. Реальный cleanup сейчас будет небезопасен."
fi

if [[ ${#STALE_LOCKS[@]} -gt 0 ]]; then
  echo
  echo "Устаревшие lock-файлы:"
  printf '%s\n' "${STALE_LOCKS[@]}"
fi

echo
echo "== Состояние Docker до cleanup =="
docker system df || true

build_referenced_image_index
gather_stopped_container_candidates
gather_image_candidates
gather_network_candidates

print_candidate_block "Остановленные контейнеры к удалению (${#CONTAINER_IDS[@]})" "${CONTAINER_LINES[@]}"
print_candidate_block "Образы к удалению (${#IMAGE_IDS[@]})" "${IMAGE_LINES[@]}"
print_candidate_block "Пользовательские сети к удалению (${#NETWORK_IDS[@]})" "${NETWORK_LINES[@]}"

echo
echo "== Build cache =="
if [[ $SKIP_BUILD_CACHE -eq 1 ]]; then
  echo "Очистка build cache отключена флагом --skip-build-cache."
else
  echo "Точный dry-run для build cache Docker CLI не показывает."
  echo "В режиме apply будет выполнен: docker builder prune -f --filter until=$BUILDER_AGE"
fi

if [[ $APPLY -eq 0 ]]; then
  echo
  echo "Dry-run завершен. Для реального удаления повторите команду с --apply."
  exit 0
fi

if [[ ${#STALE_LOCKS[@]} -gt 0 ]]; then
  purge_stale_locks
fi

acquire_barrier_locks

FAILURES=0
remove_stopped_containers || FAILURES=$((FAILURES + 1))
remove_image_candidates || FAILURES=$((FAILURES + 1))
remove_network_candidates || FAILURES=$((FAILURES + 1))
prune_builder_cache || FAILURES=$((FAILURES + 1))

echo
echo "== Состояние Docker после cleanup =="
docker system df || true

if [[ $FAILURES -ne 0 ]]; then
  die "Cleanup завершился с ошибками. Подробности сохранены в отчете: $REPORT_FILE"
fi

echo
echo "Cleanup завершен успешно."
