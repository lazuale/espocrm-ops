#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/status-report.sh <dev|prod> [--json] [--output PATH]

Show a canonical detailed contour status report with:
  resolved env and storage context
  doctor readiness summary
  runtime service and lock state
  latest operation summary
  backup, report, and support artifact summary

Examples:
  ./scripts/status-report.sh prod
  ./scripts/status-report.sh dev --json
  ./scripts/status-report.sh prod --output /tmp/prod-status.txt

This compatibility wrapper delegates immediately to:
  espops status-report --scope <dev|prod> --project-dir <repo> --compose-file <repo>/compose.yaml [args...]
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

OUTPUT_PATH=""
PASSTHROUGH_ARGS=()

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--output must be followed by a path"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    *)
      PASSTHROUGH_ARGS+=("${POSITIONAL_ARGS[0]}")
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
  esac
done

require_explicit_contour

args=(
  status-report
  --scope "$ESPO_ENV"
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
)

if [[ -n "${ENV_FILE:-}" && $has_env_file_flag -eq 0 ]]; then
  args+=(--env-file "$ENV_FILE")
fi

args+=("${PASSTHROUGH_ARGS[@]}")

if [[ -z "$OUTPUT_PATH" ]]; then
  run_espops "${args[@]}"
  exit $?
fi

tmp_output="$(mktemp)"
append_trap "rm -f \"$tmp_output\"" EXIT

set +e
run_espops "${args[@]}" > "$tmp_output"
status=$?
set -e

mkdir -p "$(dirname "$OUTPUT_PATH")"
cp "$tmp_output" "$OUTPUT_PATH"
printf 'Report saved: %s\n' "$OUTPUT_PATH"
exit "$status"
