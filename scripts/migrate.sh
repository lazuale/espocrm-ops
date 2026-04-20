#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/migrate.sh <from> <to> [migrate flags...]

Examples:
  ./scripts/migrate.sh dev prod --force --confirm-prod prod
  ./scripts/migrate.sh prod dev --force --db-backup /path/to/db.sql.gz --files-backup /path/to/files.tar.gz
EOF
}

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 1
fi

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

if [[ $# -lt 2 ]]; then
  usage >&2
  exit 1
fi

SOURCE_CONTOUR="$1"
TARGET_CONTOUR="$2"
shift 2

case "$SOURCE_CONTOUR" in
  dev|prod)
    ;;
  *)
    usage_error "Source contour must be dev or prod"
    ;;
esac

case "$TARGET_CONTOUR" in
  dev|prod)
    ;;
  *)
    usage_error "Target contour must be dev or prod"
    ;;
esac

args=(
  migrate
  --from "$SOURCE_CONTOUR"
  --to "$TARGET_CONTOUR"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

args+=("$@")
run_espops "${args[@]}"
