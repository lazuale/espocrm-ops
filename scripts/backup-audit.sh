#!/usr/bin/env bash
set -Eeuo pipefail

# Compatibility wrapper: shell owns contour/env/lock handling, Go owns audit semantics.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/backup-audit.sh <dev|prod> [--json] [--skip-db] [--skip-files] [--max-db-age-hours N] [--max-files-age-hours N] [--no-verify-checksum]

Examples:
  ./scripts/backup-audit.sh prod
  ./scripts/backup-audit.sh dev --json
  ./scripts/backup-audit.sh prod --max-db-age-hours 24 --max-files-age-hours 24
  ./scripts/backup-audit.sh dev --skip-files
EOF
}

json_details_success() {
  python3 -c 'import json, sys
data = json.load(sys.stdin)
sys.exit(0 if data.get("details", {}).get("success") else 1)'
}

json_output_is_valid() {
  python3 -c 'import json, sys
json.load(sys.stdin)'
}

JSON_MODE=0
SKIP_DB=0
SKIP_FILES=0
VERIFY_CHECKSUM=1
DB_MAX_AGE_OVERRIDE=""
FILES_MAX_AGE_OVERRIDE=""

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
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--max-db-age-hours must be followed by an integer"
      DB_MAX_AGE_OVERRIDE="${POSITIONAL_ARGS[1]}"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --max-files-age-hours)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--max-files-age-hours must be followed by an integer"
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
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

require_explicit_contour

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Nothing to check: --skip-db and --skip-files cannot both be set"
fi

acquire_operation_lock backup-audit
resolve_env_file
load_env
ensure_runtime_dirs

DB_MAX_AGE_HOURS="${DB_MAX_AGE_OVERRIDE:-${BACKUP_MAX_DB_AGE_HOURS:-48}}"
FILES_MAX_AGE_HOURS="${FILES_MAX_AGE_OVERRIDE:-${BACKUP_MAX_FILES_AGE_HOURS:-48}}"
[[ "$DB_MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || die "The maximum database backup age must be an integer number of hours"
[[ "$FILES_MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || die "The maximum files backup age must be an integer number of hours"

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

args=()
if [[ $JSON_MODE -eq 1 ]]; then
  args+=(--json)
fi

args+=(
  backup-audit
  --backup-root "$BACKUP_ROOT_ABS"
  --max-db-age-hours "$DB_MAX_AGE_HOURS"
  --max-files-age-hours "$FILES_MAX_AGE_HOURS"
)

if [[ $SKIP_DB -eq 1 ]]; then
  args+=(--skip-db)
fi
if [[ $SKIP_FILES -eq 1 ]]; then
  args+=(--skip-files)
fi
if [[ $VERIFY_CHECKSUM -eq 0 ]]; then
  args+=(--no-verify-checksum)
fi

if [[ $JSON_MODE -eq 1 ]]; then
  set +e
  output="$(run_espops "${args[@]}")"
  status=$?
  set -e

  printf '%s\n' "$output"

  if [[ $status -ne 0 ]]; then
    exit "$status"
  fi

  printf '%s\n' "$output" | json_output_is_valid >/dev/null
  if ! printf '%s\n' "$output" | json_details_success; then
    exit 1
  fi
  exit 0
fi

set +e
output="$(run_espops "${args[@]}" 2>&1)"
status=$?
set -e

printf '%s\n' "$output"

if [[ $status -ne 0 ]]; then
  exit "$status"
fi

set +e
audit_json="$(run_espops --json "${args[@]}")"
audit_status=$?
set -e

if [[ $audit_status -ne 0 ]]; then
  exit "$audit_status"
fi

printf '%s\n' "$audit_json" | json_output_is_valid >/dev/null
if ! printf '%s\n' "$audit_json" | json_details_success; then
  exit 1
fi
