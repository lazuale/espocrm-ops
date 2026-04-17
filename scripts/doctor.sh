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
source "$SCRIPT_DIR/lib/report.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/fs.sh"

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

JSON_MODE=0
TARGET_SCOPE="all"
MIN_DOCKER_VERSION="24.0.0"
MIN_DOCKER_COMPOSE_VERSION="2.20.0"
DOCKER_AVAILABLE=0
DOCKER_DAEMON_AVAILABLE=0
DOCKER_COMPOSE_AVAILABLE=0
DOCKER_ENGINE_VERSION=""
DOCKER_COMPOSE_VERSION=""
declare -A LOADED_APP_PORTS=()
declare -A LOADED_WS_PORTS=()
declare -A LOADED_PROJECTS=()
declare -A LOADED_DB_STORAGE_DIRS=()
declare -A LOADED_ESPO_STORAGE_DIRS=()
declare -A LOADED_BACKUP_ROOTS=()
declare -A LOADED_IMAGES=()
declare -A LOADED_MARIADB_TAGS=()
declare -A LOADED_DEFAULT_LANGUAGES=()
declare -A LOADED_TIME_ZONES=()
report_reset

section() {
  [[ $JSON_MODE -eq 0 ]] || return 0
  echo
  echo "== $* =="
}

ok() {
  report_add ok "$(( JSON_MODE == 0 ))" "$@"
}

warn() {
  report_add warn "$(( JSON_MODE == 0 ))" "$@"
}

fail() {
  report_add fail "$(( JSON_MODE == 0 ))" "$@"
}

render_json_report() {
  local created_at
  created_at="$(date +%F_%H-%M-%S)"

  {
    printf '{\n'
    printf '  "canonical": false,\n'
    printf '  "contract_level": "non_canonical_shell",\n'
    printf '  "machine_contract": false,\n'
    printf '  "created_at": "%s",\n' "$(json_escape "$created_at")"
    printf '  "target_scope": "%s",\n' "$(json_escape "$TARGET_SCOPE")"
    printf '  "success": %s,\n' "$(report_success_json)"
    printf '  "summary": '
    report_summary_json
    printf ',\n'
    printf '  "findings": '
    report_findings_json
    printf '\n}\n'
  }
}

check_sha256_tool() {
  if command_exists sha256sum; then
    ok "sha256sum is available for backup integrity checks"
  elif command_exists shasum; then
    ok "shasum is available for backup integrity checks"
  elif command_exists openssl; then
    ok "openssl is available for backup integrity checks"
  else
    fail "No SHA-256 tool found: sha256sum, shasum, or openssl is required"
  fi
}

check_env_file_permissions() {
  local contour="$1"
  local env_file="$2"
  local mode normalized permission_bits

  mode="$(file_mode_octal "$env_file" 2>/dev/null || true)"
  if [[ -z "$mode" ]]; then
    warn "[$contour] Could not determine env-file permissions: $env_file"
    return
  fi

  normalized="${mode: -3}"
  permission_bits=$((8#$normalized))

  if (( (permission_bits & 0111) != 0 )); then
    warn "[$contour] Env file must not be executable: $env_file (mode $mode)"
  fi

  if (( (permission_bits & 0077) != 0 )); then
    warn "[$contour] Env-file permissions are too broad; 600 is recommended: $env_file (mode $mode)"
  else
    ok "[$contour] Env-file permissions are restricted: $env_file (mode $mode)"
  fi
}

check_url() {
  local contour="$1"
  local name="$2"
  local value="$3"
  local pattern="$4"

  if [[ -z "$value" ]]; then
    fail "[$contour] Variable $name is not set"
    return
  fi

  case "$value" in
    *change_me*|*YOUR_SERVER_IP*)
      fail "[$contour] Variable $name contains a placeholder value: $value"
      return
      ;;
  esac

  if [[ "$value" =~ $pattern ]]; then
    ok "[$contour] Variable $name looks valid"
  else
    fail "[$contour] Variable $name has an invalid format: $value"
  fi
}

check_secret_value() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ -z "$value" ]]; then
    fail "[$contour] Variable $name is not set"
  elif [[ "$value" == *change_me* ]]; then
    fail "[$contour] Variable $name contains a placeholder value"
  else
    ok "[$contour] Variable $name is populated"
  fi
}

check_integer_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    ok "[$contour] Variable $name contains an integer"
  else
    fail "[$contour] Variable $name must be an integer: $value"
  fi
}

check_decimal_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    ok "[$contour] Variable $name contains a number"
  else
    fail "[$contour] Variable $name must be a number: $value"
  fi
}

check_mem_limit_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?[bkmgBKMG]$ ]]; then
    ok "[$contour] Variable $name looks like a memory limit"
  else
    fail "[$contour] Variable $name must look like 512m or 1g: $value"
  fi
}

check_log_size_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?[bkmgBKMG]$ ]]; then
    ok "[$contour] Variable $name looks like a log size"
  else
    fail "[$contour] Variable $name must look like 10m: $value"
  fi
}

version_ge() {
  local current="$1"
  local minimum="$2"
  [[ "$(printf '%s\n%s\n' "$minimum" "$current" | sort -V | head -n 1)" == "$minimum" ]]
}

check_recommended_version() {
  local label="$1"
  local current="$2"
  local minimum="$3"

  if [[ -z "$current" ]]; then
    warn "Could not determine version for $label"
    return
  fi

  if version_ge "$current" "$minimum"; then
    ok "$label  version $current meets the recommended minimum of $minimum"
  else
    warn "$label  version $current is below the recommended minimum of $minimum"
  fi
}

filesystem_free_mb() {
  local path="$1"
  local existing_path
  existing_path="$(nearest_existing_parent "$path")"

  df -Pm "$existing_path" | awk 'NR == 2 { print $4 }'
}

check_free_space() {
  local contour="$1"
  local name="$2"
  local path_value="$3"
  local min_free_mb="$4"
  local absolute_path existing_path free_mb

  absolute_path="$(root_path "$path_value")"
  existing_path="$(nearest_existing_parent "$absolute_path")"

  if ! command_exists df; then
    warn "[$contour] Free-space check for $name was skipped because df is unavailable"
    return
  fi

  free_mb="$(filesystem_free_mb "$absolute_path" 2>/dev/null || true)"
  if [[ ! "$free_mb" =~ ^[0-9]+$ ]]; then
    warn "[$contour] Could not determine free space for $name"
    return
  fi

  if (( free_mb >= min_free_mb )); then
    ok "[$contour] For $name available is $free_mb MB, which is not below the threshold $min_free_mb MB"
  else
    warn "[$contour] For $name only $free_mb MB are available, which is below the recommended threshold $min_free_mb MB"
  fi
}

check_port_setting() {
  local contour="$1"
  local name="$2"
  local value="$3"

  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    fail "[$contour] Port $name must be numeric: $value"
    return
  fi

  if (( value < 1 || value > 65535 )); then
    fail "[$contour] Port $name is outside the allowed range: $value"
    return
  fi

  local port_state
  if is_tcp_port_busy "$value"; then
    if contour_publishes_port "$value"; then
      ok "[$contour] Port $name=$value is already published by the current contour"
    else
      fail "[$contour] Port $name=$value is already in use"
    fi
    return
  else
    port_state=$?
  fi

  case "$port_state" in
    1)
      ok "[$contour] Port $name=$value is free"
      ;;
    2)
      warn "[$contour] Could not check whether port $name=$value: no ss, lsof, or netstat"
      ;;
    *)
      fail "[$contour] Unexpected port-check result for $name=$value"
      ;;
  esac
}

contour_service_is_running() {
  local service="$1"

  [[ $DOCKER_COMPOSE_AVAILABLE -eq 1 && $DOCKER_DAEMON_AVAILABLE -eq 1 ]] || return 1
  compose ps --status running --services 2>/dev/null | grep -qx "$service"
}

contour_publishes_port() {
  local port="$1"

  [[ $DOCKER_COMPOSE_AVAILABLE -eq 1 && $DOCKER_DAEMON_AVAILABLE -eq 1 ]] || return 1
  compose ps --status running 2>/dev/null | grep -q -- ":${port}->"
}

path_is_managed_by_running_contour() {
  local name="$1"
  local absolute_path="$2"

  [[ -e "$absolute_path" ]] || return 1

  case "$name" in
    DB_STORAGE_DIR)
      contour_service_is_running db
      ;;
    ESPO_STORAGE_DIR)
      contour_service_is_running espocrm \
        || contour_service_is_running espocrm-daemon \
        || contour_service_is_running espocrm-websocket
      ;;
    *)
      return 1
      ;;
  esac
}

check_path_setting() {
  local contour="$1"
  local name="$2"
  local path_value="$3"
  local absolute_path
  absolute_path="$(root_path "$path_value")"

  if path_is_writable_or_creatable "$absolute_path"; then
    ok "[$contour] Path $name is writable or creatable: $absolute_path"
  elif [[ "$name" != "BACKUP_ROOT" ]] && path_is_replaceable_directory "$absolute_path"; then
    case "$name" in
      DB_STORAGE_DIR)
        warn "[$contour] Path $name is not directly writable, but this is an allowed toolkit bind-mount mode: the MariaDB directory can be replaced through the parent directory: $absolute_path"
        ;;
      ESPO_STORAGE_DIR)
        warn "[$contour] Path $name is not directly writable, but this is an allowed toolkit directory-mount mode: restore and cleanup replace the directory through the parent path, and file backup can use a Docker fallback when needed: $absolute_path"
        ;;
      *)
        warn "[$contour] Path $name is not directly writable, but can be replaced through the parent directory: $absolute_path"
        ;;
    esac
  elif path_is_managed_by_running_contour "$name" "$absolute_path"; then
    ok "[$contour] Path $name is already served by a running container: $absolute_path"
  else
    fail "[$contour] Path $name is not writable or creatable: $absolute_path"
  fi
}

check_repo_local_path_policy() {
  local contour="$1"
  local name="$2"
  local path_value="$3"
  local absolute_path

  absolute_path="$(root_path "$path_value")"

  if path_is_within "$absolute_path" "$ROOT_DIR"; then
    warn "[$contour] Path $name lives inside the toolkit repository: $absolute_path. For persistent contours, keep runtime data and backups outside the project so the working tree does not accumulate EspoCRM trees and support artifacts."
  else
    ok "[$contour] Path $name is outside the toolkit repository"
  fi
}

check_required_var() {
  local contour="$1"
  local var_name="$2"
  local value="${!var_name:-}"

  if [[ -z "$value" ]]; then
    fail "[$contour] Required variable is not set: $var_name"
    return 1
  fi

  return 0
}

check_compose_config_for_contour() {
  local contour="$1"

  if compose config >/dev/null 2>&1; then
    ok "[$contour] docker compose config succeeds"
  else
    fail "[$contour] docker compose config failed"
  fi
}

check_cross_contour_must_differ() {
  local name="$1"
  local prod_value="$2"
  local dev_value="$3"

  if [[ "$prod_value" == "$dev_value" ]]; then
    fail "[cross] $name matches between prod and dev: $prod_value"
  else
    ok "[cross] $name differs between prod and dev"
  fi
}

check_cross_contour_must_match() {
  local name="$1"
  local prod_value="$2"
  local dev_value="$3"
  local reason="$4"

  if [[ "$prod_value" == "$dev_value" ]]; then
    ok "[cross] $name matches between prod and dev"
  else
    fail "[cross] $name must match between prod and dev: '$prod_value' vs '$dev_value'. $reason"
  fi
}

check_cross_contour_conflicts() {
  if [[ -z "${LOADED_PROJECTS[prod]:-}" || -z "${LOADED_PROJECTS[dev]:-}" ]]; then
    return
  fi

  # These settings must differ for isolation and match for migrate/restore safety.
  section "Cross-checking dev and prod"

  check_cross_contour_must_differ "COMPOSE_PROJECT_NAME" "${LOADED_PROJECTS[prod]}" "${LOADED_PROJECTS[dev]}"
  check_cross_contour_must_differ "APP_PORT" "${LOADED_APP_PORTS[prod]}" "${LOADED_APP_PORTS[dev]}"
  check_cross_contour_must_differ "WS_PORT" "${LOADED_WS_PORTS[prod]}" "${LOADED_WS_PORTS[dev]}"
  check_cross_contour_must_differ "DB_STORAGE_DIR" "${LOADED_DB_STORAGE_DIRS[prod]}" "${LOADED_DB_STORAGE_DIRS[dev]}"
  check_cross_contour_must_differ "ESPO_STORAGE_DIR" "${LOADED_ESPO_STORAGE_DIRS[prod]}" "${LOADED_ESPO_STORAGE_DIRS[dev]}"
  check_cross_contour_must_differ "BACKUP_ROOT" "${LOADED_BACKUP_ROOTS[prod]}" "${LOADED_BACKUP_ROOTS[dev]}"

  check_cross_contour_must_match "ESPOCRM_IMAGE" "${LOADED_IMAGES[prod]}" "${LOADED_IMAGES[dev]}" \
    "Otherwise cross-contour migration or restore can load data into a different EspoCRM runtime version."
  check_cross_contour_must_match "MARIADB_TAG" "${LOADED_MARIADB_TAGS[prod]}" "${LOADED_MARIADB_TAGS[dev]}" \
    "Otherwise cross-contour migration or restore can hit an incompatible MariaDB version."
  check_cross_contour_must_match "ESPO_DEFAULT_LANGUAGE" "${LOADED_DEFAULT_LANGUAGES[prod]}" "${LOADED_DEFAULT_LANGUAGES[dev]}" \
    "Otherwise application behavior after migration can depend on the contour instead of the migrated state."
  check_cross_contour_must_match "ESPO_TIME_ZONE" "${LOADED_TIME_ZONES[prod]}" "${LOADED_TIME_ZONES[dev]}" \
    "Otherwise dates and background jobs can behave differently after migration."
}

check_contour() {
  local contour="$1"
  local env_file_path=""
  local required_vars=(
    COMPOSE_PROJECT_NAME
    ESPOCRM_IMAGE
    MARIADB_TAG
    DB_STORAGE_DIR
    ESPO_STORAGE_DIR
    BACKUP_ROOT
    BACKUP_NAME_PREFIX
    BACKUP_RETENTION_DAYS
    BACKUP_MAX_DB_AGE_HOURS
    BACKUP_MAX_FILES_AGE_HOURS
    REPORT_RETENTION_DAYS
    SUPPORT_RETENTION_DAYS
    MIN_FREE_DISK_MB
    DOCKER_LOG_MAX_SIZE
    DOCKER_LOG_MAX_FILE
    DB_MEM_LIMIT
    DB_CPUS
    DB_PIDS_LIMIT
    ESPO_MEM_LIMIT
    ESPO_CPUS
    ESPO_PIDS_LIMIT
    DAEMON_MEM_LIMIT
    DAEMON_CPUS
    DAEMON_PIDS_LIMIT
    WS_MEM_LIMIT
    WS_CPUS
    WS_PIDS_LIMIT
    APP_PORT
    WS_PORT
    SITE_URL
    WS_PUBLIC_URL
    DB_ROOT_PASSWORD
    DB_NAME
    DB_USER
    DB_PASSWORD
    ADMIN_USERNAME
    ADMIN_PASSWORD
    ESPO_DEFAULT_LANGUAGE
    ESPO_TIME_ZONE
    ESPO_LOGGER_LEVEL
  )

  section "Checking contour: $contour"

  # shellcheck disable=SC2034
  ESPO_ENV="$contour"
  if [[ "$TARGET_SCOPE" == "all" ]]; then
    unset ENV_FILE
  fi
  if ! resolve_env_file --soft; then
    fail "[$contour] Env file for contour not found"
    return
  fi
  env_file_path="${ENV_FILE:-}"
  ok "[$contour] Env file found: $env_file_path"
  check_env_file_permissions "$contour" "$env_file_path"

  if ! load_env 2>/dev/null; then
    fail "[$contour] Could not load env file: $env_file_path"
    return
  fi
  ok "[$contour] Env file loaded successfully"

  local missing_required=0
  local var_name
  for var_name in "${required_vars[@]}"; do
    if ! check_required_var "$contour" "$var_name"; then
      missing_required=1
    fi
  done

  if [[ $missing_required -ne 0 ]]; then
    return
  fi

  LOADED_PROJECTS["$contour"]="$COMPOSE_PROJECT_NAME"
  LOADED_APP_PORTS["$contour"]="$APP_PORT"
  LOADED_WS_PORTS["$contour"]="$WS_PORT"
  LOADED_DB_STORAGE_DIRS["$contour"]="$DB_STORAGE_DIR"
  LOADED_ESPO_STORAGE_DIRS["$contour"]="$ESPO_STORAGE_DIR"
  LOADED_BACKUP_ROOTS["$contour"]="$BACKUP_ROOT"
  LOADED_IMAGES["$contour"]="$ESPOCRM_IMAGE"
  LOADED_MARIADB_TAGS["$contour"]="$MARIADB_TAG"
  LOADED_DEFAULT_LANGUAGES["$contour"]="$ESPO_DEFAULT_LANGUAGE"
  LOADED_TIME_ZONES["$contour"]="$ESPO_TIME_ZONE"

  # Validate the env contract before ports, paths, and compose rendering.
  check_secret_value "$contour" "DB_ROOT_PASSWORD" "$DB_ROOT_PASSWORD"
  check_secret_value "$contour" "DB_PASSWORD" "$DB_PASSWORD"
  check_secret_value "$contour" "ADMIN_PASSWORD" "$ADMIN_PASSWORD"
  check_url "$contour" "SITE_URL" "$SITE_URL" '^https?://'
  check_url "$contour" "WS_PUBLIC_URL" "$WS_PUBLIC_URL" '^wss?://'
  check_integer_setting "$contour" "BACKUP_RETENTION_DAYS" "$BACKUP_RETENTION_DAYS"
  check_integer_setting "$contour" "BACKUP_MAX_DB_AGE_HOURS" "$BACKUP_MAX_DB_AGE_HOURS"
  check_integer_setting "$contour" "BACKUP_MAX_FILES_AGE_HOURS" "$BACKUP_MAX_FILES_AGE_HOURS"
  check_integer_setting "$contour" "REPORT_RETENTION_DAYS" "$REPORT_RETENTION_DAYS"
  check_integer_setting "$contour" "SUPPORT_RETENTION_DAYS" "$SUPPORT_RETENTION_DAYS"
  check_integer_setting "$contour" "MIN_FREE_DISK_MB" "$MIN_FREE_DISK_MB"
  check_integer_setting "$contour" "DOCKER_LOG_MAX_FILE" "$DOCKER_LOG_MAX_FILE"
  check_log_size_setting "$contour" "DOCKER_LOG_MAX_SIZE" "$DOCKER_LOG_MAX_SIZE"
  check_mem_limit_setting "$contour" "DB_MEM_LIMIT" "$DB_MEM_LIMIT"
  check_decimal_setting "$contour" "DB_CPUS" "$DB_CPUS"
  check_integer_setting "$contour" "DB_PIDS_LIMIT" "$DB_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "ESPO_MEM_LIMIT" "$ESPO_MEM_LIMIT"
  check_decimal_setting "$contour" "ESPO_CPUS" "$ESPO_CPUS"
  check_integer_setting "$contour" "ESPO_PIDS_LIMIT" "$ESPO_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "DAEMON_MEM_LIMIT" "$DAEMON_MEM_LIMIT"
  check_decimal_setting "$contour" "DAEMON_CPUS" "$DAEMON_CPUS"
  check_integer_setting "$contour" "DAEMON_PIDS_LIMIT" "$DAEMON_PIDS_LIMIT"
  check_mem_limit_setting "$contour" "WS_MEM_LIMIT" "$WS_MEM_LIMIT"
  check_decimal_setting "$contour" "WS_CPUS" "$WS_CPUS"
  check_integer_setting "$contour" "WS_PIDS_LIMIT" "$WS_PIDS_LIMIT"
  check_port_setting "$contour" "APP_PORT" "$APP_PORT"
  check_port_setting "$contour" "WS_PORT" "$WS_PORT"

  if [[ "$APP_PORT" == "$WS_PORT" ]]; then
    fail "[$contour] APP_PORT and WS_PORT must not match"
  else
    ok "[$contour] APP_PORT and WS_PORT do not conflict"
  fi

  check_path_setting "$contour" "DB_STORAGE_DIR" "$DB_STORAGE_DIR"
  check_path_setting "$contour" "ESPO_STORAGE_DIR" "$ESPO_STORAGE_DIR"
  check_path_setting "$contour" "BACKUP_ROOT" "$BACKUP_ROOT"
  check_repo_local_path_policy "$contour" "DB_STORAGE_DIR" "$DB_STORAGE_DIR"
  check_repo_local_path_policy "$contour" "ESPO_STORAGE_DIR" "$ESPO_STORAGE_DIR"
  check_repo_local_path_policy "$contour" "BACKUP_ROOT" "$BACKUP_ROOT"
  check_free_space "$contour" "DB_STORAGE_DIR" "$DB_STORAGE_DIR" "$MIN_FREE_DISK_MB"
  check_free_space "$contour" "ESPO_STORAGE_DIR" "$ESPO_STORAGE_DIR" "$MIN_FREE_DISK_MB"
  check_free_space "$contour" "BACKUP_ROOT" "$BACKUP_ROOT" "$MIN_FREE_DISK_MB"

  if [[ $DOCKER_COMPOSE_AVAILABLE -eq 1 ]]; then
    check_compose_config_for_contour "$contour"
  else
    warn "[$contour] docker compose config check skipped because docker compose is unavailable"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    all|prod|dev)
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

case "$TARGET_SCOPE" in
  all)
    CONTOURS=(prod dev)
    ;;
  prod|dev)
    CONTOURS=("$TARGET_SCOPE")
    ;;
  *)
    usage >&2
    die "Unknown check mode: $TARGET_SCOPE"
    ;;
esac

acquire_operation_lock doctor
section "Baseline environment checks"
if command_exists bash; then
  ok "bash is available"
else
  fail "bash command not found"
fi

if command_exists docker; then
  DOCKER_AVAILABLE=1
  ok "docker is available"
else
  fail "docker command not found"
fi

if [[ $DOCKER_AVAILABLE -eq 1 ]] && docker info >/dev/null 2>&1; then
  DOCKER_DAEMON_AVAILABLE=1
  DOCKER_ENGINE_VERSION="$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)"
  ok "Docker daemon is available"
else
  fail "Docker daemon is unavailable or not running"
fi

if [[ $DOCKER_AVAILABLE -eq 1 ]] && docker compose version >/dev/null 2>&1; then
  DOCKER_COMPOSE_AVAILABLE=1
  DOCKER_COMPOSE_VERSION="$(docker compose version --short 2>/dev/null || true)"
  ok "docker is available compose"
else
  fail "docker compose is unavailable"
fi

if [[ $DOCKER_DAEMON_AVAILABLE -eq 1 ]]; then
  check_recommended_version "Docker Engine" "$DOCKER_ENGINE_VERSION" "$MIN_DOCKER_VERSION"
fi

if [[ $DOCKER_COMPOSE_AVAILABLE -eq 1 ]]; then
  check_recommended_version "Docker Compose" "$DOCKER_COMPOSE_VERSION" "$MIN_DOCKER_COMPOSE_VERSION"
fi

check_sha256_tool

for contour in "${CONTOURS[@]}"; do
  check_contour "$contour"
done

if [[ "$TARGET_SCOPE" == "all" ]]; then
  check_cross_contour_conflicts
fi

if [[ $JSON_MODE -eq 0 ]]; then
  echo
  echo "Check summary: ok=$(report_ok_count) warn=$(report_warn_count) fail=$(report_fail_count)"
else
  render_json_report
fi

if report_has_failures; then
  exit 1
fi
