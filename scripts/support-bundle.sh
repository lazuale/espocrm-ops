#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/support-bundle.sh <dev|prod> [--tail N] [--output PATH]

Examples:
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

redact_yaml_secret_values() {
  awk '
    {
      if (match($0, /^([[:space:]]*)([A-Za-z_][A-Za-z0-9_]*):[[:space:]]*(.*)$/, parts)) {
        if (parts[2] ~ /(PASSWORD|TOKEN|SECRET)$/) {
          print parts[1] parts[2] ": <redacted>"
          next
        }
      }
      print
    }
  '
}

copy_if_exists() {
  local src="$1"
  local dst="$2"

  if [[ -f "$src" ]]; then
    cp "$src" "$dst"
  fi
}

declare -a BUNDLE_TASK_PIDS=()
declare -a BUNDLE_TASK_LABELS=()
declare -a BUNDLE_TASK_ALLOW_FAIL=()

start_bundle_capture() {
  local label="$1"
  local output_file="$2"
  local allow_fail="$3"
  shift 3

  (
    set +e
    "$@" > "$output_file" 2>&1
    status=$?
    exit "$status"
  ) &

  BUNDLE_TASK_PIDS+=("$!")
  BUNDLE_TASK_LABELS+=("$label")
  BUNDLE_TASK_ALLOW_FAIL+=("$allow_fail")
}

wait_bundle_tasks() {
  local index label allow_fail

  for index in "${!BUNDLE_TASK_PIDS[@]}"; do
    label="${BUNDLE_TASK_LABELS[$index]}"
    allow_fail="${BUNDLE_TASK_ALLOW_FAIL[$index]}"

    if wait "${BUNDLE_TASK_PIDS[$index]}"; then
      continue
    fi

    if [[ "$allow_fail" == "1" ]]; then
      warn "Artifact collection for '$label' failed, continuing"
    else
      die "Could not collect a required support-bundle artifact: $label"
    fi
  done
}

parse_contour_arg "$@"
TAIL_LINES=300
OUTPUT_PATH=""

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --tail)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--tail must be followed by a line count"
      TAIL_LINES="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--output must be followed by a path"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

[[ "$TAIL_LINES" =~ ^[0-9]+$ ]] || die "--tail must be an integer"

require_explicit_contour

acquire_operation_lock support-bundle
resolve_env_file
load_env
ensure_runtime_dirs
require_compose --skip-daemon-check

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
SUPPORT_RETENTION="${SUPPORT_RETENTION_DAYS:-14}"
LATEST_MANIFEST_JSON="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.json' || true)"
LATEST_MANIFEST_TXT="$(latest_backup_file "$BACKUP_ROOT_ABS/manifests" '*.manifest.txt' || true)"

if [[ -z "$OUTPUT_PATH" ]]; then
  STAMP="$(next_unique_stamp "$SUPPORT_DIR/${NAME_PREFIX}_support___STAMP__.tar.gz")"
  OUTPUT_PATH="$SUPPORT_DIR/${NAME_PREFIX}_support_${STAMP}.tar.gz"
fi

TMP_DIR="$(mktemp -d "$ROOT_DIR/.support.${ESPO_ENV}.XXXXXX")"
# shellcheck disable=SC2016
append_trap 'rm -rf -- "$TMP_DIR"' EXIT

print_context
echo "Building support bundle: $OUTPUT_PATH"

redact_env_file "$ENV_FILE" "$TMP_DIR/env.redacted"
compose config | redact_yaml_secret_values > "$TMP_DIR/compose.config.yaml"
compose ps > "$TMP_DIR/compose.ps.txt" 2>&1 || true
compose logs --no-color --tail "$TAIL_LINES" > "$TMP_DIR/compose.logs.txt" 2>&1 || true
docker version > "$TMP_DIR/docker.version.txt" 2>&1 || true
docker compose version > "$TMP_DIR/docker.compose.version.txt" 2>&1 || true
start_bundle_capture "doctor.txt" "$TMP_DIR/doctor.txt" 1 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV"
start_bundle_capture "doctor.json" "$TMP_DIR/doctor.json" 1 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV" --json
start_bundle_capture "status-report.txt" "$TMP_DIR/status-report.txt" 0 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV"
start_bundle_capture "status-report.json" "$TMP_DIR/status-report.json" 0 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json
start_bundle_capture "backup-catalog.txt" "$TMP_DIR/backup-catalog.txt" 1 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV"
start_bundle_capture "backup-catalog.json" "$TMP_DIR/backup-catalog.json" 1 env ENV_FILE="$ENV_FILE" bash "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV" --json
wait_bundle_tasks

copy_if_exists "$LATEST_MANIFEST_JSON" "$TMP_DIR/latest.manifest.json"
copy_if_exists "$LATEST_MANIFEST_TXT" "$TMP_DIR/latest.manifest.txt"

tar -C "$(dirname "$TMP_DIR")" -czf "$OUTPUT_PATH" "$(basename "$TMP_DIR")"
cleanup_old_files "$SUPPORT_DIR" "$SUPPORT_RETENTION" '*.tar.gz'
echo "Support bundle created: $OUTPUT_PATH"
