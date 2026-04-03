#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт восстановления базы данных.
# Важно: он пересоздает целевую БД и затем импортирует дамп в "чистую" схему.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/restore-db.sh [dev|prod] /путь/к/backup.sql.gz [--snapshot-before-restore] [--no-stop] [--no-start]

Примеры:
  ./scripts/restore-db.sh prod /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/restore-db.sh dev /opt/espo/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/restore-db.sh prod /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --snapshot-before-restore
  ./scripts/restore-db.sh prod /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --no-start
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
acquire_maintenance_lock restore-db
require_compose

# Восстановление возможно только при запущенном контейнере БД.
require_service_running db

# Если прикладные сервисы были активны до restore, остановим их
# и затем поднимем обратно после успешного завершения.
SHOULD_RESTART_APP_SERVICES=0
APP_SERVICES_WERE_RUNNING=0
if [[ $NO_STOP -eq 0 ]]; then
  if app_services_running; then
    APP_SERVICES_WERE_RUNNING=1
    echo "Остановка прикладных сервисов перед восстановлением базы данных"
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
  echo "Создание аварийного snapshot перед восстановлением базы данных"
  SNAPSHOT_ARGS=("$ESPO_ENV" "--skip-files")
  if [[ $NO_STOP -eq 1 ]]; then
    SNAPSHOT_ARGS+=("--no-stop")
  fi
  run_repo_script "$SCRIPT_DIR/backup.sh" "${SNAPSHOT_ARGS[@]}"
fi

echo "Проверка целостности backup-файла базы данных"
verify_sha256_or_warn "$BACKUP_FILE"

echo "Пересоздание базы данных $DB_NAME перед восстановлением"
{
  # Полностью удаляем старую БД, если она существует.
  printf "DROP DATABASE IF EXISTS \`%s\`;\n" "$DB_NAME"
  # Создаем БД с нужной кодировкой.
  printf "CREATE DATABASE \`%s\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n" "$DB_NAME"
  # Возвращаем прикладному пользователю полный доступ к ней.
  printf "GRANT ALL PRIVILEGES ON \`%s\`.* TO '%s'@'%%';\n" "$DB_NAME" "$DB_USER"
  # Сбрасываем кэш привилегий.
  printf 'FLUSH PRIVILEGES;\n'
} | compose exec -T db sh -lc "mariadb -uroot -p\"\$MARIADB_ROOT_PASSWORD\""

echo "Восстановление базы данных из $BACKUP_FILE"
# Импорт выполняем от root, чтобы не упереться в права,
# если в дампе есть служебные объекты.
gunzip -c "$BACKUP_FILE" | compose exec -T db sh -lc "mariadb -uroot -p\"\$MARIADB_ROOT_PASSWORD\" \"\$MARIADB_DATABASE\""

if [[ $SHOULD_RESTART_APP_SERVICES -eq 1 ]]; then
  echo "Запуск прикладных сервисов после успешного восстановления базы данных"
  start_app_services
elif [[ $NO_START -eq 1 ]]; then
  echo "Прикладные сервисы оставлены остановленными по флагу --no-start"
fi

echo "Восстановление базы данных завершено"
