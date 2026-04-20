#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/backup.sh <dev|prod> [--skip-db] [--skip-files] [--no-stop] [args...]
  ./scripts/backup.sh verify <dev|prod> [--manifest PATH | --backup-root PATH] [args...]

Examples:
  ./scripts/backup.sh prod
  ./scripts/backup.sh prod --skip-files
  ./scripts/backup.sh prod --no-stop
  ./scripts/backup.sh verify dev
  ./scripts/backup.sh verify prod --manifest /path/to/manifest.json
EOF
}

has_explicit_verify_source() {
  local arg

  for arg in "$@"; do
    case "$arg" in
      --manifest|--backup-root)
        return 0
        ;;
    esac
  done

  return 1
}

run_backup_create() {
  parse_contour_arg "$@"
  require_explicit_contour

  local args=(
    backup
    --scope "$ESPO_ENV"
    --project-dir "$ROOT_DIR"
    --compose-file "$ROOT_DIR/compose.yaml"
  )

  if [[ -n "${ENV_FILE:-}" ]]; then
    args+=(--env-file "$ENV_FILE")
  fi

  args+=("${POSITIONAL_ARGS[@]}")
  run_espops "${args[@]}"
}

run_backup_verify() {
  parse_contour_arg "$@"

  local args=(backup verify)

  if ! has_explicit_verify_source "${POSITIONAL_ARGS[@]}"; then
    require_explicit_contour
    resolve_env_file
    load_env
    args+=(--backup-root "$(root_path "$BACKUP_ROOT")")
  fi

  args+=("${POSITIONAL_ARGS[@]}")
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
