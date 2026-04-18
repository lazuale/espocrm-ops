#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/compose.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/update.sh <dev|prod> [--dry-run] [--skip-doctor] [--skip-backup] [--skip-pull] [--skip-http-probe] [--timeout SEC]

Examples:
  ./scripts/update.sh prod
  ./scripts/update.sh prod --dry-run
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

RAW_ARGS=("$@")
parse_contour_arg "$@"
SKIP_DOCTOR=0
SKIP_BACKUP=0
SKIP_PULL=0
SKIP_HTTP_PROBE=0
DRY_RUN=0
BUNDLE_ON_FAIL=1
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --dry-run)
      DRY_RUN=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
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

if [[ $DRY_RUN -eq 1 ]]; then
  update_plan_args=(
    update-plan
    --scope "$ESPO_ENV"
    --project-dir "$ROOT_DIR"
    --compose-file "$ROOT_DIR/compose.yaml"
    --timeout "$TIMEOUT_SECONDS"
  )

  if [[ -n "${ENV_FILE:-}" ]]; then
    update_plan_args+=(--env-file "$ENV_FILE")
  fi
  if [[ $SKIP_DOCTOR -eq 1 ]]; then
    update_plan_args+=(--skip-doctor)
  fi
  if [[ $SKIP_BACKUP -eq 1 ]]; then
    update_plan_args+=(--skip-backup)
  fi
  if [[ $SKIP_PULL -eq 1 ]]; then
    update_plan_args+=(--skip-pull)
  fi
  if [[ $SKIP_HTTP_PROBE -eq 1 ]]; then
    update_plan_args+=(--skip-http-probe)
  fi

  run_espops "${update_plan_args[@]}"
  exit $?
fi

if [[ "${ESPO_SHELL_EXEC_CONTEXT:-0}" != "1" ]]; then
  preflight_args=(
    run-operation
    --scope "$ESPO_ENV"
    --operation update
    --project-dir "$ROOT_DIR"
  )

  if [[ -n "${ENV_FILE:-}" ]]; then
    preflight_args+=(--env-file "$ENV_FILE")
  fi

  preflight_args+=(-- bash "$0" "${RAW_ARGS[@]}")
  run_espops "${preflight_args[@]}"
  exit $?
fi

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

if [[ $SKIP_BACKUP -eq 0 ]]; then
  update_backup_args=(
    update-backup
    --scope "$ESPO_ENV"
    --project-dir "$ROOT_DIR"
    --compose-file "$ROOT_DIR/compose.yaml"
    --env-file "$ENV_FILE"
    --timeout "$TIMEOUT_SECONDS"
  )
  run_espops "${update_backup_args[@]}"
else
  echo "Backup skipped because of --skip-backup"
fi

update_runtime_args=(
  update-runtime
  --project-dir "$ROOT_DIR"
  --compose-file "$ROOT_DIR/compose.yaml"
  --env-file "$ENV_FILE"
  --site-url "$SITE_URL"
  --timeout "$TIMEOUT_SECONDS"
)

if [[ $SKIP_PULL -eq 1 ]]; then
  update_runtime_args+=(--skip-pull)
fi

if [[ $SKIP_HTTP_PROBE -eq 1 ]]; then
  update_runtime_args+=(--skip-http-probe)
fi

run_espops "${update_runtime_args[@]}"

ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --output "$POST_REPORT_TXT"
ENV_FILE="$ENV_FILE" run_repo_script "$SCRIPT_DIR/status-report.sh" "$ESPO_ENV" --json --output "$POST_REPORT_JSON"
cleanup_old_files "$REPORTS_DIR" "$REPORT_RETENTION" '*.txt' '*.json'

trap - ERR

echo "Update completed successfully"
echo "Pre-update report:  $PRE_REPORT_TXT"
echo "Post-update report: $POST_REPORT_TXT"
