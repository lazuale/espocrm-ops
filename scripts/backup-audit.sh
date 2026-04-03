#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт аудита последних backup-артефактов контура.
# Он проверяет:
# - есть ли свежий backup БД;
# - есть ли свежий backup файлов;
# - существуют ли checksum-файлы;
# - совпадают ли контрольные суммы;
# - присутствуют ли manifest-файлы и базовые ключи в них.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/backup-audit.sh [dev|prod] [--json] [--skip-db] [--skip-files] [--max-db-age-hours N] [--max-files-age-hours N] [--no-verify-checksum]

Примеры:
  ./scripts/backup-audit.sh prod
  ./scripts/backup-audit.sh dev --json
  ./scripts/backup-audit.sh prod --max-db-age-hours 24 --max-files-age-hours 24
  ./scripts/backup-audit.sh dev --skip-files
EOF
}

OK_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0
JSON_MODE=0
SKIP_DB=0
SKIP_FILES=0
VERIFY_CHECKSUM=1
DB_MAX_AGE_OVERRIDE=""
FILES_MAX_AGE_OVERRIDE=""
declare -a FINDING_LEVELS=()
declare -a FINDING_MESSAGES=()

record_finding() {
  local level="$1"
  shift
  FINDING_LEVELS+=("$level")
  FINDING_MESSAGES+=("$*")
}

ok() {
  record_finding ok "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[ok] $*"
  OK_COUNT=$((OK_COUNT + 1))
}

warn_audit() {
  record_finding warn "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[warn] $*"
  WARN_COUNT=$((WARN_COUNT + 1))
}

fail_audit() {
  record_finding fail "$*"
  [[ $JSON_MODE -eq 0 ]] && echo "[fail] $*"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

json_bool() {
  if [[ "$1" == "1" ]]; then
    printf 'true'
  else
    printf 'false'
  fi
}

json_value_or_null() {
  local value="${1:-}"

  if [[ -n "$value" ]]; then
    printf '"%s"' "$(json_escape "$value")"
  else
    printf 'null'
  fi
}

json_number_or_null() {
  local value="${1:-}"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
  else
    printf 'null'
  fi
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

set_result() {
  local prefix="$1"
  local key="$2"
  local value="${3:-}"
  printf -v "${prefix}_${key}" '%s' "$value"
}

age_hours_for_file() {
  local file="$1"
  local now mtime

  now="$(date +%s)"
  mtime="$(file_mtime_epoch "$file")"
  echo $(((now - mtime) / 3600))
}

max_int() {
  local a="$1"
  local b="$2"

  if (( a >= b )); then
    echo "$a"
  else
    echo "$b"
  fi
}

audit_backup_artifact() {
  local prefix="$1"
  local label="$2"
  local directory="$3"
  local pattern="$4"
  local max_age_hours="$5"
  local file sidecar age_hours checksum_verified=0 status="ok" message=""

  file="$(latest_backup_file "$directory" "$pattern" || true)"

  if [[ -z "$file" ]]; then
    status="fail"
    message="Не найден ни один backup-файл в $directory по шаблону $pattern"
    fail_audit "[$label] $message"
    set_result "$prefix" STATUS "$status"
    set_result "$prefix" MESSAGE "$message"
    set_result "$prefix" FILE ""
    set_result "$prefix" SIDECAR ""
    set_result "$prefix" AGE_HOURS ""
    set_result "$prefix" MAX_AGE_HOURS "$max_age_hours"
    set_result "$prefix" CHECKSUM_VERIFIED "0"
    return
  fi

  sidecar="${file}.sha256"
  age_hours="$(age_hours_for_file "$file")"

  if [[ ! -f "$sidecar" ]]; then
    status="fail"
    message="Не найден checksum-файл рядом с backup: $sidecar"
    fail_audit "[$label] $message"
  fi

  if (( age_hours > max_age_hours )); then
    status="fail"
    if [[ -n "$message" ]]; then
      message="$message; "
    fi
    message="${message}backup старше порога: ${age_hours}ч > ${max_age_hours}ч"
    fail_audit "[$label] backup старше порога: ${age_hours}ч > ${max_age_hours}ч"
  fi

  if [[ -f "$sidecar" && $VERIFY_CHECKSUM -eq 1 ]]; then
    if verify_sha256_sidecar "$file" "$sidecar"; then
      checksum_verified=1
    else
      status="fail"
      if [[ -n "$message" ]]; then
        message="$message; "
      fi
      message="${message}контрольная сумма не совпадает"
      fail_audit "[$label] контрольная сумма не совпадает: $file"
    fi
  elif [[ -f "$sidecar" && $VERIFY_CHECKSUM -eq 0 ]]; then
    warn_audit "[$label] checksum-проверка пропущена по флагу --no-verify-checksum"
  fi

  if [[ "$status" == "ok" ]]; then
    message="Последний backup валиден и не старше порога"
    ok "[$label] $message"
  fi

  set_result "$prefix" STATUS "$status"
  set_result "$prefix" MESSAGE "$message"
  set_result "$prefix" FILE "$file"
  set_result "$prefix" SIDECAR "$sidecar"
  set_result "$prefix" AGE_HOURS "$age_hours"
  set_result "$prefix" MAX_AGE_HOURS "$max_age_hours"
  set_result "$prefix" CHECKSUM_VERIFIED "$checksum_verified"
}

audit_manifest_artifact() {
  local prefix="$1"
  local label="$2"
  local directory="$3"
  local pattern="$4"
  local max_age_hours="$5"
  shift 5

  local file age_hours status="ok" message="" required_pattern

  file="$(latest_backup_file "$directory" "$pattern" || true)"

  if [[ -z "$file" ]]; then
    status="fail"
    message="Не найден manifest-файл в $directory по шаблону $pattern"
    fail_audit "[$label] $message"
    set_result "$prefix" STATUS "$status"
    set_result "$prefix" MESSAGE "$message"
    set_result "$prefix" FILE ""
    set_result "$prefix" AGE_HOURS ""
    set_result "$prefix" MAX_AGE_HOURS "$max_age_hours"
    return
  fi

  age_hours="$(age_hours_for_file "$file")"
  if (( age_hours > max_age_hours )); then
    status="fail"
    message="manifest старше порога: ${age_hours}ч > ${max_age_hours}ч"
    fail_audit "[$label] $message"
  fi

  for required_pattern in "$@"; do
    if ! grep -q "$required_pattern" "$file"; then
      status="fail"
      if [[ -n "$message" ]]; then
        message="$message; "
      fi
      message="${message}в manifest отсутствует обязательный ключ: $required_pattern"
      fail_audit "[$label] в manifest отсутствует обязательный ключ: $required_pattern"
    fi
  done

  if [[ "$status" == "ok" ]]; then
    message="Последний manifest валиден и не старше порога"
    ok "[$label] $message"
  fi

  set_result "$prefix" STATUS "$status"
  set_result "$prefix" MESSAGE "$message"
  set_result "$prefix" FILE "$file"
  set_result "$prefix" AGE_HOURS "$age_hours"
  set_result "$prefix" MAX_AGE_HOURS "$max_age_hours"
}

render_json_report() {
  local created_at
  created_at="$(date +%F_%H-%M-%S)"

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$created_at")"
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "success": %s,\n' "$([[ $FAIL_COUNT -eq 0 ]] && echo true || echo false)"
    printf '  "verify_checksum": %s,\n' "$(json_bool "$VERIFY_CHECKSUM")"
    printf '  "summary": {\n'
    printf '    "ok": %s,\n' "$OK_COUNT"
    printf '    "warn": %s,\n' "$WARN_COUNT"
    printf '    "fail": %s\n' "$FAIL_COUNT"
    printf '  },\n'
    printf '  "thresholds": {\n'
    printf '    "db_max_age_hours": %s,\n' "$DB_MAX_AGE_HOURS"
    printf '    "files_max_age_hours": %s,\n' "$FILES_MAX_AGE_HOURS"
    printf '    "manifest_max_age_hours": %s\n' "$MANIFEST_MAX_AGE_HOURS"
    printf '  },\n'
    printf '  "db_backup": {\n'
    printf '    "status": "%s",\n' "$(json_escape "$DB_STATUS")"
    printf '    "message": %s,\n' "$(json_value_or_null "$DB_MESSAGE")"
    printf '    "file": %s,\n' "$(json_value_or_null "$DB_FILE")"
    printf '    "sidecar": %s,\n' "$(json_value_or_null "$DB_SIDECAR")"
    printf '    "age_hours": %s,\n' "$(json_number_or_null "$DB_AGE_HOURS")"
    printf '    "max_age_hours": %s,\n' "$(json_number_or_null "$DB_MAX_AGE_HOURS")"
    printf '    "checksum_verified": %s\n' "$(json_bool "$DB_CHECKSUM_VERIFIED")"
    printf '  },\n'
    printf '  "files_backup": {\n'
    printf '    "status": "%s",\n' "$(json_escape "$FILES_STATUS")"
    printf '    "message": %s,\n' "$(json_value_or_null "$FILES_MESSAGE")"
    printf '    "file": %s,\n' "$(json_value_or_null "$FILES_FILE")"
    printf '    "sidecar": %s,\n' "$(json_value_or_null "$FILES_SIDECAR")"
    printf '    "age_hours": %s,\n' "$(json_number_or_null "$FILES_AGE_HOURS")"
    printf '    "max_age_hours": %s,\n' "$(json_number_or_null "$FILES_MAX_AGE_HOURS")"
    printf '    "checksum_verified": %s\n' "$(json_bool "$FILES_CHECKSUM_VERIFIED")"
    printf '  },\n'
    printf '  "manifest_json": {\n'
    printf '    "status": "%s",\n' "$(json_escape "$MANIFEST_JSON_STATUS")"
    printf '    "message": %s,\n' "$(json_value_or_null "$MANIFEST_JSON_MESSAGE")"
    printf '    "file": %s,\n' "$(json_value_or_null "$MANIFEST_JSON_FILE")"
    printf '    "age_hours": %s,\n' "$(json_number_or_null "$MANIFEST_JSON_AGE_HOURS")"
    printf '    "max_age_hours": %s\n' "$(json_number_or_null "$MANIFEST_JSON_MAX_AGE_HOURS")"
    printf '  },\n'
    printf '  "manifest_txt": {\n'
    printf '    "status": "%s",\n' "$(json_escape "$MANIFEST_TXT_STATUS")"
    printf '    "message": %s,\n' "$(json_value_or_null "$MANIFEST_TXT_MESSAGE")"
    printf '    "file": %s,\n' "$(json_value_or_null "$MANIFEST_TXT_FILE")"
    printf '    "age_hours": %s,\n' "$(json_number_or_null "$MANIFEST_TXT_AGE_HOURS")"
    printf '    "max_age_hours": %s\n' "$(json_number_or_null "$MANIFEST_TXT_MAX_AGE_HOURS")"
    printf '  },\n'
    printf '  "findings": '
    json_findings
    printf '\n}\n'
  }
}

render_text_report() {
  cat <<EOF
Аудит бэкапов EspoCRM
Контур:            $ESPO_ENV
Compose-проект:    $COMPOSE_PROJECT_NAME
Env-файл:          $ENV_FILE
Проверка checksum: $([[ $VERIFY_CHECKSUM -eq 1 ]] && echo enabled || echo skipped)

Порог свежести:
  DB backup:       ${DB_MAX_AGE_HOURS}ч
  Files backup:    ${FILES_MAX_AGE_HOURS}ч
  Manifests:       ${MANIFEST_MAX_AGE_HOURS}ч

Сводка:
  ok:              $OK_COUNT
  warn:            $WARN_COUNT
  fail:            $FAIL_COUNT

Последний DB backup:
  status:          $DB_STATUS
  file:            ${DB_FILE:-n/a}
  sidecar:         ${DB_SIDECAR:-n/a}
  age_hours:       ${DB_AGE_HOURS:-n/a}
  message:         ${DB_MESSAGE:-n/a}

Последний Files backup:
  status:          $FILES_STATUS
  file:            ${FILES_FILE:-n/a}
  sidecar:         ${FILES_SIDECAR:-n/a}
  age_hours:       ${FILES_AGE_HOURS:-n/a}
  message:         ${FILES_MESSAGE:-n/a}

Последний Manifest JSON:
  status:          $MANIFEST_JSON_STATUS
  file:            ${MANIFEST_JSON_FILE:-n/a}
  age_hours:       ${MANIFEST_JSON_AGE_HOURS:-n/a}
  message:         ${MANIFEST_JSON_MESSAGE:-n/a}

Последний Manifest TXT:
  status:          $MANIFEST_TXT_STATUS
  file:            ${MANIFEST_TXT_FILE:-n/a}
  age_hours:       ${MANIFEST_TXT_AGE_HOURS:-n/a}
  message:         ${MANIFEST_TXT_MESSAGE:-n/a}
EOF
}

parse_contour_arg "$@"

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --json)
      JSON_MODE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-db)
      SKIP_DB=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-files)
      SKIP_FILES=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --max-db-age-hours)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --max-db-age-hours должно идти целое число"
      DB_MAX_AGE_OVERRIDE="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --max-files-age-hours)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --max-files-age-hours должно идти целое число"
      FILES_MAX_AGE_OVERRIDE="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --no-verify-checksum)
      VERIFY_CHECKSUM=0
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

DB_MAX_AGE_HOURS="${DB_MAX_AGE_OVERRIDE:-${BACKUP_MAX_DB_AGE_HOURS:-48}}"
FILES_MAX_AGE_HOURS="${FILES_MAX_AGE_OVERRIDE:-${BACKUP_MAX_FILES_AGE_HOURS:-48}}"
[[ "$DB_MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || die "DB max age должен быть целым числом часов"
[[ "$FILES_MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || die "Files max age должен быть целым числом часов"
if [[ $SKIP_DB -eq 0 && $SKIP_FILES -eq 0 ]]; then
  MANIFEST_MAX_AGE_HOURS="$(max_int "$DB_MAX_AGE_HOURS" "$FILES_MAX_AGE_HOURS")"
elif [[ $SKIP_DB -eq 0 ]]; then
  MANIFEST_MAX_AGE_HOURS="$DB_MAX_AGE_HOURS"
else
  MANIFEST_MAX_AGE_HOURS="$FILES_MAX_AGE_HOURS"
fi

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
DB_DIR="$BACKUP_ROOT_ABS/db"
FILES_DIR="$BACKUP_ROOT_ABS/files"
MANIFESTS_DIR="$BACKUP_ROOT_ABS/manifests"

if [[ $SKIP_DB -eq 0 ]]; then
  audit_backup_artifact "DB" "База данных" "$DB_DIR" '*.sql.gz' "$DB_MAX_AGE_HOURS"
else
  set_result "DB" STATUS "skipped"
  set_result "DB" MESSAGE "Проверка пропущена по флагу --skip-db"
  set_result "DB" FILE ""
  set_result "DB" SIDECAR ""
  set_result "DB" AGE_HOURS ""
  set_result "DB" MAX_AGE_HOURS "$DB_MAX_AGE_HOURS"
  set_result "DB" CHECKSUM_VERIFIED "0"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  audit_backup_artifact "FILES" "Файлы" "$FILES_DIR" '*.tar.gz' "$FILES_MAX_AGE_HOURS"
else
  set_result "FILES" STATUS "skipped"
  set_result "FILES" MESSAGE "Проверка пропущена по флагу --skip-files"
  set_result "FILES" FILE ""
  set_result "FILES" SIDECAR ""
  set_result "FILES" AGE_HOURS ""
  set_result "FILES" MAX_AGE_HOURS "$FILES_MAX_AGE_HOURS"
  set_result "FILES" CHECKSUM_VERIFIED "0"
fi

audit_manifest_artifact "MANIFEST_JSON" "Manifest JSON" "$MANIFESTS_DIR" '*.manifest.json' "$MANIFEST_MAX_AGE_HOURS" '"created_at"' '"contour"' '"compose_project"'
audit_manifest_artifact "MANIFEST_TXT" "Manifest TXT" "$MANIFESTS_DIR" '*.manifest.txt' "$MANIFEST_MAX_AGE_HOURS" '^created_at=' '^contour=' '^compose_project='

if [[ $JSON_MODE -eq 0 ]]; then
  print_context
  render_text_report
else
  render_json_report
fi

if [[ $FAIL_COUNT -ne 0 ]]; then
  exit 1
fi
