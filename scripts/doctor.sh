#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/doctor.sh <dev|prod|all> [args...]

Examples:
  ./scripts/doctor.sh all
  ./scripts/doctor.sh prod
  ./scripts/doctor.sh dev --json
EOF
}

doctor_args=(doctor --project-dir "$ROOT_DIR")

if [[ $# -gt 0 && "$1" != -* ]]; then
  doctor_args+=(--scope "$1")
  shift
fi

if [[ -n "${ENV_FILE:-}" ]]; then
  doctor_args+=(--env-file "$ENV_FILE")
fi

doctor_args+=("$@")
run_espops "${doctor_args[@]}"
