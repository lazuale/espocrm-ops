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
source "$SCRIPT_DIR/lib/fs.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/smoke-test.sh <dev|prod> [--from-example] [--timeout SEC] [--keep-artifacts]

Examples:
  ./scripts/smoke-test.sh dev --from-example
  ./scripts/smoke-test.sh prod --from-example --timeout 900
  ./scripts/smoke-test.sh dev --keep-artifacts

  --timeout SEC sets the shared readiness budget for the smoke stack.
EOF
}

cleanup() {
  local exit_code="$1"

  if [[ -n "${SMOKE_ENV_FILE:-}" && -f "${SMOKE_ENV_FILE:-}" ]]; then
    if [[ ${KEEP_ARTIFACTS:-0} -eq 0 ]]; then
      ENV_FILE="$SMOKE_ENV_FILE" compose down >/dev/null 2>&1 || true
      rm -f "$SMOKE_ENV_FILE"
      if [[ -n "${SMOKE_DB_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_DB_STORAGE_ABS"
      fi
      if [[ -n "${SMOKE_ESPO_STORAGE_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_ESPO_STORAGE_ABS"
      fi
      if [[ -n "${SMOKE_BACKUP_ROOT_ABS:-}" ]]; then
        safe_remove_tree "$SMOKE_BACKUP_ROOT_ABS"
      fi
    else
      warn "Temporary smoke environment preserved because of --keep-artifacts"
      warn "Env file: $SMOKE_ENV_FILE"
    fi
  fi

  exit "$exit_code"
}

parse_contour_arg "$@"
USE_EXAMPLE_ENV=0
KEEP_ARTIFACTS=0
TIMEOUT_SECONDS=600

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --from-example)
      USE_EXAMPLE_ENV=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --keep-artifacts)
      KEEP_ARTIFACTS=1
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

require_compose

SMOKE_ENV_FILE=""

if [[ $USE_EXAMPLE_ENV -eq 1 ]]; then
  EXAMPLE_ENV_FILE="$ROOT_DIR/ops/env/.env.${ESPO_ENV}.example"
  [[ -f "$EXAMPLE_ENV_FILE" ]] || die "Env-file template not found: $EXAMPLE_ENV_FILE"

  mkdir -p "$ROOT_DIR/.cache/env"
  SMOKE_ENV_FILE="$(mktemp "$ROOT_DIR/.cache/env/smoke.${ESPO_ENV}.XXXXXX.env")"
  cp "$EXAMPLE_ENV_FILE" "$SMOKE_ENV_FILE"

  if [[ "$ESPO_ENV" == "prod" ]]; then
    SMOKE_APP_PORT=18080
    SMOKE_WS_PORT=18081
  else
    SMOKE_APP_PORT=18088
    SMOKE_WS_PORT=18089
  fi

  set_env_value "$SMOKE_ENV_FILE" "COMPOSE_PROJECT_NAME" "espo-smoke-$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "DB_STORAGE_DIR" "./storage/smoke/$ESPO_ENV/db"
  set_env_value "$SMOKE_ENV_FILE" "ESPO_STORAGE_DIR" "./storage/smoke/$ESPO_ENV/espo"
  set_env_value "$SMOKE_ENV_FILE" "BACKUP_ROOT" "./backups/smoke/$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "BACKUP_NAME_PREFIX" "espocrm-smoke-$ESPO_ENV"
  set_env_value "$SMOKE_ENV_FILE" "APP_PORT" "$SMOKE_APP_PORT"
  set_env_value "$SMOKE_ENV_FILE" "WS_PORT" "$SMOKE_WS_PORT"
  set_env_value "$SMOKE_ENV_FILE" "SITE_URL" "http://127.0.0.1:$SMOKE_APP_PORT"
  set_env_value "$SMOKE_ENV_FILE" "WS_PUBLIC_URL" "ws://127.0.0.1:$SMOKE_WS_PORT"

  export ENV_FILE="$SMOKE_ENV_FILE"
  resolve_env_file
else
  resolve_env_file
  warn "The smoke test will run against a real contour '$ESPO_ENV'"
fi

trap 'cleanup $?' EXIT

acquire_operation_lock smoke-test
load_env
ensure_runtime_dirs
acquire_maintenance_lock smoke-test
# shellcheck disable=SC2034
READINESS_TIMEOUT_BUDGET="$TIMEOUT_SECONDS"

if [[ $USE_EXAMPLE_ENV -eq 1 ]]; then
  SMOKE_DB_STORAGE_ABS="$(root_path "$DB_STORAGE_DIR")"
  SMOKE_ESPO_STORAGE_ABS="$(root_path "$ESPO_STORAGE_DIR")"
  SMOKE_BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

  safe_remove_tree "$SMOKE_DB_STORAGE_ABS"
  safe_remove_tree "$SMOKE_ESPO_STORAGE_ABS"
  safe_remove_tree "$SMOKE_BACKUP_ROOT_ABS"
  ensure_runtime_dirs
fi

print_context

echo "[1/5] Starting the smoke stack"
compose up -d

echo "[2/5] Waiting for service readiness"
wait_for_application_stack_with_shared_timeout READINESS_TIMEOUT_BUDGET "smoke test"

echo "[3/5] Checking application HTTP availability: $SITE_URL"
http_probe "$SITE_URL"

echo "[4/5] Creating a backup"
ESPO_SHELL_EXEC_CONTEXT=1 run_repo_script "$SCRIPT_DIR/backup.sh" "$ESPO_ENV"

echo "[5/5] Verifying the backup"
run_repo_script "$SCRIPT_DIR/verify-backup.sh" "$ESPO_ENV"

echo "Smoke test completed successfully"
