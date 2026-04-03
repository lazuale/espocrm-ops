#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт восстановления файлов приложения.
# Перед распаковкой он очищает целевой каталог контура,
# чтобы в нем не осталось старых файлов после предыдущего состояния.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/restore-files.sh [dev|prod] /путь/к/backup.tar.gz [--snapshot-before-restore] [--no-stop] [--no-start]

Примеры:
  ./scripts/restore-files.sh prod /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/restore-files.sh dev /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/restore-files.sh prod /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz --snapshot-before-restore
  ./scripts/restore-files.sh prod /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz --no-start
EOF
}

# Разбираем аргументы.
parse_contour_arg "$@"
NO_STOP=0
NO_START=0
SNAPSHOT_BEFORE_RESTORE=0
BACKUP_ARG=""

for arg in "${POSITIONAL_ARGS[@]}"; do
  case "$arg" in
    --snapshot-before-restore)
      SNAPSHOT_BEFORE_RESTORE=1
      ;;
    --no-stop)
      NO_STOP=1
      ;;
    --no-start)
      NO_START=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      usage >&2
      die "Неизвестный аргумент: $arg"
      ;;
    *)
      if [[ -n "$BACKUP_ARG" ]]; then
        usage >&2
        die "Передано больше одного пути к backup-файлу"
      fi
      BACKUP_ARG="$arg"
      ;;
  esac
done

if [[ -z "$BACKUP_ARG" ]]; then
  usage >&2
  exit 1
fi

# Приводим путь к backup-файлу к абсолютному виду.
BACKUP_FILE="$(caller_path "$BACKUP_ARG")"
if [[ ! -f "$BACKUP_FILE" ]]; then
  die "Файл бэкапа не найден: $BACKUP_FILE"
fi

# Готовим окружение.
resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock restore-files
require_compose

# Целевой каталог с файлами приложения и его родитель.
TARGET_STORAGE="$(root_path "$ESPO_STORAGE_DIR")"
TARGET_PARENT="$(dirname "$TARGET_STORAGE")"

# Если прикладные сервисы были активны до restore, остановим их
# и затем поднимем обратно после успешного завершения.
SHOULD_RESTART_APP_SERVICES=0
APP_SERVICES_WERE_RUNNING=0
if [[ $NO_STOP -eq 0 ]]; then
  if app_services_running; then
    APP_SERVICES_WERE_RUNNING=1
    echo "Остановка прикладных сервисов перед восстановлением файлов"
    stop_app_services
    if [[ $NO_START -eq 0 ]]; then
      SHOULD_RESTART_APP_SERVICES=1
    fi
  else
    echo "Прикладные сервисы не запущены, останавливать нечего"
  fi
fi

if [[ $NO_STOP -eq 1 ]] && app_services_running; then
  APP_SERVICES_WERE_RUNNING=1
fi

print_context

if [[ $SNAPSHOT_BEFORE_RESTORE -eq 1 ]]; then
  if [[ $NO_STOP -eq 1 && $APP_SERVICES_WERE_RUNNING -eq 1 ]]; then
    warn "Snapshot перед restore будет создан без остановки прикладных сервисов"
  fi
  echo "Создание аварийного snapshot перед восстановлением файлов"
  SNAPSHOT_ARGS=("$ESPO_ENV" "--skip-db")
  if [[ $NO_STOP -eq 1 ]]; then
    SNAPSHOT_ARGS+=("--no-stop")
  fi
  ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/backup.sh" "${SNAPSHOT_ARGS[@]}"
fi

echo "Проверка целостности backup-файла с файлами"
verify_sha256_or_warn "$BACKUP_FILE"

echo "Очистка целевого каталога файлов: $TARGET_STORAGE"
safe_empty_dir "$TARGET_STORAGE"

echo "Восстановление файлов из $BACKUP_FILE"
tar -C "$TARGET_PARENT" -xzf "$BACKUP_FILE"

if [[ $SHOULD_RESTART_APP_SERVICES -eq 1 ]]; then
  echo "Запуск прикладных сервисов после успешного восстановления файлов"
  start_app_services
elif [[ $NO_START -eq 1 ]]; then
  echo "Прикладные сервисы оставлены остановленными по флагу --no-start"
fi

echo "Восстановление файлов завершено"
