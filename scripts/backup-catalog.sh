#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт строит каталог backup-наборов контура и помогает быстро понять:
# - какие backup-наборы вообще есть;
# - насколько они полны для restore;
# - есть ли sidecar-файлы и совпадают ли checksum;
# - какой набор можно безопасно брать для восстановления или миграции.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/backup-catalog.sh [dev|prod] [--json] [--limit N] [--latest-only] [--ready-only] [--verify-checksum]

Примеры:
  ./scripts/backup-catalog.sh prod
  ./scripts/backup-catalog.sh dev --json
  ./scripts/backup-catalog.sh prod --latest-only
  ./scripts/backup-catalog.sh dev --limit 10 --verify-checksum
  ./scripts/backup-catalog.sh prod --ready-only
EOF
}

JSON_MODE=0
VERIFY_CHECKSUM=0
LATEST_ONLY=0
READY_ONLY=0
LIMIT=""

declare -A GROUP_SEEN=()
declare -A GROUP_PREFIX=()
declare -A GROUP_STAMP=()
declare -A DB_FILE_MAP=()
declare -A DB_SIDECAR_MAP=()
declare -A DB_AGE_MAP=()
declare -A DB_SIZE_MAP=()
declare -A DB_CHECKSUM_STATUS=()
declare -A FILES_FILE_MAP=()
declare -A FILES_SIDECAR_MAP=()
declare -A FILES_AGE_MAP=()
declare -A FILES_SIZE_MAP=()
declare -A FILES_CHECKSUM_STATUS=()
declare -A MANIFEST_TXT_MAP=()
declare -A MANIFEST_TXT_AGE_MAP=()
declare -A MANIFEST_JSON_MAP=()
declare -A MANIFEST_JSON_AGE_MAP=()
declare -a GROUP_KEYS=()

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

age_hours_for_file() {
  local file="$1"
  local now mtime

  now="$(date +%s)"
  mtime="$(file_mtime_epoch "$file")"
  echo $(((now - mtime) / 3600))
}

register_group() {
  local key="$1"
  local prefix="$2"
  local stamp="$3"

  if [[ -z "${GROUP_SEEN[$key]:-}" ]]; then
    GROUP_SEEN["$key"]=1
    GROUP_KEYS+=("$key")
  fi

  GROUP_PREFIX["$key"]="$prefix"
  GROUP_STAMP["$key"]="$stamp"
}

parse_db_group() {
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

parse_files_group() {
  local file="$1"
  local base prefix stamp

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_files_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.tar\.gz$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    printf '%s|%s\n' "$prefix" "$stamp"
    return 0
  fi

  return 1
}

parse_manifest_group() {
  local file="$1"
  local base prefix stamp kind

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.manifest\.(txt|json)$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    kind="${BASH_REMATCH[3]}"
    printf '%s|%s|%s\n' "$prefix" "$stamp" "$kind"
    return 0
  fi

  return 1
}

checksum_status_for_file() {
  local file="$1"
  local sidecar="${file}.sha256"

  if [[ ! -f "$sidecar" ]]; then
    printf 'sidecar_missing\n'
    return 0
  fi

  if [[ $VERIFY_CHECKSUM -eq 0 ]]; then
    printf 'sidecar_present\n'
    return 0
  fi

  if verify_sha256_sidecar "$file" "$sidecar"; then
    printf 'verified\n'
  else
    printf 'mismatch\n'
  fi
}

collect_db_backups() {
  local file key prefix stamp

  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    if IFS='|' read -r prefix stamp < <(parse_db_group "$file"); then
      key="${prefix}|${stamp}"
      register_group "$key" "$prefix" "$stamp"
      DB_FILE_MAP["$key"]="$file"
      DB_AGE_MAP["$key"]="$(age_hours_for_file "$file")"
      DB_SIZE_MAP["$key"]="$(file_size_bytes "$file")"
      if [[ -f "${file}.sha256" ]]; then
        DB_SIDECAR_MAP["$key"]="${file}.sha256"
      fi
      DB_CHECKSUM_STATUS["$key"]="$(checksum_status_for_file "$file")"
    fi
  done < <(find "$BACKUP_ROOT_ABS/db" -maxdepth 1 -type f -name '*.sql.gz' | sort)
}

collect_files_backups() {
  local file key prefix stamp

  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    if IFS='|' read -r prefix stamp < <(parse_files_group "$file"); then
      key="${prefix}|${stamp}"
      register_group "$key" "$prefix" "$stamp"
      FILES_FILE_MAP["$key"]="$file"
      FILES_AGE_MAP["$key"]="$(age_hours_for_file "$file")"
      FILES_SIZE_MAP["$key"]="$(file_size_bytes "$file")"
      if [[ -f "${file}.sha256" ]]; then
        FILES_SIDECAR_MAP["$key"]="${file}.sha256"
      fi
      FILES_CHECKSUM_STATUS["$key"]="$(checksum_status_for_file "$file")"
    fi
  done < <(find "$BACKUP_ROOT_ABS/files" -maxdepth 1 -type f -name '*.tar.gz' | sort)
}

collect_manifests() {
  local file key prefix stamp kind

  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    if IFS='|' read -r prefix stamp kind < <(parse_manifest_group "$file"); then
      key="${prefix}|${stamp}"
      register_group "$key" "$prefix" "$stamp"
      case "$kind" in
        txt)
          MANIFEST_TXT_MAP["$key"]="$file"
          MANIFEST_TXT_AGE_MAP["$key"]="$(age_hours_for_file "$file")"
          ;;
        json)
          MANIFEST_JSON_MAP["$key"]="$file"
          MANIFEST_JSON_AGE_MAP["$key"]="$(age_hours_for_file "$file")"
          ;;
      esac
    fi
  done < <(find "$BACKUP_ROOT_ABS/manifests" -maxdepth 1 -type f \( -name '*.manifest.txt' -o -name '*.manifest.json' \) | sort)
}

restore_readiness_for_group() {
  local key="$1"

  if [[ "${DB_CHECKSUM_STATUS[$key]:-}" == "mismatch" || "${FILES_CHECKSUM_STATUS[$key]:-}" == "mismatch" ]]; then
    printf 'corrupted\n'
    return
  fi

  if [[ -n "${DB_FILE_MAP[$key]:-}" \
    && -n "${DB_SIDECAR_MAP[$key]:-}" \
    && -n "${FILES_FILE_MAP[$key]:-}" \
    && -n "${FILES_SIDECAR_MAP[$key]:-}" \
    && -n "${MANIFEST_TXT_MAP[$key]:-}" \
    && -n "${MANIFEST_JSON_MAP[$key]:-}" ]]; then
    if [[ $VERIFY_CHECKSUM -eq 1 ]]; then
      printf 'ready_verified\n'
    else
      printf 'ready_unverified\n'
    fi
  else
    printf 'incomplete\n'
  fi
}

group_is_complete() {
  local key="$1"
  [[ "$(restore_readiness_for_group "$key")" == ready_* ]]
}

sorted_group_keys() {
  if [[ ${#GROUP_KEYS[@]} -eq 0 ]]; then
    return 0
  fi

  printf '%s\n' "${GROUP_KEYS[@]}" | sort -t '|' -k2,2r -k1,1r
}

text_line_for_artifact() {
  local label="$1"
  local file="$2"
  local sidecar="$3"
  local age="$4"
  local size="$5"
  local checksum_status="$6"

  if [[ -z "$file" ]]; then
    printf '  %s: missing\n' "$label"
    return
  fi

  printf '  %s: %s\n' "$label" "$file"
  printf '    age_hours=%s size_bytes=%s\n' "${age:-n/a}" "${size:-n/a}"
  printf '    sidecar=%s checksum=%s\n' "${sidecar:-missing}" "${checksum_status:-n/a}"
}

text_line_for_manifest() {
  local label="$1"
  local file="$2"
  local age="$3"

  if [[ -z "$file" ]]; then
    printf '  %s: missing\n' "$label"
    return
  fi

  printf '  %s: %s\n' "$label" "$file"
  printf '    age_hours=%s\n' "${age:-n/a}"
}

render_text_report() {
  local total="$1"
  local shown="$2"
  local key readiness
  local index=0

  cat <<EOF
Каталог backup-наборов EspoCRM
Контур:              $ESPO_ENV
Compose-проект:      $COMPOSE_PROJECT_NAME
Env-файл:            $ENV_FILE
Каталог backup:      $BACKUP_ROOT_ABS
Проверка checksum:   $([[ $VERIFY_CHECKSUM -eq 1 ]] && echo enabled || echo skipped)
Фильтр ready-only:   $([[ $READY_ONLY -eq 1 ]] && echo enabled || echo disabled)
Лимит:               ${LIMIT:-all}
Всего наборов:       $total
Показано наборов:    $shown
EOF

  if [[ $shown -eq 0 ]]; then
    echo
    echo "Backup-наборы не найдены по текущим условиям выборки"
    return
  fi

  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    readiness="$(restore_readiness_for_group "$key")"
    if [[ $READY_ONLY -eq 1 && "$readiness" != ready_* ]]; then
      continue
    fi

    index=$((index + 1))
    if [[ -n "$LIMIT" && $index -gt $LIMIT ]]; then
      break
    fi

    echo
    echo "[$index] ${GROUP_STAMP[$key]} | prefix=${GROUP_PREFIX[$key]} | readiness=$readiness"
    text_line_for_artifact "DB" "${DB_FILE_MAP[$key]:-}" "${DB_SIDECAR_MAP[$key]:-}" "${DB_AGE_MAP[$key]:-}" "${DB_SIZE_MAP[$key]:-}" "${DB_CHECKSUM_STATUS[$key]:-not_applicable}"
    text_line_for_artifact "Files" "${FILES_FILE_MAP[$key]:-}" "${FILES_SIDECAR_MAP[$key]:-}" "${FILES_AGE_MAP[$key]:-}" "${FILES_SIZE_MAP[$key]:-}" "${FILES_CHECKSUM_STATUS[$key]:-not_applicable}"
    text_line_for_manifest "Manifest TXT" "${MANIFEST_TXT_MAP[$key]:-}" "${MANIFEST_TXT_AGE_MAP[$key]:-}"
    text_line_for_manifest "Manifest JSON" "${MANIFEST_JSON_MAP[$key]:-}" "${MANIFEST_JSON_AGE_MAP[$key]:-}"
  done < <(sorted_group_keys)
}

render_json_report() {
  local created_at key readiness first=1 emitted=0 total="$1"
  created_at="$(date +%F_%H-%M-%S)"

  {
    printf '{\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$created_at")"
    printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
    printf '  "compose_project": "%s",\n' "$(json_escape "$COMPOSE_PROJECT_NAME")"
    printf '  "env_file": "%s",\n' "$(json_escape "$ENV_FILE")"
    printf '  "backup_root": "%s",\n' "$(json_escape "$BACKUP_ROOT_ABS")"
    printf '  "verify_checksum": %s,\n' "$(json_bool "$VERIFY_CHECKSUM")"
    printf '  "ready_only": %s,\n' "$(json_bool "$READY_ONLY")"
    printf '  "limit": %s,\n' "$(json_number_or_null "$LIMIT")"
    printf '  "total_sets": %s,\n' "$total"
    printf '  "sets": ['

    while IFS= read -r key; do
      [[ -n "$key" ]] || continue
      readiness="$(restore_readiness_for_group "$key")"
      if [[ $READY_ONLY -eq 1 && "$readiness" != ready_* ]]; then
        continue
      fi

      emitted=$((emitted + 1))
      if [[ -n "$LIMIT" && $emitted -gt $LIMIT ]]; then
        break
      fi

      if [[ $first -eq 0 ]]; then
        printf ','
      fi
      printf '\n    {\n'
      printf '      "prefix": "%s",\n' "$(json_escape "${GROUP_PREFIX[$key]}")"
      printf '      "stamp": "%s",\n' "$(json_escape "${GROUP_STAMP[$key]}")"
      printf '      "group_key": "%s",\n' "$(json_escape "$key")"
      printf '      "restore_readiness": "%s",\n' "$(json_escape "$readiness")"
      printf '      "is_ready": %s,\n' "$(json_bool "$([[ "$readiness" == ready_* ]] && echo 1 || echo 0)")"
      printf '      "db": {\n'
      printf '        "file": %s,\n' "$(json_value_or_null "${DB_FILE_MAP[$key]:-}")"
      printf '        "sidecar": %s,\n' "$(json_value_or_null "${DB_SIDECAR_MAP[$key]:-}")"
      printf '        "age_hours": %s,\n' "$(json_number_or_null "${DB_AGE_MAP[$key]:-}")"
      printf '        "size_bytes": %s,\n' "$(json_number_or_null "${DB_SIZE_MAP[$key]:-}")"
      printf '        "checksum_status": "%s"\n' "$(json_escape "${DB_CHECKSUM_STATUS[$key]:-not_applicable}")"
      printf '      },\n'
      printf '      "files": {\n'
      printf '        "file": %s,\n' "$(json_value_or_null "${FILES_FILE_MAP[$key]:-}")"
      printf '        "sidecar": %s,\n' "$(json_value_or_null "${FILES_SIDECAR_MAP[$key]:-}")"
      printf '        "age_hours": %s,\n' "$(json_number_or_null "${FILES_AGE_MAP[$key]:-}")"
      printf '        "size_bytes": %s,\n' "$(json_number_or_null "${FILES_SIZE_MAP[$key]:-}")"
      printf '        "checksum_status": "%s"\n' "$(json_escape "${FILES_CHECKSUM_STATUS[$key]:-not_applicable}")"
      printf '      },\n'
      printf '      "manifest_txt": {\n'
      printf '        "file": %s,\n' "$(json_value_or_null "${MANIFEST_TXT_MAP[$key]:-}")"
      printf '        "age_hours": %s\n' "$(json_number_or_null "${MANIFEST_TXT_AGE_MAP[$key]:-}")"
      printf '      },\n'
      printf '      "manifest_json": {\n'
      printf '        "file": %s,\n' "$(json_value_or_null "${MANIFEST_JSON_MAP[$key]:-}")"
      printf '        "age_hours": %s\n' "$(json_number_or_null "${MANIFEST_JSON_AGE_MAP[$key]:-}")"
      printf '      }\n'
      printf '    }'
      first=0
    done < <(sorted_group_keys)

    if [[ $emitted -gt 0 ]]; then
      printf '\n  '
    fi
    printf ']\n'
    printf '}\n'
  }
}

parse_contour_arg "$@"

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --json)
      JSON_MODE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --verify-checksum)
      VERIFY_CHECKSUM=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --latest-only)
      LATEST_ONLY=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --ready-only)
      READY_ONLY=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --limit)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --limit должно идти целое число"
      LIMIT="${POSITIONAL_ARGS[1]}"
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

if [[ -n "$LIMIT" && ! "$LIMIT" =~ ^[0-9]+$ ]]; then
  die "Параметр --limit должен быть целым числом"
fi

if [[ $LATEST_ONLY -eq 1 ]]; then
  LIMIT=1
fi

resolve_env_file
load_env
ensure_runtime_dirs

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

collect_db_backups
collect_files_backups
collect_manifests

TOTAL_SET_COUNT="${#GROUP_KEYS[@]}"

if [[ $JSON_MODE -eq 0 ]]; then
  local_shown=0
  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    if [[ $READY_ONLY -eq 1 && "$(restore_readiness_for_group "$key")" != ready_* ]]; then
      continue
    fi
    local_shown=$((local_shown + 1))
    if [[ -n "$LIMIT" && $local_shown -gt $LIMIT ]]; then
      local_shown="$LIMIT"
      break
    fi
  done < <(sorted_group_keys)
  render_text_report "$TOTAL_SET_COUNT" "$local_shown"
else
  render_json_report "$TOTAL_SET_COUNT"
fi
