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

parse_contour_arg "$@"

for arg in "${POSITIONAL_ARGS[@]}"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
  esac
done

require_explicit_contour

args=(
  restore
  --scope "$ESPO_ENV"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

if [[ -n "${ENV_FILE:-}" ]]; then
  args+=(--env-file "$ENV_FILE")
fi

args+=("${POSITIONAL_ARGS[@]}")
run_espops "${args[@]}"
