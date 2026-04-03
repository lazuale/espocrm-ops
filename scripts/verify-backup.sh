#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт проверки целостности backup-файлов по checksum-файлам.
# Может работать как с явно указанными путями, так и с последними бэкапами контура.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/verify-backup.sh [dev|prod] [--db-backup PATH] [--files-backup PATH] [--skip-db] [--skip-files]

Примеры:
  ./scripts/verify-backup.sh prod
  ./scripts/verify-backup.sh dev
  ./scripts/verify-backup.sh prod --db-backup /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/verify-backup.sh dev --files-backup /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
EOF
}

verify_required_sidecar() {
  local label="$1"
  local backup_file="$2"
  local sidecar_file="${backup_file}.sha256"

  [[ -f "$backup_file" ]] || die "Не найден backup-файл для проверки: $backup_file"
  [[ -f "$sidecar_file" ]] || die "Не найден checksum-файл для проверки: $sidecar_file"

  if verify_sha256_sidecar "$backup_file" "$sidecar_file"; then
    echo "[ok] $label: контрольная сумма подтверждена"
    echo "      Файл: $backup_file"
    echo "      SHA:  $(read_sha256_sidecar "$sidecar_file")"
  else
    die "Контрольная сумма не совпадает для файла: $backup_file"
  fi
}

parse_contour_arg "$@"

DB_BACKUP=""
FILES_BACKUP=""
SKIP_DB=0
SKIP_FILES=0

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --db-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --db-backup должен быть путь"
      DB_BACKUP="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --files-backup должен быть путь"
      FILES_BACKUP="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --skip-db)
      SKIP_DB=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-files)
      SKIP_FILES=1
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

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Нечего проверять: одновременно заданы --skip-db и --skip-files"
fi

resolve_env_file
load_env
ensure_runtime_dirs

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

if [[ $SKIP_DB -eq 0 && -z "$DB_BACKUP" ]]; then
  DB_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/db" '*.sql.gz' || true)"
fi

if [[ $SKIP_FILES -eq 0 && -z "$FILES_BACKUP" ]]; then
  FILES_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/files" '*.tar.gz' || true)"
fi

print_context

if [[ $SKIP_DB -eq 0 ]]; then
  [[ -n "$DB_BACKUP" ]] || die "Не найден backup базы данных для проверки"
  verify_required_sidecar "База данных" "$DB_BACKUP"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  [[ -n "$FILES_BACKUP" ]] || die "Не найден backup файлов для проверки"
  verify_required_sidecar "Файлы" "$FILES_BACKUP"
fi

echo "Проверка backup-файлов завершена успешно"
