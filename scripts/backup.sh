#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт резервного копирования контура.
# По умолчанию он кратко останавливает прикладные сервисы EspoCRM,
# чтобы дамп БД и архив файлов были сняты из одной спокойной точки.
#
# Скрипт создает:
# - gzip-дамп базы данных;
# - tar.gz-архив файлов приложения;
# - SHA-256 sidecar-файлы для проверки целостности;
# - manifest-файлы с метаданными бэкапа;
# - ротацию старых резервных копий.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/backup.sh [dev|prod] [--skip-db] [--skip-files] [--no-stop]

Примеры:
  ./scripts/backup.sh
  ./scripts/backup.sh prod
  ./scripts/backup.sh dev
  ./scripts/backup.sh prod --skip-files
  ./scripts/backup.sh dev --skip-db
  ./scripts/backup.sh prod --no-stop
EOF
}

restart_app_services_on_exit() {
  if [[ ${RESTART_APP_SERVICES_ON_EXIT:-0} -eq 1 ]]; then
    warn "Бэкап завершился нештатно, возвращаю прикладные сервисы"
    start_app_services || warn "Не удалось автоматически поднять прикладные сервисы после сбоя backup"
    RESTART_APP_SERVICES_ON_EXIT=0
  fi
}

parse_contour_arg "$@"
SKIP_DB=0
SKIP_FILES=0
NO_STOP=0

for arg in "${POSITIONAL_ARGS[@]}"; do
  case "$arg" in
    --skip-db)
      SKIP_DB=1
      ;;
    --skip-files)
      SKIP_FILES=1
      ;;
    --no-stop)
      NO_STOP=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Неизвестный аргумент: $arg"
      ;;
  esac
done

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Нечего резервировать: одновременно заданы --skip-db и --skip-files"
fi

resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock backup

if [[ $SKIP_DB -eq 0 || $NO_STOP -eq 0 ]]; then
  require_compose
fi

if [[ $SKIP_DB -eq 0 ]]; then
  # Бэкап БД возможен только при реально запущенном контейнере MariaDB.
  require_service_running db
fi

APP_SERVICES_WERE_RUNNING=0
SHOULD_RESTART_APP_SERVICES=0
RESTART_APP_SERVICES_ON_EXIT=0

if [[ $NO_STOP -eq 0 ]]; then
  if app_services_running; then
    APP_SERVICES_WERE_RUNNING=1
    SHOULD_RESTART_APP_SERVICES=1
    RESTART_APP_SERVICES_ON_EXIT=1
    append_trap 'restart_app_services_on_exit' EXIT
    echo "Остановка прикладных сервисов на время консистентного backup"
    stop_app_services
  else
    echo "Прикладные сервисы уже остановлены, дополнительная остановка не требуется"
  fi
else
  warn "Бэкап создается без остановки прикладных сервисов: строгая консистентность не гарантируется"
fi

STAMP="$(date +%F_%H-%M-%S)"
BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
DB_DIR="$BACKUP_ROOT_ABS/db"
FILES_DIR="$BACKUP_ROOT_ABS/files"
MANIFESTS_DIR="$BACKUP_ROOT_ABS/manifests"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"

DB_FILE="$DB_DIR/${NAME_PREFIX}_${STAMP}.sql.gz"
FILES_FILE="$FILES_DIR/${NAME_PREFIX}_files_${STAMP}.tar.gz"
DB_SUM_FILE="${DB_FILE}.sha256"
FILES_SUM_FILE="${FILES_FILE}.sha256"
MANIFEST_FILE="$MANIFESTS_DIR/${NAME_PREFIX}_${STAMP}.manifest.txt"
MANIFEST_JSON_FILE="$MANIFESTS_DIR/${NAME_PREFIX}_${STAMP}.manifest.json"

DB_FILE_TMP="${DB_FILE}.tmp"
FILES_FILE_TMP="${FILES_FILE}.tmp"
DB_SUM_FILE_TMP="${DB_SUM_FILE}.tmp"
FILES_SUM_FILE_TMP="${FILES_SUM_FILE}.tmp"
MANIFEST_FILE_TMP="${MANIFEST_FILE}.tmp"
MANIFEST_JSON_FILE_TMP="${MANIFEST_JSON_FILE}.tmp"

trap 'rm -f "$DB_FILE_TMP" "$FILES_FILE_TMP" "$DB_SUM_FILE_TMP" "$FILES_SUM_FILE_TMP" "$MANIFEST_FILE_TMP" "$MANIFEST_JSON_FILE_TMP"' EXIT

print_context

STEP=1
if [[ $SKIP_DB -eq 0 ]]; then
  echo "[$STEP/4] Создание дампа базы данных: $DB_FILE"
  # Внутри контейнера используем переменные MariaDB из самого контейнера.
  # shellcheck disable=SC2016
  compose exec -T db sh -lc 'exec mariadb-dump -u"$MARIADB_USER" -p"$MARIADB_PASSWORD" "$MARIADB_DATABASE" --single-transaction --quick --routines --triggers --events' \
    | gzip -9 > "$DB_FILE_TMP"
  mv "$DB_FILE_TMP" "$DB_FILE"
else
  echo "[$STEP/4] Резервное копирование базы данных пропущено по флагу --skip-db"
fi
STEP=$((STEP + 1))

if [[ $SKIP_FILES -eq 0 ]]; then
  echo "[$STEP/4] Архивация файлов приложения: $FILES_FILE"
  tar -C "$(dirname "$ESPO_STORAGE_ABS")" -czf "$FILES_FILE_TMP" "$(basename "$ESPO_STORAGE_ABS")"
  mv "$FILES_FILE_TMP" "$FILES_FILE"
else
  echo "[$STEP/4] Резервное копирование файлов пропущено по флагу --skip-files"
fi
STEP=$((STEP + 1))

echo "[$STEP/4] Формирование контрольных сумм и manifest-файлов"

if [[ $SKIP_DB -eq 0 ]]; then
  DB_SHA256="$(sha256_file "$DB_FILE")"
  DB_SIZE_BYTES="$(file_size_bytes "$DB_FILE")"
  write_sha256_sidecar "$DB_FILE" "$DB_SHA256" "$DB_SUM_FILE_TMP"
  mv "$DB_SUM_FILE_TMP" "$DB_SUM_FILE"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  FILES_SHA256="$(sha256_file "$FILES_FILE")"
  FILES_SIZE_BYTES="$(file_size_bytes "$FILES_FILE")"
  write_sha256_sidecar "$FILES_FILE" "$FILES_SHA256" "$FILES_SUM_FILE_TMP"
  mv "$FILES_SUM_FILE_TMP" "$FILES_SUM_FILE"
fi

cat > "$MANIFEST_FILE_TMP" <<EOF
created_at=$STAMP
contour=$ESPO_ENV
compose_project=$COMPOSE_PROJECT_NAME
env_file=$(basename "$ENV_FILE")
espocrm_image=${ESPOCRM_IMAGE:-}
mariadb_tag=${MARIADB_TAG:-}
retention_days=$RETENTION_DAYS
db_backup_created=$((1 - SKIP_DB))
files_backup_created=$((1 - SKIP_FILES))
consistent_snapshot=$((1 - NO_STOP))
app_services_were_running=$APP_SERVICES_WERE_RUNNING
EOF

if [[ $SKIP_DB -eq 0 ]]; then
  cat >> "$MANIFEST_FILE_TMP" <<EOF
db_backup_file=$(basename "$DB_FILE")
db_backup_checksum_file=$(basename "$DB_SUM_FILE")
db_backup_sha256=$DB_SHA256
db_backup_size_bytes=$DB_SIZE_BYTES
EOF
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  cat >> "$MANIFEST_FILE_TMP" <<EOF
files_backup_file=$(basename "$FILES_FILE")
files_backup_checksum_file=$(basename "$FILES_SUM_FILE")
files_backup_sha256=$FILES_SHA256
files_backup_size_bytes=$FILES_SIZE_BYTES
EOF
fi

mv "$MANIFEST_FILE_TMP" "$MANIFEST_FILE"

{
  printf '{\n'
  printf '  "created_at": "%s",\n' "$(json_escape "$STAMP")"
  printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
  printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
  printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
  printf '  "espocrm_image": "%s",\n' "$(json_escape "${ESPOCRM_IMAGE:-}")"
  printf '  "mariadb_tag": "%s",\n' "$(json_escape "${MARIADB_TAG:-}")"
  printf '  "retention_days": %s,\n' "$RETENTION_DAYS"
  printf '  "consistent_snapshot": %s,\n' "$([[ $NO_STOP -eq 0 ]] && echo true || echo false)"
  printf '  "app_services_were_running": %s,\n' "$([[ $APP_SERVICES_WERE_RUNNING -eq 1 ]] && echo true || echo false)"
  printf '  "db_backup_created": %s,\n' "$([[ $SKIP_DB -eq 0 ]] && echo true || echo false)"
  printf '  "files_backup_created": %s' "$([[ $SKIP_FILES -eq 0 ]] && echo true || echo false)"

  if [[ $SKIP_DB -eq 0 ]]; then
    printf ',\n  "db_backup": {\n'
    printf '    "file": "%s",\n' "$(json_escape "$(basename "$DB_FILE")")"
    printf '    "checksum_file": "%s",\n' "$(json_escape "$(basename "$DB_SUM_FILE")")"
    printf '    "sha256": "%s",\n' "$(json_escape "$DB_SHA256")"
    printf '    "size_bytes": %s\n' "$DB_SIZE_BYTES"
    printf '  }'
  fi

  if [[ $SKIP_FILES -eq 0 ]]; then
    printf ',\n  "files_backup": {\n'
    printf '    "file": "%s",\n' "$(json_escape "$(basename "$FILES_FILE")")"
    printf '    "checksum_file": "%s",\n' "$(json_escape "$(basename "$FILES_SUM_FILE")")"
    printf '    "sha256": "%s",\n' "$(json_escape "$FILES_SHA256")"
    printf '    "size_bytes": %s\n' "$FILES_SIZE_BYTES"
    printf '  }'
  fi

  printf '\n}\n'
} > "$MANIFEST_JSON_FILE_TMP"
mv "$MANIFEST_JSON_FILE_TMP" "$MANIFEST_JSON_FILE"
STEP=$((STEP + 1))

echo "[$STEP/4] Удаление бэкапов старше $RETENTION_DAYS дней"
cleanup_old_files "$DB_DIR" "$RETENTION_DAYS" '*.sql.gz' '*.sql.gz.sha256'
cleanup_old_files "$FILES_DIR" "$RETENTION_DAYS" '*.tar.gz' '*.tar.gz.sha256'
cleanup_old_files "$MANIFESTS_DIR" "$RETENTION_DAYS" '*.manifest.txt' '*.manifest.json'

if [[ $SHOULD_RESTART_APP_SERVICES -eq 1 ]]; then
  echo "Запуск прикладных сервисов после успешного backup"
  start_app_services
  RESTART_APP_SERVICES_ON_EXIT=0
fi

echo "Резервное копирование завершено:"
if [[ $SKIP_DB -eq 0 ]]; then
  echo "  База данных: $DB_FILE"
  echo "  Checksum DB: $DB_SUM_FILE"
fi
if [[ $SKIP_FILES -eq 0 ]]; then
  echo "  Файлы:       $FILES_FILE"
  echo "  Checksum FS: $FILES_SUM_FILE"
fi
echo "  Manifest:    $MANIFEST_FILE"
echo "  Manifest JSON: $MANIFEST_JSON_FILE"
