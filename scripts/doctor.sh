#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/doctor.sh [dev|prod|all] [--json]

Examples:
  ./scripts/doctor.sh
  ./scripts/doctor.sh all
  ./scripts/doctor.sh prod
  ./scripts/doctor.sh dev --json
EOF
}

TARGET_SCOPE="all"
JSON_MODE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    all|dev|prod)
      TARGET_SCOPE="$1"
      shift
      ;;
    --json)
      JSON_MODE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown check mode: $1"
      ;;
  esac
done

doctor_args=(
  doctor
  --scope "$TARGET_SCOPE"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

if [[ "$TARGET_SCOPE" != "all" && -n "${ENV_FILE:-}" ]]; then
  doctor_args+=(--env-file "$ENV_FILE")
fi

if [[ $JSON_MODE -eq 1 ]]; then
  run_espops --json "${doctor_args[@]}"
else
  run_espops "${doctor_args[@]}"
fi
