#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/restore-files.sh /path/to/manifest.json /path/to/target-dir [--dry-run]
  ./scripts/restore-files.sh <dev|prod> /path/to/files.tar.gz [--force] [--confirm-prod prod] [--dry-run] [--no-snapshot] [--snapshot-before-restore] [--no-stop] [--no-start]
  ./scripts/restore-files.sh <dev|prod> --manifest /path/to/manifest.json [--force] [--confirm-prod prod] [--dry-run] [--no-snapshot] [--snapshot-before-restore] [--no-stop] [--no-start]

Examples:
  ./scripts/restore-files.sh /opt/espocrm-data/backups/prod/manifests/espocrm-prod_YYYY-MM-DD_HH-MM-SS.manifest.json /var/lib/espocrm/storage
  ./scripts/restore-files.sh dev /opt/espocrm-data/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz --force
  ./scripts/restore-files.sh prod --manifest /opt/espocrm-data/backups/prod/manifests/espocrm-prod_YYYY-MM-DD_HH-MM-SS.manifest.json --force --confirm-prod prod --no-start

This compatibility wrapper delegates immediately to:
  espops restore --scope <dev|prod> --project-dir <repo> --compose-file <repo>/compose.yaml --skip-db ...
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
  if [[ $# -eq 3 && "${3:-}" == "--dry-run" ]]; then
    DIRECT_DRY_RUN=1
  elif [[ $# -ne 2 ]]; then
    echo "usage: $0 MANIFEST.json TARGET_DIR [--dry-run]" >&2
    exit 2
  fi

  args=(
    restore-files
    --manifest "$(caller_path "$1")"
    --target-dir "$(caller_path "$2")"
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
  --skip-db
)

if [[ -n "${ENV_FILE:-}" ]]; then
  args+=(--env-file "$ENV_FILE")
fi

source_seen=0
while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --force|--dry-run|--no-snapshot|--snapshot-before-restore|--no-stop|--no-start)
      args+=("${POSITIONAL_ARGS[0]}")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --confirm-prod|--manifest)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "${POSITIONAL_ARGS[0]} must be followed by a value"
      if [[ "${POSITIONAL_ARGS[0]}" == "--manifest" ]]; then
        [[ $source_seen -eq 0 ]] || die "Pass either a files backup path or --manifest, but not both"
        source_seen=1
        args+=(--manifest "$(caller_path "${POSITIONAL_ARGS[1]}")")
      else
        args+=("${POSITIONAL_ARGS[0]}" "${POSITIONAL_ARGS[1]}")
      fi
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
      [[ $source_seen -eq 0 ]] || die "Pass either a files backup path or --manifest, but not both"
      source_seen=1
      args+=(--files-backup "$(caller_path "${POSITIONAL_ARGS[0]}")")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
  esac
done

run_espops "${args[@]}"
