#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/restore-drill.sh <dev|prod> [args...]

Examples:
  ./scripts/restore-drill.sh prod
  ./scripts/restore-drill.sh dev --timeout 900
  ./scripts/restore-drill.sh prod --db-backup /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/restore-drill.sh dev --files-backup /opt/espocrm-data/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
  ./scripts/restore-drill.sh prod --app-port 28080 --ws-port 28081 --keep-artifacts

This compatibility wrapper delegates immediately to:
  espops restore-drill --scope <dev|prod> --project-dir <repo> --compose-file <repo>/compose.yaml [args...]
EOF
}

has_env_file_flag=0
for arg in "$@"; do
  if [[ "$arg" == "--env-file" ]]; then
    has_env_file_flag=1
    break
  fi
done

parse_contour_arg "$@"

if [[ "${ESPO_ENV:-}" == "" && ${#POSITIONAL_ARGS[@]} -gt 0 ]]; then
  case "${POSITIONAL_ARGS[0]}" in
    -h|--help)
      usage
      exit 0
      ;;
  esac
fi

require_explicit_contour

args=(
  restore-drill
  --scope "$ESPO_ENV"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

if [[ -n "${ENV_FILE:-}" && $has_env_file_flag -eq 0 ]]; then
  args+=(--env-file "$ENV_FILE")
fi

args+=("${POSITIONAL_ARGS[@]}")

run_espops "${args[@]}"
