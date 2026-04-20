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

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

args=(migrate --project-dir "$ROOT_DIR")

if [[ $# -gt 0 && "$1" != -* ]]; then
  args+=(--from "$1")
  shift
fi
if [[ $# -gt 0 && "$1" != -* ]]; then
  args+=(--to "$1")
  shift
fi

args+=("$@")
run_espops "${args[@]}"
