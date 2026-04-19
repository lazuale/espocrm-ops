#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/restore-db.sh /path/to/manifest.json [--dry-run]
  ./scripts/restore-db.sh <dev|prod> /path/to/backup.sql.gz [--force] [--confirm-prod prod] [--dry-run] [--no-snapshot] [--snapshot-before-restore] [--no-stop] [--no-start]

Examples:
  ESPOPS_DB_CONTAINER=espocrm-prod-db-1 ESPOPS_DB_NAME=espocrm ESPOPS_DB_USER=espocrm ESPOPS_DB_PASSWORD_FILE=/run/secrets/db-password ./scripts/restore-db.sh /opt/espocrm-data/backups/prod/manifests/espocrm-prod_YYYY-MM-DD_HH-MM-SS.manifest.json --dry-run
  ./scripts/restore-db.sh dev /opt/espocrm-data/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz --force
  ./scripts/restore-db.sh prod /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz --force --confirm-prod prod --no-start

This compatibility wrapper delegates immediately to:
  espops restore --scope <dev|prod> --project-dir <repo> --compose-file <repo>/compose.yaml --skip-files ...
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

if [[ "${1:-}" != "dev" && "${1:-}" != "prod" ]]; then
  DIRECT_DRY_RUN=0
  if [[ $# -eq 2 && "${2:-}" == "--dry-run" ]]; then
    DIRECT_DRY_RUN=1
  elif [[ $# -ne 1 ]]; then
    echo "usage: $0 MANIFEST.json [--dry-run]" >&2
    exit 2
  fi

  args=(
    restore-db
    --manifest "$(caller_path "$1")"
  )
  if [[ $DIRECT_DRY_RUN -eq 1 ]]; then
    args+=(--dry-run)
  fi

  run_espops "${args[@]}"
  exit $?
fi

parse_contour_arg "$@"
require_explicit_contour

args=(
  restore
  --scope "$ESPO_ENV"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
  --skip-files
)

if [[ -n "${ENV_FILE:-}" ]]; then
  args+=(--env-file "$ENV_FILE")
fi

backup_path=""
manifest_path=""
while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --force|--dry-run|--no-snapshot|--snapshot-before-restore|--no-stop|--no-start)
      args+=("${POSITIONAL_ARGS[0]}")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --confirm-prod)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--confirm-prod must be followed by prod"
      args+=("${POSITIONAL_ARGS[0]}" "${POSITIONAL_ARGS[1]}")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
    *)
      if [[ -n "$backup_path" || -n "$manifest_path" ]]; then
        usage >&2
        die "More than one restore source was provided"
      fi
      backup_path="$(caller_path "${POSITIONAL_ARGS[0]}")"
      args+=(--db-backup "$backup_path")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
  esac
done

run_espops "${args[@]}"
