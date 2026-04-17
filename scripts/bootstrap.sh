#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/bootstrap.sh <dev|prod>

Prepare data and backup directories for the selected contour.
EOF
}

parse_contour_arg "$@"
case "${POSITIONAL_ARGS[0]:-}" in
  "")
    ;;
  -h|--help)
    if [[ ${#POSITIONAL_ARGS[@]} -eq 1 ]]; then
      usage
      exit 0
    fi
    usage >&2
    die "Unknown argument: ${POSITIONAL_ARGS[1]}"
    ;;
  *)
    usage >&2
    die "Unknown argument: ${POSITIONAL_ARGS[0]}"
    ;;
esac

require_explicit_contour

resolve_env_file
load_env
ensure_runtime_dirs

print_context
echo "Directories prepared:"
echo "  Database: $(root_path "$DB_STORAGE_DIR")"
echo "  Application files: $(root_path "$ESPO_STORAGE_DIR")"
echo "  Backups: $(root_path "$BACKUP_ROOT")"
