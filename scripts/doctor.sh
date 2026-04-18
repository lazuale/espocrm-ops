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

if [[ $JSON_MODE -eq 0 ]]; then
  run_espops "${doctor_args[@]}"
  exit $?
fi

set +e
doctor_output="$(run_espops --json "${doctor_args[@]}" 2>&1)"
doctor_status=$?
set -e

printf '{\n'
printf '  "canonical": false,\n'
printf '  "contract_level": "non_canonical_shell",\n'
printf '  "machine_contract": false,\n'
printf '  "target_scope": "%s",\n' "$(json_escape "$TARGET_SCOPE")"
printf '  "success": %s' "$(json_bool "$(( doctor_status == 0 ))")"

if [[ "$doctor_output" == \{* ]]; then
  printf ',\n  "doctor": %s\n' "$doctor_output"
else
  printf ',\n  "error_message": "%s"\n' "$(json_escape "$doctor_output")"
fi

printf '}\n'
exit "$doctor_status"
