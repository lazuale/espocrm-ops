#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CALLER_DIR="$(pwd)"
# shellcheck disable=SC2034
POSITIONAL_ARGS=()
declare -a KNOWN_ENV_VARS=(
  ESPO_CONTOUR
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

declare -a LOADED_ENV_VARS=()

PARSED_ENV_VALUE=""

die() {
  echo "Error: $*" >&2
  exit 1
}

usage_error() {
  if declare -F usage >/dev/null 2>&1; then
    usage >&2
  fi
  die "$@"
}

note() {
  echo "[info] $*"
}

warn() {
  echo "[warn] $*" >&2
}

append_trap() {
  local new_command="$1"
  shift

  local signal existing_command raw_trap
  for signal in "$@"; do
    raw_trap="$(trap -p "$signal")"
    existing_command=""

    if [[ -n "$raw_trap" ]]; then
      existing_command="${raw_trap#trap -- \'}"
      existing_command="${existing_command%\' "$signal"}"
    fi

    if [[ -n "$existing_command" ]]; then
      # shellcheck disable=SC2064
      trap "$existing_command"$'\n'"$new_command" "$signal"
    else
      # shellcheck disable=SC2064
      trap "$new_command" "$signal"
    fi
  done
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Run child scripts through bash so checkout mode drift does not matter.
run_repo_script() {
  local script_path="$1"
  shift

  [[ -f "$script_path" ]] || die "Child script not found: $script_path"
  bash "$script_path" "$@"
}

ensure_espops_binary() {
  local bin_path="${1:-$ROOT_DIR/bin/espops}"

  if [[ -x "$bin_path" ]]; then
    return 0
  fi

  die "A prebuilt Go CLI is required: $bin_path. Build it explicitly with make build or set ESPOPS_BIN"
}

run_espops() {
  local bin_path="${ESPOPS_BIN:-$ROOT_DIR/bin/espops}"

  ensure_espops_binary "$bin_path"
  "$bin_path" "$@"
}

json_escape() {
  local value="$1"
  local code octal control_char replacement

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"

  for ((code=1; code<32; code++)); do
    case "$code" in
      8)
        control_char=$'\b'
        replacement='\b'
        ;;
      9)
        control_char=$'\t'
        replacement='\t'
        ;;
      10)
        control_char=$'\n'
        replacement='\n'
        ;;
      12)
        control_char=$'\f'
        replacement='\f'
        ;;
      13)
        control_char=$'\r'
        replacement='\r'
        ;;
      *)
        printf -v octal '%03o' "$code"
        printf -v control_char '%b' "\\$octal"
        printf -v replacement '\\u%04x' "$code"
        ;;
    esac

    value="${value//$control_char/$replacement}"
  done

  printf '%s' "$value"
}

json_bool() {
  if [[ "${1:-0}" == "1" ]]; then
    printf 'true'
  else
    printf 'false'
  fi
}

json_value_or_null() {
  local value="${1:-}"

  if [[ -n "$value" ]]; then
    printf '"%s"' "$(json_escape "$value")"
  else
    printf 'null'
  fi
}

json_number_or_null() {
  local value="${1:-}"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
  else
    printf 'null'
  fi
}

parse_contour_arg() {
  ESPO_ENV=""

  if [[ $# -gt 0 ]]; then
    case "$1" in
      dev|prod)
        ESPO_ENV="$1"
        shift
        ;;
    esac
  fi

  export ESPO_ENV
  # shellcheck disable=SC2034
  POSITIONAL_ARGS=("$@")
}

require_explicit_contour() {
  contour_value_is_supported "${ESPO_ENV:-}" && return 0
  usage_error "You must explicitly pass dev or prod as the first argument"
}

contour_value_is_supported() {
  case "${1:-}" in
    dev|prod)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

infer_contour_token_from_text() {
  local value="$1"
  local found=""

  if [[ "$value" =~ (^|[^[:alnum:]])dev([^[:alnum:]]|$) ]]; then
    found="dev"
  fi

  if [[ "$value" =~ (^|[^[:alnum:]])prod([^[:alnum:]]|$) ]]; then
    if [[ -n "$found" && "$found" != "prod" ]]; then
      return 1
    fi
    found="prod"
  fi

  [[ -n "$found" ]] || return 1
  printf '%s\n' "$found"
}

infer_env_file_contour_from_path() {
  local env_file="$1"
  infer_contour_token_from_text "$(basename "$env_file")"
}

# Only explicit `.env.dev` and `.env.prod` files are allowed.
resolve_env_file() {
  local soft_fail=0
  local requested_env_file=""

  case "${1:-}" in
    "")
      ;;
    --soft)
      soft_fail=1
      ;;
    *)
      die "Unknown resolve_env_file argument: $1"
      ;;
  esac

  if [[ -n "${ENV_FILE:-}" ]]; then
    requested_env_file="$ENV_FILE"
    if [[ ! -f "$ENV_FILE" ]]; then
      unset ENV_FILE
      if [[ $soft_fail -eq 1 ]]; then
        return 1
      fi
      die "Overridden env file not found: $requested_env_file"
    fi
    export ENV_FILE
    return
  fi

  case "$ESPO_ENV" in
    prod)
      ENV_FILE="$ROOT_DIR/.env.prod"
      ;;
    dev)
      ENV_FILE="$ROOT_DIR/.env.dev"
      ;;
    *)
      die "Unsupported contour '$ESPO_ENV'. Use dev or prod."
      ;;
  esac

  requested_env_file="$ENV_FILE"

  if [[ ! -f "$ENV_FILE" ]]; then
    unset ENV_FILE
    if [[ $soft_fail -eq 1 ]]; then
      return 1
    fi
    die "Missing $requested_env_file"
  fi

  export ENV_FILE
}

path_mode_octal() {
  stat -c '%a' "$1"
}

path_owner_uid() {
  stat -c '%u' "$1"
}

validate_env_file_for_loading() {
  local env_file="$1"
  local env_mode env_mode_octal env_owner_uid current_uid

  [[ -e "$env_file" ]] || die "Missing $env_file"
  [[ ! -L "$env_file" ]] || die "Env file must not be a symlink: $env_file"
  [[ -f "$env_file" ]] || die "Env file must be a regular file: $env_file"
  [[ -r "$env_file" ]] || die "Env file is not readable: $env_file"

  current_uid="$(id -u)"
  env_owner_uid="$(path_owner_uid "$env_file")" || die "Could not determine env file owner: $env_file"
  [[ "$env_owner_uid" == "$current_uid" || "$env_owner_uid" == "0" ]] \
    || die "Env file must belong to the current user or root: $env_file"

  env_mode="$(path_mode_octal "$env_file")" || die "Could not determine env file permissions: $env_file"
  [[ "$env_mode" =~ ^[0-7]{3,4}$ ]] || die "Unexpected env file permission format: $env_mode ($env_file)"
  env_mode_octal=$((8#$env_mode))

  (( (env_mode_octal & 8#137) == 0 )) \
    || die "Env file must not be broader than 640 and must not have execute bits: $env_file (current $env_mode)"
}

require_destructive_approval() {
  local operation_name="$1"
  local force_flag="$2"
  local confirm_prod="$3"
  local effective_contour="${RESOLVED_ENV_CONTOUR:-$ESPO_ENV}"

  [[ "$force_flag" == "1" ]] || usage_error "Command $operation_name changes data and requires an explicit --force flag"

  if [[ "$effective_contour" == "prod" ]]; then
    [[ "$confirm_prod" == "prod" ]] \
      || usage_error "For the prod contour, command $operation_name requires explicit confirmation via --confirm-prod prod"
  fi
}

validate_loaded_env_file_contour() {
  local env_file="$1"
  local hint_contour="${2:-}"
  local path_contour=""
  local declared_contour="${ESPO_CONTOUR:-}"
  local effective_contour=""

  path_contour="$(infer_env_file_contour_from_path "$env_file" || true)"

  if [[ -n "$declared_contour" ]] && ! contour_value_is_supported "$declared_contour"; then
    die "ESPO_CONTOUR in the env file must be dev or prod: $declared_contour ($env_file)"
  fi

  if [[ -n "$hint_contour" ]] && ! contour_value_is_supported "$hint_contour"; then
    die "Environment variable ESPO_ENV_FILE_CONTOUR must be dev or prod: $hint_contour"
  fi

  if [[ -n "$path_contour" && -n "$declared_contour" && "$path_contour" != "$declared_contour" ]]; then
    die "Env filename points to contour '$path_contour', but ESPO_CONTOUR=$declared_contour: $env_file"
  fi

  if [[ -n "$hint_contour" && -n "$declared_contour" && "$hint_contour" != "$declared_contour" ]]; then
    die "ESPO_ENV_FILE_CONTOUR=$hint_contour conflicts with ESPO_CONTOUR=$declared_contour: $env_file"
  fi

  if [[ -n "$hint_contour" && -n "$path_contour" && "$hint_contour" != "$path_contour" ]]; then
    die "ESPO_ENV_FILE_CONTOUR=$hint_contour conflicts with the env filename for contour '$path_contour': $env_file"
  fi

  effective_contour="${declared_contour:-${hint_contour:-$path_contour}}"
  [[ -n "$effective_contour" ]] \
    || die "Could not determine env file contour: $env_file. Add ESPO_CONTOUR=dev|prod to the env file or export ESPO_ENV_FILE_CONTOUR=dev|prod"

  if [[ "$effective_contour" != "$ESPO_ENV" ]]; then
    die "Env file '$env_file' belongs to contour '$effective_contour', but the command was run for '$ESPO_ENV'"
  fi

  RESOLVED_ENV_CONTOUR="$effective_contour"
  export RESOLVED_ENV_CONTOUR
}

clear_loaded_env_vars() {
  local env_var_name

  if [[ ${#LOADED_ENV_VARS[@]} -eq 0 ]]; then
    return 0
  fi

  for env_var_name in "${LOADED_ENV_VARS[@]}"; do
    unset "$env_var_name" 2>/dev/null || true
  done

  LOADED_ENV_VARS=()
}

env_parse_error() {
  local env_file="$1"
  local line_no="$2"
  local message="$3"

  die "Env file '$env_file' contains unsupported syntax on line $line_no: $message"
}

decode_double_quoted_env_value() {
  local raw_value="$1"
  local env_file="$2"
  local line_no="$3"
  local inner="" decoded="" char=""
  local index escape_next=0

  [[ ${#raw_value} -ge 2 && "${raw_value: -1}" == '"' ]] \
    || env_parse_error "$env_file" "$line_no" "double-quoted value must end with a closing quote"

  inner="${raw_value:1:${#raw_value}-2}"

  for (( index=0; index<${#inner}; index++ )); do
    char="${inner:index:1}"

    if [[ $escape_next -eq 1 ]]; then
      case "$char" in
        "\\"|'"'|'$'|'`')
          decoded+="$char"
          ;;
        *)
          env_parse_error "$env_file" "$line_no" "unsupported escape sequence \\$char"
          ;;
      esac
      escape_next=0
      continue
    fi

    case "$char" in
      "\\")
        escape_next=1
        ;;
      '"')
        env_parse_error "$env_file" "$line_no" "inner double quotes must be escaped"
        ;;
      '$'|'`')
        env_parse_error "$env_file" "$line_no" "double-quoted value must not contain unescaped shell expansions"
        ;;
      *)
        decoded+="$char"
        ;;
    esac
  done

  [[ $escape_next -eq 0 ]] \
    || env_parse_error "$env_file" "$line_no" "double-quoted value ends with an unfinished escape sequence"

  PARSED_ENV_VALUE="$decoded"
}

decode_single_quoted_env_value() {
  local raw_value="$1"
  local env_file="$2"
  local line_no="$3"
  local inner=""

  [[ ${#raw_value} -ge 2 && "${raw_value: -1}" == "'" ]] \
    || env_parse_error "$env_file" "$line_no" "single-quoted value must end with a closing quote"

  inner="${raw_value:1:${#raw_value}-2}"
  [[ "$inner" != *"'"* ]] \
    || env_parse_error "$env_file" "$line_no" "single-quoted value must not contain a raw single quote"

  PARSED_ENV_VALUE="$inner"
}

decode_unquoted_env_value() {
  local raw_value="$1"
  local env_file="$2"
  local line_no="$3"

  # shellcheck disable=SC2016
  case "$raw_value" in
    *'$('*|*'${'*|*'`'*)
      env_parse_error "$env_file" "$line_no" "value must not contain shell expansions"
      ;;
  esac

  if [[ "$raw_value" =~ [[:space:]] ]]; then
    env_parse_error "$env_file" "$line_no" "a value containing spaces must be quoted"
  fi

  PARSED_ENV_VALUE="$raw_value"
}

parse_env_value() {
  local raw_value="$1"
  local env_file="$2"
  local line_no="$3"

  case "$raw_value" in
    \"*)
      decode_double_quoted_env_value "$raw_value" "$env_file" "$line_no"
      ;;
    \'*)
      decode_single_quoted_env_value "$raw_value" "$env_file" "$line_no"
      ;;
    *)
      decode_unquoted_env_value "$raw_value" "$env_file" "$line_no"
      ;;
  esac
}

load_env_assignments() {
  local env_file="$1"
  local raw_line="" line="" env_key="" raw_value="" env_value=""
  local line_no=0

  # shellcheck disable=SC2094
  while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
    line_no=$((line_no + 1))
    line="${raw_line%$'\r'}"

    if [[ "$line" =~ ^[[:space:]]*$ ]] || [[ "$line" =~ ^[[:space:]]*# ]]; then
      continue
    fi

    if [[ ! "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      env_parse_error "$env_file" "$line_no" "expected a KEY=VALUE line without shell code"
    fi

    env_key="${BASH_REMATCH[1]}"
    raw_value="${BASH_REMATCH[2]}"
    PARSED_ENV_VALUE=""
    parse_env_value "$raw_value" "$env_file" "$line_no"
    env_value="$PARSED_ENV_VALUE"

    if ! export "$env_key=$env_value"; then
      env_parse_error "$env_file" "$line_no" "variable '$env_key' cannot be safely overridden"
    fi

    LOADED_ENV_VARS+=("$env_key")
  done < "$env_file" || die "Could not read env file: $env_file"
}

# Parse env files as strict dotenv data, never through shell execution.
load_env() {
  local env_file_contour_hint="${ESPO_ENV_FILE_CONTOUR:-}"

  clear_loaded_env_vars
  unset "${KNOWN_ENV_VARS[@]}" 2>/dev/null || true
  unset RESOLVED_ENV_CONTOUR 2>/dev/null || true
  validate_env_file_for_loading "$ENV_FILE"
  load_env_assignments "$ENV_FILE"

  : "${COMPOSE_PROJECT_NAME:?COMPOSE_PROJECT_NAME is not set in $ENV_FILE}"
  : "${DB_STORAGE_DIR:?DB_STORAGE_DIR is not set in $ENV_FILE}"
  : "${ESPO_STORAGE_DIR:?ESPO_STORAGE_DIR is not set in $ENV_FILE}"
  : "${BACKUP_ROOT:?BACKUP_ROOT is not set in $ENV_FILE}"

  validate_loaded_env_file_contour "$ENV_FILE" "$env_file_contour_hint"
}

root_path() {
  local path="$1"

  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$ROOT_DIR/${path#./}"
  fi
}

caller_path() {
  local path="$1"

  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$CALLER_DIR/${path#./}"
  fi
}

next_unique_stamp() {
  local stamp template path collision_found=0

  while true; do
    stamp="$(date +%F_%H-%M-%S)"

    if [[ $# -eq 0 ]]; then
      printf '%s\n' "$stamp"
      return 0
    fi

    collision_found=0
    for template in "$@"; do
      path="${template//__STAMP__/$stamp}"
      if [[ -e "$path" ]]; then
        collision_found=1
        break
      fi
    done

    if [[ $collision_found -eq 0 ]]; then
      printf '%s\n' "$stamp"
      return 0
    fi

    warn "Detected a name collision on second-level stamp '$stamp', waiting for the next free stamp"
    sleep 1
  done
}

render_env_value() {
  local value="$1"

  [[ "$value" != *$'\n'* ]] || die "env value must not contain a newline"
  [[ "$value" != *$'\r'* ]] || die "env value must not contain \\r"

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//\$/\\$}"
  value="${value//\`/\\\`}"

  printf '"%s"' "$value"
}

set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  local rendered_value tmp_file parent_dir base_name
  local line replaced=0

  rendered_value="$(render_env_value "$value")"
  parent_dir="$(dirname "$file")"
  base_name="$(basename "$file")"

  mkdir -p "$parent_dir"
  tmp_file="$(mktemp "$parent_dir/.env-edit.${base_name}.XXXXXX")"

  if [[ -f "$file" ]]; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      if [[ "$line" == "$key="* ]]; then
        if [[ $replaced -eq 0 ]]; then
          printf '%s=%s\n' "$key" "$rendered_value" >> "$tmp_file"
          replaced=1
        fi
        continue
      fi

      printf '%s\n' "$line" >> "$tmp_file"
    done < "$file" || {
      rm -f -- "$tmp_file"
      die "Could not update env file: $file"
    }

    if [[ $replaced -eq 0 ]]; then
      printf '%s=%s\n' "$key" "$rendered_value" >> "$tmp_file" \
        || { rm -f -- "$tmp_file"; die "Could not append to env file: $file"; }
    fi
  else
    printf '%s=%s\n' "$key" "$rendered_value" > "$tmp_file" \
      || { rm -f -- "$tmp_file"; die "Could not create env file: $file"; }
  fi

  mv -- "$tmp_file" "$file" || {
    rm -f -- "$tmp_file"
    die "Could not save env file: $file"
  }
}

ensure_runtime_dirs() {
  mkdir -p \
    "$(root_path "$DB_STORAGE_DIR")" \
    "$(root_path "$ESPO_STORAGE_DIR")" \
    "$(root_path "$BACKUP_ROOT")/db" \
    "$(root_path "$BACKUP_ROOT")/files" \
    "$(root_path "$BACKUP_ROOT")/locks" \
    "$(root_path "$BACKUP_ROOT")/manifests" \
    "$(root_path "$BACKUP_ROOT")/reports" \
    "$(root_path "$BACKUP_ROOT")/support"
}

print_context() {
  note "Contour: $ESPO_ENV"
  note "Env file: $ENV_FILE"
  note "Compose project: $COMPOSE_PROJECT_NAME"
}
