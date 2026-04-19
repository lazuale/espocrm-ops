#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/migrate-backup.sh <from> <to> [--force] [--confirm-prod prod] [--db-backup PATH] [--files-backup PATH] [--skip-db] [--skip-files] [--no-start]

Examples:
  ./scripts/migrate-backup.sh prod dev --force
  ./scripts/migrate-backup.sh dev prod --force --confirm-prod prod
  ./scripts/migrate-backup.sh dev prod --force --confirm-prod prod --db-backup /opt/espocrm-data/backups/dev/db/espocrm-dev_2026-04-03_10-00-00.sql.gz --files-backup /opt/espocrm-data/backups/dev/files/espocrm-dev_files_2026-04-03_10-00-00.tar.gz

This compatibility wrapper delegates immediately to:
  espops migrate-backup --from <dev|prod> --to <dev|prod> --project-dir <repo> --compose-file <repo>/compose.yaml [args...]
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

args=(
  migrate-backup
  --from "$SOURCE_CONTOUR"
  --to "$TARGET_CONTOUR"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

args+=("$@")

run_espops "${args[@]}"
