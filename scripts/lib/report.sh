#!/usr/bin/env bash
set -Eeuo pipefail

# Narrow state and report helper library for doctor and audit flows.
# Load it only after `scripts/lib/common.sh`.

REPORT_OK_COUNT=0
REPORT_WARN_COUNT=0
REPORT_FAIL_COUNT=0
declare -a REPORT_FINDING_LEVELS=()
declare -a REPORT_FINDING_MESSAGES=()

report_reset() {
  REPORT_OK_COUNT=0
  REPORT_WARN_COUNT=0
  REPORT_FAIL_COUNT=0
  REPORT_FINDING_LEVELS=()
  REPORT_FINDING_MESSAGES=()
}

report_add() {
  local level="$1"
  local emit_stdout="${2:-1}"
  shift 2

  local message="$*"

  REPORT_FINDING_LEVELS+=("$level")
  REPORT_FINDING_MESSAGES+=("$message")

  case "$level" in
    ok) REPORT_OK_COUNT=$((REPORT_OK_COUNT + 1)) ;;
    warn) REPORT_WARN_COUNT=$((REPORT_WARN_COUNT + 1)) ;;
    fail) REPORT_FAIL_COUNT=$((REPORT_FAIL_COUNT + 1)) ;;
    *) die "Unknown finding level: $level" ;;
  esac

  if [[ "$emit_stdout" == "1" ]]; then
    printf '[%s] %s\n' "$level" "$message"
  fi
}

report_ok_count() {
  printf '%s' "$REPORT_OK_COUNT"
}

report_warn_count() {
  printf '%s' "$REPORT_WARN_COUNT"
}

report_fail_count() {
  printf '%s' "$REPORT_FAIL_COUNT"
}

report_has_failures() {
  [[ $REPORT_FAIL_COUNT -ne 0 ]]
}

report_success_json() {
  if report_has_failures; then
    printf 'false'
  else
    printf 'true'
  fi
}

report_summary_json() {
  printf '{\n'
  printf '    "ok": %s,\n' "$REPORT_OK_COUNT"
  printf '    "warn": %s,\n' "$REPORT_WARN_COUNT"
  printf '    "fail": %s\n' "$REPORT_FAIL_COUNT"
  printf '  }'
}

report_findings_json() {
  local first=1
  local index

  printf '['
  for index in "${!REPORT_FINDING_LEVELS[@]}"; do
    if [[ $first -eq 0 ]]; then
      printf ','
    fi
    printf '\n    {"level": "%s", "message": "%s"}' \
      "$(json_escape "${REPORT_FINDING_LEVELS[$index]}")" \
      "$(json_escape "${REPORT_FINDING_MESSAGES[$index]}")"
    first=0
  done

  if [[ ${#REPORT_FINDING_LEVELS[@]} -gt 0 ]]; then
    printf '\n  '
  fi
  printf ']'
}
