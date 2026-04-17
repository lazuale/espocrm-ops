#!/usr/bin/env bash
set -Eeuo pipefail

lock_info() {
  printf '[info] %s\n' "$*" >&2
}

lock_warn() {
  printf '[warn] %s\n' "$*" >&2
}

require_flock_command() {
  command_exists flock || die "flock not found. Sequential toolkit mode requires util-linux."
}

lock_owner_is_alive() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1

  if [[ -d "/proc/$pid" ]]; then
    return 0
  fi

  ps -p "$pid" >/dev/null 2>&1
}

lock_file_owner_pid() {
  local lock_file="$1"
  [[ -f "$lock_file" ]] || return 1
  head -n 1 "$lock_file" 2>/dev/null || true
}

lock_handle_file_for_lock_file() {
  local lock_file="$1"
  printf '%s/.%s.flock\n' "$(dirname "$lock_file")" "$(basename "$lock_file")"
}

alternate_lock_handle_file_for_lock_file() {
  local lock_file="$1"
  local operation_lock_file

  operation_lock_file="$(repo_operation_lock_file)"
  if [[ "$lock_file" == "$operation_lock_file" ]]; then
    repo_operation_lock_handle_file
    return 0
  fi

  return 1
}

lock_file_has_handle() {
  local lock_file="$1"
  local handle_file alternate_handle_file

  handle_file="$(lock_handle_file_for_lock_file "$lock_file")"
  [[ -e "$handle_file" ]] && return 0

  alternate_handle_file="$(alternate_lock_handle_file_for_lock_file "$lock_file" || true)"
  [[ -n "$alternate_handle_file" && -e "$alternate_handle_file" ]]
}

legacy_metadata_only_lock_detected() {
  local lock_file="$1"
  [[ -f "$lock_file" ]] || return 1
  lock_file_has_handle "$lock_file" && return 1
  return 0
}

die_unverified_legacy_lock() {
  local lock_file="$1"
  local lock_kind="$2"
  local pid

  pid="$(lock_file_owner_pid "$lock_file" || true)"
  if [[ -n "$pid" ]]; then
    die "Found a legacy lock file without a flock handle for $lock_kind (PID $pid): $lock_file. Its state can no longer be verified safely; remove it manually after verification"
  fi

  die "Found a legacy lock file without a flock handle for $lock_kind: $lock_file. Its state can no longer be verified safely; remove it manually after verification"
}

lock_handle_state() {
  local handle_file="$1"

  [[ -e "$handle_file" ]] || return 1

  if flock -n "$handle_file" true 2>/dev/null; then
    printf 'stale\n'
  else
    printf 'active\n'
  fi
}

lock_handle_state_for_lock_file() {
  local lock_file="$1"
  local handle_file alternate_handle_file handle_state

  handle_file="$(lock_handle_file_for_lock_file "$lock_file")"
  handle_state="$(lock_handle_state "$handle_file" || true)"
  if [[ -n "$handle_state" ]]; then
    printf '%s\n' "$handle_state"
    return 0
  fi

  alternate_handle_file="$(alternate_lock_handle_file_for_lock_file "$lock_file" || true)"
  if [[ -n "$alternate_handle_file" && "$alternate_handle_file" != "$handle_file" ]]; then
    handle_state="$(lock_handle_state "$alternate_handle_file" || true)"
    if [[ -n "$handle_state" ]]; then
      printf '%s\n' "$handle_state"
      return 0
    fi
  fi

  return 1
}

lock_file_state() {
  local lock_file="$1"
  local handle_state

  handle_state="$(lock_handle_state_for_lock_file "$lock_file" || true)"
  if [[ -n "$handle_state" ]]; then
    printf '%s\n' "$handle_state"
    return 0
  fi

  if legacy_metadata_only_lock_detected "$lock_file"; then
    printf 'legacy_unverified\n'
    return 0
  fi

  return 1
}

list_lock_files() {
  local directory="$1"
  [[ -d "$directory" ]] || return 0
  find "$directory" -maxdepth 1 -type f -name '*.lock' | sort
}

collect_lock_entries() {
  local locks_dir="$1"
  local lock_file pid state

  while IFS= read -r lock_file; do
    [[ -n "$lock_file" ]] || continue
    pid="$(lock_file_owner_pid "$lock_file" || true)"
    state="$(lock_file_state "$lock_file")"
    printf '%s|%s|%s|%s\n' "$(basename "$lock_file")" "$pid" "$state" "$lock_file"
  done < <(list_lock_files "$locks_dir")
}

repo_operation_lock_file() {
  local repo_hash=""
  local lock_root="${TMPDIR:-/tmp}/espocrm-toolkit-locks"

  mkdir -p "$lock_root"

  if command_exists sha256sum; then
    repo_hash="$(printf '%s' "$ROOT_DIR" | sha256sum | awk '{print $1}')"
  elif command_exists shasum; then
    repo_hash="$(printf '%s' "$ROOT_DIR" | shasum -a 256 | awk '{print $1}')"
  elif command_exists openssl; then
    repo_hash="$(printf '%s' "$ROOT_DIR" | openssl dgst -sha256 | awk '{print $NF}')"
  else
    repo_hash="$(printf '%s' "$ROOT_DIR" | cksum | awk '{print $1}')"
  fi

  printf '%s/repo-operation-%s.lock\n' "$lock_root" "${repo_hash:0:16}"
}

repo_operation_lock_handle_file() {
  local metadata_file
  metadata_file="$(repo_operation_lock_file)"
  printf '%s.flock\n' "${metadata_file%.lock}"
}

write_lock_metadata_file() {
  local lock_file="$1"
  shift

  local lock_dir lock_basename tmp_file

  lock_dir="$(dirname "$lock_file")"
  lock_basename="$(basename "$lock_file")"
  tmp_file="$(mktemp "$lock_dir/.${lock_basename}.tmp.XXXXXX")" \
    || die "Could not create a temporary metadata lock file: $lock_file"

  if ! printf '%s\n' "$@" > "$tmp_file"; then
    rm -f -- "$tmp_file"
    die "Could not write metadata for lock file: $lock_file"
  fi

  if ! mv -f -- "$tmp_file" "$lock_file"; then
    rm -f -- "$tmp_file"
    die "Could not update lock-file metadata atomically: $lock_file"
  fi
}

release_operation_lock() {
  if [[ ${OPERATION_LOCK_HELD:-0} -eq 1 ]]; then
    if [[ -n "${OPERATION_LOCK_FILE:-}" ]]; then
      rm -f -- "$OPERATION_LOCK_FILE"
    fi

    if [[ -n "${OPERATION_LOCK_FD:-}" ]]; then
      flock -u "$OPERATION_LOCK_FD" 2>/dev/null || true
      exec {OPERATION_LOCK_FD}>&-
      unset OPERATION_LOCK_FD
    fi

    unset ESPO_OPERATION_LOCK
    OPERATION_LOCK_HELD=0
  fi
}

acquire_operation_lock() {
  local scope="${1:-operation}"
  local existing_pid wait_logged=0

  require_flock_command

  if [[ "${ESPO_OPERATION_LOCK:-0}" == "1" ]]; then
    lock_info "Using the inherited shared operations lock for scope '$scope'"
    return 0
  fi

  OPERATION_LOCK_FILE="$(repo_operation_lock_file)"
  if legacy_metadata_only_lock_detected "$OPERATION_LOCK_FILE"; then
    die_unverified_legacy_lock "$OPERATION_LOCK_FILE" "the toolkit shared operations lock"
  fi

  exec {OPERATION_LOCK_FD}>>"$(repo_operation_lock_handle_file)"

  while true; do
    if flock -n "$OPERATION_LOCK_FD"; then
      break
    fi

    if [[ $wait_logged -eq 0 ]]; then
      existing_pid="$(lock_file_owner_pid "$OPERATION_LOCK_FILE" || true)"
      if [[ -n "$existing_pid" ]]; then
        lock_info "Detected another active toolkit operation (PID $existing_pid), waiting for the shared lock to be released"
      else
        lock_info "Detected another active toolkit operation, waiting for the shared lock to be released"
      fi
      wait_logged=1
    fi

    sleep 1
  done

  if [[ -f "$OPERATION_LOCK_FILE" ]]; then
    lock_warn "Detected leftovers from a previous unfinished toolkit operation, rewriting metadata: $OPERATION_LOCK_FILE"
  fi

  write_lock_metadata_file \
    "$OPERATION_LOCK_FILE" \
    "$$" \
    "$scope" \
    "$(date +%F_%H-%M-%S)" \
    "$ROOT_DIR"

  export ESPO_OPERATION_LOCK=1
  OPERATION_LOCK_HELD=1
  append_trap 'release_operation_lock' EXIT
  lock_info "Acquired shared operations lock '$scope': $OPERATION_LOCK_FILE"
}

release_maintenance_lock() {
  if [[ ${MAINTENANCE_LOCK_HELD:-0} -eq 1 ]]; then
    if [[ -n "${MAINTENANCE_LOCK_FILE:-}" ]]; then
      rm -f -- "$MAINTENANCE_LOCK_FILE"
    fi

    if [[ -n "${MAINTENANCE_LOCK_FD:-}" ]]; then
      flock -u "$MAINTENANCE_LOCK_FD" 2>/dev/null || true
      exec {MAINTENANCE_LOCK_FD}>&-
      unset MAINTENANCE_LOCK_FD
    fi

    unset ESPO_MAINTENANCE_LOCK
    MAINTENANCE_LOCK_HELD=0
  fi
}

# Reuse the inherited maintenance lock in child flows.
acquire_maintenance_lock() {
  local scope="${1:-maintenance}"
  local locks_dir lock_file existing_pid lock_state
  local maintenance_lock_handle_file

  require_flock_command

  if [[ "${ESPO_MAINTENANCE_LOCK:-0}" == "1" ]]; then
    lock_info "Using the inherited maintenance lock for contour '$ESPO_ENV'"
    return 0
  fi

  locks_dir="$(root_path "$BACKUP_ROOT")/locks"
  mkdir -p "$locks_dir"

  # Active locks block, stale locks are cleaned, legacy metadata-only locks fail closed.
  while IFS= read -r lock_file; do
    [[ -n "$lock_file" ]] || continue
    lock_state="$(lock_file_state "$lock_file")"

    case "$lock_state" in
      active)
        existing_pid="$(lock_file_owner_pid "$lock_file" || true)"
        if [[ -n "$existing_pid" ]]; then
          die "For contour '$ESPO_ENV' another maintenance operation is already running (PID $existing_pid): $lock_file"
        fi
        die "For contour '$ESPO_ENV' another maintenance operation is already running: $lock_file"
        ;;
      legacy_unverified)
        die_unverified_legacy_lock "$lock_file" "contour maintenance operation '$ESPO_ENV'"
        ;;
    esac

    lock_warn "Found a stale lock file, removing: $lock_file"
    rm -f -- "$lock_file"
  done < <(list_lock_files "$locks_dir")

  MAINTENANCE_LOCK_FILE="$locks_dir/maintenance.lock"
  maintenance_lock_handle_file="$(lock_handle_file_for_lock_file "$MAINTENANCE_LOCK_FILE")"
  exec {MAINTENANCE_LOCK_FD}>>"$maintenance_lock_handle_file"

  if ! flock -n "$MAINTENANCE_LOCK_FD"; then
    existing_pid="$(lock_file_owner_pid "$MAINTENANCE_LOCK_FILE" || true)"
    if [[ -n "$existing_pid" ]]; then
      die "For contour '$ESPO_ENV' another maintenance operation is already running (PID $existing_pid): $MAINTENANCE_LOCK_FILE"
    fi
    die "For contour '$ESPO_ENV' another maintenance operation is already running: $MAINTENANCE_LOCK_FILE"
  fi

  write_lock_metadata_file \
    "$MAINTENANCE_LOCK_FILE" \
    "$$" \
    "$scope" \
    "$(date +%F_%H-%M-%S)"

  export ESPO_MAINTENANCE_LOCK=1
  MAINTENANCE_LOCK_HELD=1
  append_trap 'release_maintenance_lock' EXIT
  lock_info "Acquired maintenance lock '$scope': $MAINTENANCE_LOCK_FILE"
}
