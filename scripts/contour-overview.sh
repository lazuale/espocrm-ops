#!/usr/bin/env bash
set -Eeuo pipefail

# Read-only operator cockpit for one contour.
# It composes existing safe checks and intentionally owns no domain policy.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/contour-overview.sh <dev|prod> [--json] [--output PATH]

Collect a read-only contour summary:
  doctor
  status
  backup audit
  latest valid backup catalog

Examples:
  ./scripts/contour-overview.sh prod
  ./scripts/contour-overview.sh dev --json
  ./scripts/contour-overview.sh prod --output /tmp/prod-overview.txt
EOF
}

add_overview_section() {
  local id="$1"
  local title="$2"
  local command_display="$3"
  shift 3
  local output status

  set +e
  output="$(run_repo_script "$@" 2>&1)"
  status=$?
  set -e

  OVERVIEW_SECTION_IDS+=("$id")
  OVERVIEW_SECTION_TITLES+=("$title")
  OVERVIEW_SECTION_COMMANDS+=("$command_display")
  OVERVIEW_SECTION_OUTPUTS+=("$output")
  OVERVIEW_SECTION_EXIT_CODES+=("$status")

  if [[ $status -ne 0 ]]; then
    OVERVIEW_STATUS=1
    OVERVIEW_FAILED_SECTIONS=$((OVERVIEW_FAILED_SECTIONS + 1))
  fi
}

section_success_bool() {
  local status="$1"

  if [[ "$status" == "0" ]]; then
    printf 'true'
  else
    printf 'false'
  fi
}

overview_status_text() {
  if [[ $OVERVIEW_STATUS -eq 0 ]]; then
    printf 'ok'
  else
    printf 'failed'
  fi
}

overview_next_command() {
  if [[ $OVERVIEW_STATUS -eq 0 ]]; then
    printf './scripts/espo.sh backup %s' "$ESPO_ENV"
  else
    printf './scripts/espo.sh support %s' "$ESPO_ENV"
  fi
}

overview_next_message() {
  if [[ $OVERVIEW_STATUS -eq 0 ]]; then
    printf 'Keep a fresh backup available before making changes.'
  else
    printf 'Do not run update, restore, or rollback until the overview errors are understood.'
  fi
}

render_overview_text() {
  local index section_count
  section_count="${#OVERVIEW_SECTION_IDS[@]}"

  cat <<EOF
Contour: $ESPO_ENV
Mode: read-only overview

This command changes nothing. It collects a short operator snapshot:
environment readiness, current state, backup freshness, and the latest valid set.
EOF

  for ((index=0; index<section_count; index++)); do
    printf '\n== %s ==\n' "${OVERVIEW_SECTION_TITLES[$index]}"
    if [[ -n "${OVERVIEW_SECTION_OUTPUTS[$index]}" ]]; then
      printf '%s\n' "${OVERVIEW_SECTION_OUTPUTS[$index]}"
    fi

    if [[ "${OVERVIEW_SECTION_EXIT_CODES[$index]}" != "0" ]]; then
      printf '[overview] Section failed: %s (exit=%s)\n' \
        "${OVERVIEW_SECTION_TITLES[$index]}" \
        "${OVERVIEW_SECTION_EXIT_CODES[$index]}"
    fi
  done

  printf '\n== Summary ==\n'
  if [[ $OVERVIEW_STATUS -eq 0 ]]; then
    cat <<EOF
Read-only overview completed without errors.
$(overview_next_message)
  $(overview_next_command)
EOF
  else
    cat <<EOF
Overview found problems in one or more sections.
$(overview_next_message)
For troubleshooting:
  $(overview_next_command)
EOF
  fi
}

render_overview_json() {
  local index section_count first_section=1
  section_count="${#OVERVIEW_SECTION_IDS[@]}"

  printf '{\n'
  printf '  "canonical": false,\n'
  printf '  "contract_level": "non_canonical_shell",\n'
  printf '  "machine_contract": false,\n'
  printf '  "created_at": "%s",\n' "$(json_escape "$OVERVIEW_CREATED_AT")"
  printf '  "contour": "%s",\n' "$(json_escape "$ESPO_ENV")"
  printf '  "mode": "read_only",\n'
  printf '  "status": "%s",\n' "$(overview_status_text)"
  printf '  "success": %s,\n' "$(json_bool "$((1 - OVERVIEW_STATUS))")"
  printf '  "summary": {\n'
  printf '    "total_sections": %s,\n' "$section_count"
  printf '    "failed_sections": %s,\n' "$OVERVIEW_FAILED_SECTIONS"
  printf '    "next_message": "%s",\n' "$(json_escape "$(overview_next_message)")"
  printf '    "next_command": "%s"\n' "$(json_escape "$(overview_next_command)")"
  printf '  },\n'
  printf '  "sections": ['

  for ((index=0; index<section_count; index++)); do
    if [[ $first_section -eq 0 ]]; then
      printf ','
    fi
    printf '\n'
    printf '    {\n'
    printf '      "id": "%s",\n' "$(json_escape "${OVERVIEW_SECTION_IDS[$index]}")"
    printf '      "title": "%s",\n' "$(json_escape "${OVERVIEW_SECTION_TITLES[$index]}")"
    printf '      "command": "%s",\n' "$(json_escape "${OVERVIEW_SECTION_COMMANDS[$index]}")"
    printf '      "exit_code": %s,\n' "${OVERVIEW_SECTION_EXIT_CODES[$index]}"
    printf '      "success": %s,\n' "$(section_success_bool "${OVERVIEW_SECTION_EXIT_CODES[$index]}")"
    printf '      "output": "%s"\n' "$(json_escape "${OVERVIEW_SECTION_OUTPUTS[$index]}")"
    printf '    }'
    first_section=0
  done

  if [[ $section_count -gt 0 ]]; then
    printf '\n'
  fi
  printf '  ]\n'
  printf '}\n'
}

emit_overview() {
  local content="$1"

  if [[ -n "$OUTPUT_PATH" ]]; then
    mkdir -p "$(dirname "$OUTPUT_PATH")"
    printf '%s\n' "$content" > "$OUTPUT_PATH"
    printf 'Overview saved: %s\n' "$OUTPUT_PATH"
  else
    printf '%s\n' "$content"
  fi
}

parse_contour_arg "$@"
OUTPUT_MODE="text"
OUTPUT_PATH=""

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --json)
      OUTPUT_MODE="json"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --output)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--output must be followed by a path"
      OUTPUT_PATH="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

require_explicit_contour

OVERVIEW_CREATED_AT="$(date +%F_%H-%M-%S)"
OVERVIEW_STATUS=0
OVERVIEW_FAILED_SECTIONS=0
OVERVIEW_CONTENT=""
OVERVIEW_SECTION_IDS=()
OVERVIEW_SECTION_TITLES=()
OVERVIEW_SECTION_COMMANDS=()
OVERVIEW_SECTION_OUTPUTS=()
OVERVIEW_SECTION_EXIT_CODES=()

if [[ "$OUTPUT_MODE" == "json" ]]; then
  add_overview_section "doctor" "Doctor" "./scripts/espo.sh doctor $ESPO_ENV --json" "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV" --json
  add_overview_section "status" "Status" "./scripts/espo.sh status $ESPO_ENV --json" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json
  add_overview_section "backup_audit" "Backup Audit" "./scripts/espo.sh backup audit $ESPO_ENV --json" "$SCRIPT_DIR/backup-audit.sh" "$ESPO_ENV" --json
  add_overview_section "backup_catalog" "Latest Valid Backup" "./scripts/espo.sh backup catalog $ESPO_ENV --json --latest-only --verify-checksum" "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV" --json --latest-only --verify-checksum
  OVERVIEW_CONTENT="$(render_overview_json)"
else
  add_overview_section "doctor" "Doctor" "./scripts/espo.sh doctor $ESPO_ENV" "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV"
  add_overview_section "status" "Status" "./scripts/espo.sh status $ESPO_ENV" "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV"
  add_overview_section "backup_audit" "Backup Audit" "./scripts/espo.sh backup audit $ESPO_ENV" "$SCRIPT_DIR/backup-audit.sh" "$ESPO_ENV"
  add_overview_section "backup_catalog" "Latest Valid Backup" "./scripts/espo.sh backup catalog $ESPO_ENV --latest-only --verify-checksum" "$SCRIPT_DIR/backup-catalog.sh" "$ESPO_ENV" --latest-only --verify-checksum
  OVERVIEW_CONTENT="$(render_overview_text)"
fi

emit_overview "$OVERVIEW_CONTENT"
exit "$OVERVIEW_STATUS"
