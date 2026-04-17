#!/usr/bin/env bash
set -Eeuo pipefail

# Compatibility wrapper: operator UX and env/lock handling stay in shell,
# backup-set catalog semantics live in Go.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/backup-catalog.sh <dev|prod> [--json] [--limit N] [--latest-only] [--ready-only] [--verify-checksum]

Examples:
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
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--limit must be followed by an integer"
      LIMIT="${POSITIONAL_ARGS[1]}"
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

require_explicit_contour

if [[ -n "$LIMIT" && ! "$LIMIT" =~ ^[0-9]+$ ]]; then
  die "--limit must be an integer"
fi

acquire_operation_lock backup-catalog
resolve_env_file
load_env
ensure_runtime_dirs

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

args=()
if [[ $JSON_MODE -eq 1 ]]; then
  args+=(--json)
fi

args+=(backup-catalog --backup-root "$BACKUP_ROOT_ABS")

if [[ $VERIFY_CHECKSUM -eq 1 ]]; then
  args+=(--verify-checksum)
fi
if [[ $READY_ONLY -eq 1 ]]; then
  args+=(--ready-only)
fi
if [[ $LATEST_ONLY -eq 1 ]]; then
  args+=(--latest-only)
elif [[ -n "$LIMIT" ]]; then
  args+=(--limit "$LIMIT")
fi

run_espops "${args[@]}"
