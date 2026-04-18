#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/backup.sh <dev|prod> [--skip-db] [--skip-files] [--no-stop]

Examples:
  ./scripts/backup.sh prod
  ./scripts/backup.sh dev
  ./scripts/backup.sh prod --skip-files
  ./scripts/backup.sh dev --skip-db
  ./scripts/backup.sh prod --no-stop
EOF
}

RAW_ARGS=("$@")
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
      die "Unknown argument: $arg"
      ;;
  esac
done

require_explicit_contour

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Nothing to back up: --skip-db and --skip-files cannot both be set"
fi

if [[ "${ESPO_SHELL_EXEC_CONTEXT:-0}" != "1" ]]; then
  preflight_args=(
    run-operation
    --scope "$ESPO_ENV"
    --operation backup
    --project-dir "$ROOT_DIR"
  )

  if [[ -n "${ENV_FILE:-}" ]]; then
    preflight_args+=(--env-file "$ENV_FILE")
  fi

  preflight_args+=(-- bash "$0" "${RAW_ARGS[@]}")
  run_espops "${preflight_args[@]}"
  exit $?
fi

print_context

args=(
  backup-exec
  --scope "$ESPO_ENV"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
  --env-file "$ENV_FILE"
)

if [[ $SKIP_DB -eq 1 ]]; then
  args+=(--skip-db)
fi

if [[ $SKIP_FILES -eq 1 ]]; then
  args+=(--skip-files)
fi

if [[ $NO_STOP -eq 1 ]]; then
  args+=(--no-stop)
fi

run_espops "${args[@]}"
