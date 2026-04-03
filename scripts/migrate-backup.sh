#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт миграции резервной копии между контурами.
# Он позволяет переносить состояние:
# - из dev в prod;
# - из prod в dev.
#
# По умолчанию берет последние доступные backup-файлы источника,
# но также умеет работать и с явно указанными путями.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/migrate-backup.sh <from> <to> [--db-backup PATH] [--files-backup PATH] [--skip-db] [--skip-files] [--no-start]

Примеры:
  ./scripts/migrate-backup.sh dev prod
  ./scripts/migrate-backup.sh prod dev
  ./scripts/migrate-backup.sh dev prod --db-backup /opt/espo/backups/dev/db/espocrm-dev_2026-04-03_10-00-00.sql.gz --files-backup /opt/espo/backups/dev/files/espocrm-dev_files_2026-04-03_10-00-00.tar.gz
EOF
}

# Загружаем переменные выбранного контура во временные переменные.
# Это удобно, когда в одном процессе надо поочередно работать и с источником, и с целью.
load_contour_vars() {
  local contour="$1"

  # shellcheck disable=SC2034
  ESPO_ENV="$contour"
  resolve_env_file
  load_env

  LOADED_ENV_FILE="$ENV_FILE"
  LOADED_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
}

# Для миграции нужны минимум два аргумента: источник и цель.
if [[ $# -lt 2 ]]; then
  usage >&2
  exit 1
fi

SOURCE_CONTOUR="$1"
TARGET_CONTOUR="$2"
shift 2

# Разрешаем только два известных контура.
case "$SOURCE_CONTOUR" in
  dev|prod) ;;
  *) die "Исходный контур должен быть dev или prod" ;;
esac

case "$TARGET_CONTOUR" in
  dev|prod) ;;
  *) die "Целевой контур должен быть dev или prod" ;;
esac

# Перенос "сам в себя" бессмысленен.
[[ "$SOURCE_CONTOUR" != "$TARGET_CONTOUR" ]] || die "Исходный и целевой контуры должны различаться"

# Значения по умолчанию.
DB_BACKUP=""
FILES_BACKUP=""
SKIP_DB=0
SKIP_FILES=0
NO_START=0

# Разбираем опции командной строки.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --db-backup)
      [[ $# -ge 2 ]] || die "После --db-backup должен быть путь"
      DB_BACKUP="$(caller_path "$2")"
      shift 2
      ;;
    --files-backup)
      [[ $# -ge 2 ]] || die "После --files-backup должен быть путь"
      FILES_BACKUP="$(caller_path "$2")"
      shift 2
      ;;
    --skip-db)
      SKIP_DB=1
      shift
      ;;
    --skip-files)
      SKIP_FILES=1
      shift
      ;;
    --no-start)
      NO_START=1
      shift
      ;;
    *)
      usage >&2
      die "Неизвестный аргумент: $1"
      ;;
  esac
done

# Хотя бы что-то переносить нужно.
if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Нечего переносить: одновременно заданы --skip-db и --skip-files"
fi

require_compose

# Загружаем параметры источника.
load_contour_vars "$SOURCE_CONTOUR"
SOURCE_ENV_FILE="$LOADED_ENV_FILE"
SOURCE_BACKUP_ROOT_ABS="$LOADED_BACKUP_ROOT_ABS"

# Если путь к бэкапу БД не передан явно, берем последний из каталога источника.
if [[ $SKIP_DB -eq 0 && -z "$DB_BACKUP" ]]; then
  DB_BACKUP="$(latest_backup_file "$SOURCE_BACKUP_ROOT_ABS/db" '*.sql.gz' || true)"
fi

# То же самое для файлового бэкапа.
if [[ $SKIP_FILES -eq 0 && -z "$FILES_BACKUP" ]]; then
  FILES_BACKUP="$(latest_backup_file "$SOURCE_BACKUP_ROOT_ABS/files" '*.tar.gz' || true)"
fi

# Валидируем, что нужные backup-файлы реально существуют.
if [[ $SKIP_DB -eq 0 ]]; then
  [[ -n "$DB_BACKUP" && -f "$DB_BACKUP" ]] || die "Не найден backup базы для исходного контура '$SOURCE_CONTOUR'"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  [[ -n "$FILES_BACKUP" && -f "$FILES_BACKUP" ]] || die "Не найден backup файлов для исходного контура '$SOURCE_CONTOUR'"
fi

# Загружаем параметры цели.
load_contour_vars "$TARGET_CONTOUR"
TARGET_ENV_FILE="$LOADED_ENV_FILE"
ensure_runtime_dirs
acquire_maintenance_lock migrate

# Печатаем сводку операции до начала изменения данных.
echo "[info] Исходный контур: $SOURCE_CONTOUR"
echo "[info] Env-файл источника: $SOURCE_ENV_FILE"
echo "[info] Целевой контур: $TARGET_CONTOUR"
echo "[info] Env-файл цели: $TARGET_ENV_FILE"
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[info] Бэкап БД: $DB_BACKUP"
fi
if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[info] Бэкап файлов: $FILES_BACKUP"
fi

echo "[1/4] Убеждаемся, что контейнер БД целевого контура запущен"
run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" up -d db

echo "[2/4] Останавливаем прикладные сервисы целевого контура"
# Если какой-то сервис еще не создан, команда stop может завершиться ошибкой —
# это не критично для миграции, поэтому допускаем такой сценарий.
run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" stop espocrm espocrm-daemon espocrm-websocket || true

STEP=3
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[$STEP/4] Восстанавливаем базу данных в контур $TARGET_CONTOUR"
  run_repo_script "$SCRIPT_DIR/restore-db.sh" "$TARGET_CONTOUR" "$DB_BACKUP" --no-stop --no-start
  STEP=$((STEP + 1))
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[$STEP/4] Восстанавливаем файлы в контур $TARGET_CONTOUR"
  run_repo_script "$SCRIPT_DIR/restore-files.sh" "$TARGET_CONTOUR" "$FILES_BACKUP" --no-stop --no-start
fi

if [[ $NO_START -eq 0 ]]; then
  echo "[4/4] Запускаем целевой контур после миграции"
  run_repo_script "$SCRIPT_DIR/stack.sh" "$TARGET_CONTOUR" up -d
else
  echo "[4/4] Целевой контур оставлен остановленным по флагу --no-start"
fi

echo "Миграция завершена: $SOURCE_CONTOUR -> $TARGET_CONTOUR"
