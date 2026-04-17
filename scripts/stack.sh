#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/stack.sh <dev|prod> <docker-compose-arguments...>

Examples:
  ./scripts/stack.sh prod up -d
  ./scripts/stack.sh dev ps
  ./scripts/stack.sh prod logs -f espocrm
EOF
}

parse_contour_arg "$@"
if [[ ${#POSITIONAL_ARGS[@]} -eq 0 ]]; then
  usage >&2
  exit 1
fi

case "${POSITIONAL_ARGS[0]}" in
  -h|--help)
    if [[ ${#POSITIONAL_ARGS[@]} -eq 1 ]]; then
      usage
      exit 0
    fi
    usage >&2
    die "Unknown argument: ${POSITIONAL_ARGS[1]}"
    ;;
esac

require_explicit_contour

case "${POSITIONAL_ARGS[0]}" in
  up|down|start|stop|restart|pull|build|create|rm|kill|pause|unpause)
    acquire_operation_lock "stack:${POSITIONAL_ARGS[0]}"
    ;;
esac

resolve_env_file
load_env

case "${POSITIONAL_ARGS[0]}" in
  config|version)
    require_compose --skip-daemon-check
    ;;
  *)
    require_compose
    ;;
esac

compose "${POSITIONAL_ARGS[@]}"
