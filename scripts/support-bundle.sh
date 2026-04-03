#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт собирает support bundle для разбора инцидентов:
# - redacted env;
# - compose config;
# - статусы сервисов;
# - doctor-отчет;
# - статус-отчет;
# - каталог backup-наборов;
# - tail логов;
# - последние manifest-файлы.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/support-bundle.sh [dev|prod] [--tail N] [--output PATH]

Примеры:
  ./scripts/support-bundle.sh prod
  ./scripts/support-bundle.sh dev --tail 500
  ./scripts/support-bundle.sh prod --output /tmp/espo-support.tar.gz
EOF
}

redact_env_file() {
  local src="$1"
  local dst="$2"

  awk '
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { print; next }
    /^[A-Za-z_][A-Za-z0-9_]*=/ {
      split($0, parts, "=")
      key=parts[1]
      value=substr($0, length(key) + 2)
      if (key ~ /(PASSWORD|TOKEN|SECRET)$/) {
        print key "=<redacted>"
      } else {
        print
      }
      next
    }
    { print }
  ' "$src" > "$dst"
}

copy_if_exists() {
  local src="$1"
  local dst="$2"

  if [[ -f "$src" ]]; then
    cp "$src" "$dst"
  fi
}

parse_contour_arg "$@"
TAIL_LINES=300
OUTPUT_PATH=""

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --tail)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --tail должно идти число строк"
      TAIL_LINES="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "После --output должен идти путь"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
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

[[ "$TAIL_LINES" =~ ^[0-9]+$ ]] || die "Значение --tail должно быть целым числом"

resolve_env_file
load_env
ensure_runtime_dirs
require_compose

STAMP="$(date +%F_%H-%M-%S)"
BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
SUPPORT_RETENTION="${SUPPORT_RETENTION_DAYS:-14}"
LATEST_MANIFEST_JSON="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.json' || true)"
LATEST_MANIFEST_TXT="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.txt' || true)"

if [[ -z "$OUTPUT_PATH" ]]; then
  OUTPUT_PATH="$SUPPORT_DIR/${NAME_PREFIX}_support_${STAMP}.tar.gz"
fi

TMP_DIR="$(mktemp -d "$ROOT_DIR/.support.${ESPO_ENV}.XXXXXX")"
trap 'rm -rf -- "$TMP_DIR"' EXIT

print_context
echo "Сборка support bundle: $OUTPUT_PATH"

redact_env_file "$ENV_FILE" "$TMP_DIR/env.redacted"
compose config > "$TMP_DIR/compose.config.yaml"
compose ps > "$TMP_DIR/compose.ps.txt"
compose logs --no-color --tail "$TAIL_LINES" > "$TMP_DIR/compose.logs.txt" 2>&1 || true
docker version > "$TMP_DIR/docker.version.txt" 2>&1 || true
docker compose version > "$TMP_DIR/docker.compose.version.txt" 2>&1 || true
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV" > "$TMP_DIR/doctor.txt" 2>&1 || true
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV" --json > "$TMP_DIR/doctor.json" 2>&1 || true
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" > "$TMP_DIR/status-report.txt"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json > "$TMP_DIR/status-report.json"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV" > "$TMP_DIR/backup-catalog.txt" 2>&1 || true
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV" --json > "$TMP_DIR/backup-catalog.json" 2>&1 || true

copy_if_exists "$LATEST_MANIFEST_JSON" "$TMP_DIR/latest.manifest.json"
copy_if_exists "$LATEST_MANIFEST_TXT" "$TMP_DIR/latest.manifest.txt"

tar -C "$(dirname "$TMP_DIR")" -czf "$OUTPUT_PATH" "$(basename "$TMP_DIR")"
cleanup_old_files "$SUPPORT_DIR" "$SUPPORT_RETENTION" '*.tar.gz'
echo "Support bundle создан: $OUTPUT_PATH"
