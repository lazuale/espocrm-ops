#!/usr/bin/env bash
set -Eeuo pipefail

# Аварийный rollback контура на последний валидный backup-набор.
# По умолчанию скрипт:
# - ищет последний complete backup-набор с подтвержденными checksum;
# - снимает аварийный snapshot текущего состояния перед перезаписью;
# - останавливает прикладные сервисы;
# - восстанавливает БД и файлы;
# - поднимает контур обратно и проверяет готовность.
#
# Если rollback завершается ошибкой, автоматически собирается support bundle.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/rollback.sh [dev|prod] [--db-backup PATH --files-backup PATH] [--no-snapshot] [--no-start] [--skip-http-probe] [--timeout SEC]

Примеры:
  ./scripts/rollback.sh prod
  ./scripts/rollback.sh dev --timeout 900
  ./scripts/rollback.sh prod --db-backup /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --files-backup /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/rollback.sh prod --no-snapshot --no-start
EOF
}

# Разбираем имя DB backup-файла и извлекаем prefix + stamp.
parse_db_backup_name() {
  local file="$1"
  local base prefix stamp

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.sql\.gz$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    printf '%s|%s\n' "$prefix" "$stamp"
    return 0
  fi

  return 1
}

# Проверяем, что manifest TXT похож на ожидаемый backup-manifest.
manifest_txt_is_valid() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  grep -q '^created_at=' "$file" \
    && grep -q '^contour=' "$file" \
    && grep -q '^compose_project=' "$file"
}

# Проверяем, что manifest JSON содержит минимальные обязательные ключи.
manifest_json_is_valid() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  grep -q '"created_at"' "$file" \
    && grep -q '"contour"' "$file" \
    && grep -q '"compose_project"' "$file"
}

# Ищем последний backup-набор, который можно считать валидным для rollback:
# - есть DB backup;
# - есть files backup;
# - есть sidecar-файлы;
# - checksum совпадает;
# - manifest TXT/JSON присутствуют и выглядят корректно.
select_latest_valid_backup_set() {
  local db_file prefix stamp files_file manifest_txt manifest_json
  local found=0

  while IFS='|' read -r stamp db_file; do
    [[ -n "$db_file" ]] || continue

    if ! IFS='|' read -r prefix stamp < <(parse_db_backup_name "$db_file"); then
      continue
    fi

    files_file="$BACKUP_ROOT_ABS/files/${prefix}_files_${stamp}.tar.gz"
    manifest_txt="$BACKUP_ROOT_ABS/manifests/${prefix}_${stamp}.manifest.txt"
    manifest_json="$BACKUP_ROOT_ABS/manifests/${prefix}_${stamp}.manifest.json"

    [[ -f "$files_file" ]] || continue
    [[ -f "${db_file}.sha256" ]] || continue
    [[ -f "${files_file}.sha256" ]] || continue
    manifest_txt_is_valid "$manifest_txt" || continue
    manifest_json_is_valid "$manifest_json" || continue
    verify_sha256_sidecar "$db_file" "${db_file}.sha256" || continue
    verify_sha256_sidecar "$files_file" "${files_file}.sha256" || continue

    SELECTED_PREFIX="$prefix"
    SELECTED_STAMP="$stamp"
    SELECTED_DB_BACKUP="$db_file"
    SELECTED_FILES_BACKUP="$files_file"
    SELECTED_MANIFEST_TXT="$manifest_txt"
    SELECTED_MANIFEST_JSON="$manifest_json"
    found=1
    break
  done < <(
    find "$BACKUP_ROOT_ABS/db" -maxdepth 1 -type f -name '*.sql.gz' \
      | while IFS= read -r db_file; do
          if IFS='|' read -r _prefix stamp < <(parse_db_backup_name "$db_file"); then
            printf '%s|%s\n' "$stamp" "$db_file"
          fi
        done \
      | sort -r
  )

  [[ $found -eq 1 ]]
}

# Валидируем явно переданные backup-файлы, если пользователь захотел откатиться
# не на автоопределенный последний набор, а на конкретные пути.
validate_manual_backup_pair() {
  local db_file="$1"
  local files_file="$2"

  [[ -f "$db_file" ]] || die "Не найден DB backup-файл: $db_file"
  [[ -f "$files_file" ]] || die "Не найден files backup-файл: $files_file"
  [[ -f "${db_file}.sha256" ]] || die "Не найден checksum-файл для DB backup: ${db_file}.sha256"
  [[ -f "${files_file}.sha256" ]] || die "Не найден checksum-файл для files backup: ${files_file}.sha256"

  verify_sha256_sidecar "$db_file" "${db_file}.sha256" || die "Контрольная сумма не совпадает для DB backup: $db_file"
  verify_sha256_sidecar "$files_file" "${files_file}.sha256" || die "Контрольная сумма не совпадает для files backup: $files_file"

  if IFS='|' read -r SELECTED_PREFIX SELECTED_STAMP < <(parse_db_backup_name "$db_file"); then
    SELECTED_MANIFEST_TXT="$BACKUP_ROOT_ABS/manifests/${SELECTED_PREFIX}_${SELECTED_STAMP}.manifest.txt"
    SELECTED_MANIFEST_JSON="$BACKUP_ROOT_ABS/manifests/${SELECTED_PREFIX}_${SELECTED_STAMP}.manifest.json"
  else
    SELECTED_PREFIX="manual"
    SELECTED_STAMP="manual"
    SELECTED_MANIFEST_TXT=""
    SELECTED_MANIFEST_JSON=""
  fi

  SELECTED_DB_BACKUP="$db_file"
  SELECTED_FILES_BACKUP="$files_file"
}

# Сохраняем краткий план rollback в reports, чтобы потом было видно:
# какой набор использовали и с какими флагами запускали процедуру.
write_rollback_plan_reports() {
  cat > "$ROLLBACK_PLAN_TXT" <<EOF
Rollback plan for EspoCRM contour
created_at=$STAMP
contour=$ESPO_ENV
compose_project=$COMPOSE_PROJECT_NAME
env_file=$(basename "$ENV_FILE")
selection_mode=$SELECTION_MODE
selected_prefix=$SELECTED_PREFIX
selected_stamp=$SELECTED_STAMP
db_backup=$SELECTED_DB_BACKUP
files_backup=$SELECTED_FILES_BACKUP
manifest_txt=${SELECTED_MANIFEST_TXT:-}
manifest_json=${SELECTED_MANIFEST_JSON:-}
snapshot_enabled=$([[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]] && echo true || echo false)
no_start=$([[ $NO_START -eq 1 ]] && echo true || echo false)
skip_http_probe=$([[ $SKIP_HTTP_PROBE -eq 1 ]] && echo true || echo false)
timeout_seconds=$TIMEOUT_SECONDS
EOF

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$STAMP")"
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "env_file": "%s",\n' "$(json_escape "$(basename "$ENV_FILE")")"
    printf '  "selection_mode": "%s",\n' "$(json_escape "$SELECTION_MODE")"
    printf '  "selected_prefix": "%s",\n' "$(json_escape "$SELECTED_PREFIX")"
    printf '  "selected_stamp": "%s",\n' "$(json_escape "$SELECTED_STAMP")"
    printf '  "db_backup": "%s",\n' "$(json_escape "$SELECTED_DB_BACKUP")"
    printf '  "files_backup": "%s",\n' "$(json_escape "$SELECTED_FILES_BACKUP")"
    printf '  "manifest_txt": %s,\n' "$(if [[ -n "${SELECTED_MANIFEST_TXT:-}" ]]; then printf '"%s"' "$(json_escape "$SELECTED_MANIFEST_TXT")"; else printf 'null'; fi)"
    printf '  "manifest_json": %s,\n' "$(if [[ -n "${SELECTED_MANIFEST_JSON:-}" ]]; then printf '"%s"' "$(json_escape "$SELECTED_MANIFEST_JSON")"; else printf 'null'; fi)"
    printf '  "snapshot_enabled": %s,\n' "$([[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]] && echo true || echo false)"
    printf '  "no_start": %s,\n' "$([[ $NO_START -eq 1 ]] && echo true || echo false)"
    printf '  "skip_http_probe": %s,\n' "$([[ $SKIP_HTTP_PROBE -eq 1 ]] && echo true || echo false)"
    printf '  "timeout_seconds": %s\n' "$TIMEOUT_SECONDS"
    printf '}\n'
  } > "$ROLLBACK_PLAN_JSON"
}

create_failure_bundle() {
  local bundle_path="$1"

  set +e
  ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/support-bundle.sh" "$ESPO_ENV" --output "$bundle_path"
  local bundle_exit=$?
  set -e

  if [[ $bundle_exit -eq 0 ]]; then
    warn "Собран support bundle для разбора rollback-сбоя: $bundle_path"
  else
    warn "Не удалось собрать support bundle rollback автоматически"
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
    warn "Rollback завершился ошибкой, собираю support bundle"
    create_failure_bundle "$FAILURE_BUNDLE_PATH"
  fi

  exit "$exit_code"
}

wait_for_application_stack() {
  local timeout_seconds="$1"

  wait_for_service_ready db "$timeout_seconds"
  wait_for_service_ready espocrm "$timeout_seconds"
  wait_for_service_ready espocrm-daemon "$timeout_seconds"
  wait_for_service_ready espocrm-websocket "$timeout_seconds"
}

parse_contour_arg "$@"
DB_BACKUP_ARG=""
FILES_BACKUP_ARG=""
SNAPSHOT_BEFORE_ROLLBACK=1
NO_START=0
SKIP_HTTP_PROBE=0
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --db-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --db-backup должен идти путь"
      DB_BACKUP_ARG="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --files-backup должен идти путь"
      FILES_BACKUP_ARG="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --no-snapshot)
      SNAPSHOT_BEFORE_ROLLBACK=0
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --no-start)
      NO_START=1
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

if [[ -n "$DB_BACKUP_ARG" || -n "$FILES_BACKUP_ARG" ]]; then
  [[ -n "$DB_BACKUP_ARG" && -n "$FILES_BACKUP_ARG" ]] || die "Для ручного rollback нужно указать и --db-backup, и --files-backup"
fi

resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock rollback
require_compose

STAMP="$(date +%F_%H-%M-%S)"
BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
REPORT_RETENTION="${REPORT_RETENTION_DAYS:-30}"
PRE_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_pre-rollback_${STAMP}.txt"
PRE_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_pre-rollback_${STAMP}.json"
POST_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_post-rollback_${STAMP}.txt"
POST_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_post-rollback_${STAMP}.json"
ROLLBACK_PLAN_TXT="$REPORTS_DIR/${NAME_PREFIX}_rollback-plan_${STAMP}.txt"
ROLLBACK_PLAN_JSON="$REPORTS_DIR/${NAME_PREFIX}_rollback-plan_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SUPPORT_DIR/${NAME_PREFIX}_rollback-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0
BUNDLE_ON_FAIL=1

trap 'on_error' ERR

if [[ -n "$DB_BACKUP_ARG" ]]; then
  SELECTION_MODE="manual"
  validate_manual_backup_pair "$DB_BACKUP_ARG" "$FILES_BACKUP_ARG"
else
  SELECTION_MODE="auto-latest-valid"
  if ! select_latest_valid_backup_set; then
    die "Не найден ни один валидный backup-набор для rollback в $BACKUP_ROOT_ABS"
  fi
fi

write_rollback_plan_reports
print_context

echo "[info] Режим выбора backup-набора: $SELECTION_MODE"
echo "[info] Выбран prefix: $SELECTED_PREFIX"
echo "[info] Выбран stamp: $SELECTED_STAMP"
echo "[info] DB backup: $SELECTED_DB_BACKUP"
echo "[info] Files backup: $SELECTED_FILES_BACKUP"
if [[ -n "${SELECTED_MANIFEST_TXT:-}" ]]; then
  echo "[info] Manifest TXT: $SELECTED_MANIFEST_TXT"
fi
if [[ -n "${SELECTED_MANIFEST_JSON:-}" ]]; then
  echo "[info] Manifest JSON: $SELECTED_MANIFEST_JSON"
fi

echo "[1/7] Фиксация текущего статуса контура"
ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$PRE_REPORT_TXT"
ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$PRE_REPORT_JSON"

echo "[2/7] Подъем контейнера базы данных при необходимости"
if ! service_is_running db; then
  note "Контейнер БД не был запущен, временно поднимаю db для rollback"
  compose up -d db
fi
wait_for_service_ready db "$TIMEOUT_SECONDS"

echo "[3/7] Остановка прикладных сервисов перед rollback"
stop_app_services

echo "[4/7] Аварийный snapshot текущего состояния перед rollback"
if [[ $SNAPSHOT_BEFORE_ROLLBACK -eq 1 ]]; then
  ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"
else
  echo "Snapshot пропущен по флагу --no-snapshot"
fi

echo "[5/7] Восстановление базы данных"
ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/restore-db.sh" "$ESPO_ENV" "$SELECTED_DB_BACKUP" --no-stop --no-start

echo "[6/7] Восстановление файлов"
ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/restore-files.sh" "$ESPO_ENV" "$SELECTED_FILES_BACKUP" --no-stop --no-start

echo "[7/7] Возврат контура в рабочее состояние"
if [[ $NO_START -eq 0 ]]; then
  compose up -d
  wait_for_application_stack "$TIMEOUT_SECONDS"

  if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
    http_probe "$SITE_URL"
  else
    echo "HTTP-проверка пропущена по флагу --skip-http-probe"
  fi
else
  echo "Контур оставлен остановленным по флагу --no-start"
fi

ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$POST_REPORT_TXT"
ENV_FILE="$ENV_FILE" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$POST_REPORT_JSON"
cleanup_old_files "$REPORTS_DIR" "$REPORT_RETENTION" '*.txt' '*.json'

trap - ERR

echo "Rollback завершен успешно"
echo "Rollback plan:   $ROLLBACK_PLAN_TXT"
echo "Pre-status:      $PRE_REPORT_TXT"
echo "Post-status:     $POST_REPORT_TXT"
