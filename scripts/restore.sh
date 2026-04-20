#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/restore.sh <dev|prod> [restore flags...]

Examples:
  ./scripts/restore.sh dev --manifest /path/to/manifest.json --force
  ./scripts/restore.sh prod --manifest /path/to/manifest.json --force --confirm-prod prod
  ./scripts/restore.sh dev --files-backup /path/to/files.tar.gz --skip-db --force
EOF
}

args=(restore --project-dir "$ROOT_DIR")

if [[ $# -gt 0 && "$1" != -* ]]; then
  args+=(--scope "$1")
  shift
fi

if [[ -n "${ENV_FILE:-}" ]]; then
  args+=(--env-file "$ENV_FILE")
fi

args+=("$@")
run_espops "${args[@]}"
