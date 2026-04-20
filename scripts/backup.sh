#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/backup.sh <dev|prod> [--skip-db] [--skip-files] [--no-stop] [args...]
  ./scripts/backup.sh verify [--manifest PATH | --backup-root PATH] [args...]

Examples:
  ./scripts/backup.sh prod
  ./scripts/backup.sh prod --skip-files
  ./scripts/backup.sh prod --no-stop
  ./scripts/backup.sh verify --manifest /path/to/manifest.json
  ./scripts/backup.sh verify --backup-root /path/to/backups/prod
EOF
}

run_backup_create() {
  local args=(backup --project-dir "$ROOT_DIR")

  if [[ $# -gt 0 && "$1" != -* ]]; then
    args+=(--scope "$1")
    shift
  fi
  if [[ -n "${ENV_FILE:-}" ]]; then
    args+=(--env-file "$ENV_FILE")
  fi

  args+=("$@")
  run_espops "${args[@]}"
}

run_backup_verify() {
  local args=(backup verify)
  args+=("$@")
  run_espops "${args[@]}"
}

case "${1:-}" in
  -h|--help)
    usage
    ;;
  verify)
    shift
    run_backup_verify "$@"
    ;;
  *)
    run_backup_create "$@"
    ;;
esac
