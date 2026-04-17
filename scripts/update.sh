#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/update.sh <dev|prod> [--skip-doctor] [--skip-backup] [--skip-pull] [--skip-http-probe] [--timeout SEC]

Examples:
  ./scripts/update.sh prod
  ./scripts/update.sh dev --skip-backup
  ./scripts/update.sh prod --timeout 900

  --timeout SEC sets the shared service-readiness budget
  across all update wait steps.
EOF
}

on_error() {
  local exit_code="$?"
  trap - ERR

  if [[ ${ERROR_HANDLER_ACTIVE:-0} -eq 1 ]]; then
    exit "$exit_code"
  fi

  ERROR_HANDLER_ACTIVE=1

  if [[ ${BUNDLE_ON_FAIL:-1} -eq 1 && -n "${FAILURE_BUNDLE_PATH:-}" ]]; then
    warn "Update failed, collecting a support bundle"
    run_support_bundle_capture \
      "$SCRIPT_DIR" \
      "$ESPO_ENV" \
      "$ENV_FILE" \
      "$FAILURE_BUNDLE_PATH" \
      "Collected a support bundle for failure analysis" \
      "Could not collect the support bundle automatically"
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
SKIP_DOCTOR=0
SKIP_BACKUP=0
SKIP_PULL=0
SKIP_HTTP_PROBE=0
BUNDLE_ON_FAIL=1
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --skip-doctor)
      SKIP_DOCTOR=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-backup)
      SKIP_BACKUP=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-pull)
      SKIP_PULL=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-http-probe)
      SKIP_HTTP_PROBE=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --timeout)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--timeout must be followed by a number of seconds"
      TIMEOUT_SECONDS="${POSITIONAL_ARGS[1]}"
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

[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || die "Timeout must be an integer number of seconds"

require_explicit_contour

acquire_operation_lock update
resolve_env_file
load_env
ensure_runtime_dirs
acquire_maintenance_lock update
require_compose

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"
REPORTS_DIR="$BACKUP_ROOT_ABS/reports"
SUPPORT_DIR="$BACKUP_ROOT_ABS/support"
NAME_PREFIX="${BACKUP_NAME_PREFIX:-$COMPOSE_PROJECT_NAME}"
REPORT_RETENTION="${REPORT_RETENTION_DAYS:-30}"
STAMP="$(next_unique_stamp \
  "$REPORTS_DIR/${NAME_PREFIX}_pre-update___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_pre-update___STAMP__.json" \
  "$REPORTS_DIR/${NAME_PREFIX}_post-update___STAMP__.txt" \
  "$REPORTS_DIR/${NAME_PREFIX}_post-update___STAMP__.json" \
  "$SUPPORT_DIR/${NAME_PREFIX}_update-failure___STAMP__.tar.gz")"
PRE_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_pre-update_${STAMP}.txt"
PRE_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_pre-update_${STAMP}.json"
POST_REPORT_TXT="$REPORTS_DIR/${NAME_PREFIX}_post-update_${STAMP}.txt"
POST_REPORT_JSON="$REPORTS_DIR/${NAME_PREFIX}_post-update_${STAMP}.json"
FAILURE_BUNDLE_PATH="$SUPPORT_DIR/${NAME_PREFIX}_update-failure_${STAMP}.tar.gz"
ERROR_HANDLER_ACTIVE=0
# shellcheck disable=SC2034
READINESS_TIMEOUT_BUDGET="$TIMEOUT_SECONDS"

trap 'on_error' ERR

print_context

echo "[1/6] Capturing the current contour status"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$PRE_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$PRE_REPORT_JSON"

echo "[2/6] Running the pre-update environment check"
if [[ $SKIP_DOCTOR -eq 0 ]]; then
  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/doctor.sh" "$ESPO_ENV"
else
  echo "Environment check skipped because of --skip-doctor"
fi

echo "[3/6] Creating the pre-update recovery point"
if [[ $SKIP_BACKUP -eq 0 ]]; then
  if ! service_is_running db; then
    note "The DB container was not running, starting db temporarily for backup"
    compose up -d db
    wait_for_service_ready_with_shared_timeout READINESS_TIMEOUT_BUDGET db "update"
  fi

  ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"
else
  echo "Backup skipped because of --skip-backup"
fi

echo "[4/6] Updating images"
if [[ $SKIP_PULL -eq 0 ]]; then
  compose pull
else
  echo "Image pull skipped because of --skip-pull"
fi

echo "[5/6] Restarting the stack with the current configuration"
compose up -d

echo "[6/6] Checking readiness after the update"
wait_for_application_stack_with_shared_timeout READINESS_TIMEOUT_BUDGET "update"

if [[ $SKIP_HTTP_PROBE -eq 0 ]]; then
  http_probe "$SITE_URL"
else
  echo "HTTP probe skipped because of --skip-http-probe"
fi

ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$POST_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$POST_REPORT_JSON"
cleanup_old_files "$REPORTS_DIR" "$REPORT_RETENTION" '*.txt' '*.json'

trap - ERR

echo "Update completed successfully"
echo "Pre-update report:  $PRE_REPORT_TXT"
echo "Post-update report: $POST_REPORT_TXT"
