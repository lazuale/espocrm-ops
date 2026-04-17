# shellcheck shell=bash

TEST_TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/espo-regression.XXXXXX")"
ORIGINAL_ENV_DEV_EXISTS=0
ORIGINAL_ENV_PROD_EXISTS=0
declare -A ORIGINAL_REPO_FILE_EXISTS=()
declare -A ORIGINAL_REPO_FILE_BACKUPS=()
declare -a REPLACED_REPO_TARGETS=()

replace_repo_file() {
  local source="$1"
  local target="$2"
  local parent temp_file replaced_file=""

  parent="$(dirname "$target")"
  mkdir -p "$parent"
  temp_file="$(mktemp "$parent/.regression.$(basename "$target").XXXXXX")"
  cp -p "$source" "$temp_file"

  if [[ -e "$target" ]]; then
    replaced_file="$parent/.replaced.$(basename "$target").$$"
    mv "$target" "$replaced_file"
  fi

  mv "$temp_file" "$target"

  if [[ -n "$replaced_file" && -e "$replaced_file" ]]; then
    rm -f -- "$replaced_file"
  fi
}

preserve_repo_file_for_restore() {
  local target="$1"
  local backup_copy=""

  if [[ -n "${ORIGINAL_REPO_FILE_EXISTS[$target]+x}" ]]; then
    return 0
  fi

  if [[ -e "$target" ]]; then
    backup_copy="$(mktemp "$TEST_TMP_ROOT/repo.$(basename "$target").XXXXXX")"
    cp -p "$target" "$backup_copy"
    ORIGINAL_REPO_FILE_EXISTS["$target"]=1
    ORIGINAL_REPO_FILE_BACKUPS["$target"]="$backup_copy"
  else
    ORIGINAL_REPO_FILE_EXISTS["$target"]=0
    ORIGINAL_REPO_FILE_BACKUPS["$target"]=""
  fi

  REPLACED_REPO_TARGETS+=("$target")
}

replace_repo_file_temporarily() {
  local source="$1"
  local target="$2"

  preserve_repo_file_for_restore "$target"
  replace_repo_file "$source" "$target"
}

restore_replaced_repo_files() {
  local target

  for target in "${REPLACED_REPO_TARGETS[@]}"; do
    if [[ "${ORIGINAL_REPO_FILE_EXISTS[$target]:-0}" == "1" ]]; then
      replace_repo_file "${ORIGINAL_REPO_FILE_BACKUPS[$target]}" "$target"
    else
      rm -f -- "$target"
    fi

    unset "ORIGINAL_REPO_FILE_EXISTS[$target]"
    unset "ORIGINAL_REPO_FILE_BACKUPS[$target]"
  done

  REPLACED_REPO_TARGETS=()
}

backup_repo_env_files() {
  if [[ -f "$ROOT_DIR/.env.dev" ]]; then
    ORIGINAL_ENV_DEV_EXISTS=1
    cp "$ROOT_DIR/.env.dev" "$TEST_TMP_ROOT/original.env.dev"
  fi

  if [[ -f "$ROOT_DIR/.env.prod" ]]; then
    ORIGINAL_ENV_PROD_EXISTS=1
    cp "$ROOT_DIR/.env.prod" "$TEST_TMP_ROOT/original.env.prod"
  fi
}

restore_repo_env_files() {
  if [[ $ORIGINAL_ENV_DEV_EXISTS -eq 1 ]]; then
    replace_repo_file "$TEST_TMP_ROOT/original.env.dev" "$ROOT_DIR/.env.dev"
  else
    rm -f -- "$ROOT_DIR/.env.dev"
  fi

  if [[ $ORIGINAL_ENV_PROD_EXISTS -eq 1 ]]; then
    replace_repo_file "$TEST_TMP_ROOT/original.env.prod" "$ROOT_DIR/.env.prod"
  else
    rm -f -- "$ROOT_DIR/.env.prod"
  fi
}

cleanup_generated_repo_artifacts() {
  rm -rf -- \
    "$ROOT_DIR/backups/smoke" \
    "$ROOT_DIR/backups/restore-drill" \
    "$ROOT_DIR/storage/smoke" \
    "$ROOT_DIR/storage/restore-drill"
  rm -rf -- "$ROOT_DIR"/.support.*
  rm -f -- "$ROOT_DIR"/.env.smoke.*
  rm -f -- "$ROOT_DIR"/.env.restore-drill.*
}

cleanup() {
  restore_replaced_repo_files
  restore_repo_env_files
  cleanup_generated_repo_artifacts
  rm -rf -- "$TEST_TMP_ROOT"
}

trap 'cleanup' EXIT

fail_test() {
  echo "[error] $*" >&2
  exit 1
}

pass_test() {
  echo "[ok] $*"
}

announce_test() {
  echo
  echo "== $* =="
}

assert_file_contains() {
  local file="$1"
  local expected="$2"
  local label="$3"

  if ! rg -Fq -- "$expected" "$file"; then
    echo "[debug] File output $file:" >&2
    sed -n '1,220p' "$file" >&2 || true
    fail_test "$label"
  fi
}

assert_file_not_contains() {
  local file="$1"
  local unexpected="$2"
  local label="$3"

  if rg -Fq -- "$unexpected" "$file"; then
    echo "[debug] File output $file:" >&2
    sed -n '1,220p' "$file" >&2 || true
    fail_test "$label"
  fi
}

run_command_capture() {
  local output_file="$1"
  shift

  set +e
  "$@" >"$output_file" 2>&1
  local status=$?
  set -e
  return "$status"
}

copy_example_env() {
  local contour="$1"
  local target="$2"
  replace_repo_file "$ROOT_DIR/.env.${contour}.example" "$target"
  chmod 600 "$target"
}

prepare_repo_env_pair() {
  copy_example_env dev "$ROOT_DIR/.env.dev"
  copy_example_env prod "$ROOT_DIR/.env.prod"
}

create_db_backup() {
  local backup_root="$1"
  local prefix="$2"
  local stamp="$3"
  local content="$4"
  local file="$backup_root/db/${prefix}_${stamp}.sql.gz"

  mkdir -p "$backup_root/db"
  printf '%s\n' "$content" | gzip -n > "$file"
  write_sha256_sidecar "$file" "$(sha256_file "$file")"
}

create_files_backup() {
  local backup_root="$1"
  local prefix="$2"
  local stamp="$3"
  local content="$4"
  local file="$backup_root/files/${prefix}_files_${stamp}.tar.gz"
  local stage_root=""

  mkdir -p "$backup_root/files"
  stage_root="$(mktemp -d "$TEST_TMP_ROOT/files-backup.XXXXXX")"
  mkdir -p "$stage_root/storage"
  printf '%s\n' "$content" > "$stage_root/storage/payload.txt"
  tar -C "$stage_root" -czf "$file" storage
  rm -rf -- "$stage_root"
  write_sha256_sidecar "$file" "$(sha256_file "$file")"
}

create_manifest_pair() {
  local backup_root="$1"
  local prefix="$2"
  local stamp="$3"
  local contour="$4"
  local compose_project="$5"
  local created_at=""
  local created_at_time=""
  local db_name="${prefix}_${stamp}.sql.gz"
  local files_name="${prefix}_files_${stamp}.tar.gz"
  local db_sha=""
  local files_sha=""

  created_at_time="${stamp#*_}"
  created_at_time="${created_at_time//-/:}"
  created_at="${stamp%%_*}T${created_at_time}Z"

  if [[ -f "$backup_root/db/$db_name.sha256" ]]; then
    db_sha="$(read_sha256_sidecar "$backup_root/db/$db_name.sha256")"
  fi

  if [[ -f "$backup_root/files/$files_name.sha256" ]]; then
    files_sha="$(read_sha256_sidecar "$backup_root/files/$files_name.sha256")"
  fi

  mkdir -p "$backup_root/manifests"
  cat > "$backup_root/manifests/${prefix}_${stamp}.manifest.txt" <<EOF
created_at=$stamp
contour=$contour
compose_project=$compose_project
EOF

  cat > "$backup_root/manifests/${prefix}_${stamp}.manifest.json" <<EOF
{"version":1,"scope":"$contour","contour":"$contour","created_at":"$created_at","compose_project":"$compose_project","artifacts":{"db_backup":"$db_name","files_backup":"$files_name"},"checksums":{"db_backup":"$db_sha","files_backup":"$files_sha"}}
EOF
}

create_mock_echo_script() {
  local label="$1"
  local target_file="$2"

  cat > "$target_file" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail
echo "$label: \$*"
EOF
  chmod +x "$target_file"
}

create_mock_docker_cli() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  echo "permission denied while trying to connect to the docker API at unix:///var/run/docker.sock" >&2
  exit 1
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" config "* ]]; then
  cat <<'YAML'
services: {}
YAML
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up "* ]]; then
  echo "compose up must not run without daemon preflight" >&2
  exit 99
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_forbidden() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "docker must not be called in this scenario: $*" >&2
exit 97
EOF
  chmod +x "$docker_bin"
}

create_mock_ps_reports_pid_alive() {
  local target_dir="$1"
  local alive_pid="$2"
  local ps_bin="$target_dir/ps"

  mkdir -p "$target_dir"
  cat > "$ps_bin" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "\${1:-}" == "-p" && "\${2:-}" == "$alive_pid" ]]; then
  exit 0
fi

exit 1
EOF
  chmod +x "$ps_bin"
}

create_mock_docker_rollback_guard() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  echo "Server Version: mock"
  exit 0
fi

if [[ "${1:-}" == "version" ]]; then
  echo "Client: mock"
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  exit 1
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" config "* ]]; then
  cat <<'YAML'
services: {}
YAML
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" logs "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" stop "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up "* ]]; then
  echo "mock rollback stop after selection" >&2
  exit 99
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_restore_db_snapshot() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "healthy"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" --status running "* && "$args" == *" --services "* ]]; then
  printf 'db\nespocrm\n'
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" exec "* ]]; then
  echo "mock restore-db stop after snapshot" >&2
  exit 99
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_runtime_success() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "healthy"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" --status running "* && "$args" == *" --services "* ]]; then
  printf 'db\nespocrm\nespocrm-daemon\nespocrm-websocket\n'
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" down "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" stop "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" exec "* ]]; then
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_update_success() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "healthy"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" pull "* ]]; then
  echo "mock docker pull should not run" >&2
  exit 97
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_curl_success() {
  local target_dir="$1"
  local curl_bin="$target_dir/curl"

  mkdir -p "$target_dir"
  cat > "$curl_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
exit 0
EOF
  chmod +x "$curl_bin"
}

create_mock_curl_guard() {
  local target_dir="$1"
  local curl_bin="$target_dir/curl"

  mkdir -p "$target_dir"
  cat > "$curl_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock curl should not run" >&2
exit 97
EOF
  chmod +x "$curl_bin"
}

create_mock_time_control() {
  local target_dir="$1"
  local date_bin="$target_dir/date"
  local sleep_bin="$target_dir/sleep"

  mkdir -p "$target_dir"

  cat > "$date_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${1:-}" == "+%s" && $# -eq 1 ]]; then
  if [[ -n "${MOCK_TIME_FILE:-}" && -f "${MOCK_TIME_FILE:-}" ]]; then
    cat "$MOCK_TIME_FILE"
  else
    printf '0\n'
  fi
  exit 0
fi

PATH=/usr/bin:/bin exec date "$@"
EOF
  chmod +x "$date_bin"

  cat > "$sleep_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

[[ -n "${MOCK_TIME_FILE:-}" ]] || exit 96
[[ $# -eq 1 ]] || exit 95
[[ "$1" =~ ^[0-9]+$ ]] || exit 94

current_time=0
if [[ -f "$MOCK_TIME_FILE" ]]; then
  current_time="$(cat "$MOCK_TIME_FILE")"
fi

printf '%s\n' "$((current_time + $1))" > "$MOCK_TIME_FILE"
EOF
  chmod +x "$sleep_bin"
}

create_mock_docker_cleanup_plan_success() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "

if [[ "${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "${1:-}" == "system" && "${2:-}" == "df" ]]; then
  cat <<'EOF2'
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          0         0         0B        0B
Containers      0         0         0B        0B
Local Volumes   0         0         0B        0B
Build Cache     0         0         0B        0B
EOF2
  exit 0
fi

if [[ "${1:-}" == "container" && "$args" == *" ls "* && "$args" == *" -aq "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "image" && "$args" == *" ls "* && "$args" == *" -qa "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "image" && "$args" == *" ls "* && "$args" == *" -q "* && "$args" == *" --filter dangling=true "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "network" && "$args" == *" ls "* && "$args" == *" -q "* ]]; then
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_shared_timeout_budget() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

args=" $* "
state_dir="${MOCK_DOCKER_STATE_DIR:?}"
running_services_file="$state_dir/running-services"

append_running_service() {
  local service="$1"
  touch "$running_services_file"
  if ! grep -qx "$service" "$running_services_file" 2>/dev/null; then
    printf '%s\n' "$service" >> "$running_services_file"
  fi
}

if [[ "${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" version "* ]]; then
  echo "Docker Compose version v0.0.0-mock"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" --status running "* && "$args" == *" --services "* ]]; then
  if [[ -f "$running_services_file" ]]; then
    cat "$running_services_file"
  fi
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up -d db "* ]]; then
  append_running_service db
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" up -d "* ]]; then
  append_running_service db
  append_running_service espocrm
  append_running_service espocrm-daemon
  append_running_service espocrm-websocket
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" pull "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" stop "* ]]; then
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" down "* ]]; then
  : > "$running_services_file"
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  if [[ "$*" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${MOCK_DOCKER_HEALTH_MESSAGE:-mock health failure}"
    exit 0
  fi

  container_id="${*: -1}"
  service="${container_id#mock-}"
  status_file="$state_dir/${service}.statuses"
  index_file="$state_dir/${service}.index"
  status="healthy"
  index=0

  if [[ -f "$status_file" ]]; then
    mapfile -t statuses < "$status_file"

    if [[ -f "$index_file" ]]; then
      index="$(cat "$index_file")"
    fi

    if (( ${#statuses[@]} > 0 )); then
      if (( index >= ${#statuses[@]} )); then
        index=$((${#statuses[@]} - 1))
      fi

      status="${statuses[$index]}"

      if (( index < ${#statuses[@]} - 1 )); then
        printf '%s\n' "$((index + 1))" > "$index_file"
      fi
    fi
  fi

  printf '%s\n' "$status"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_docker_fs_helper() {
  local target_dir="$1"
  local docker_bin="$target_dir/docker"

  mkdir -p "$target_dir"
  cat > "$docker_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  exit 0
fi

if [[ "${1:-}" == "run" ]]; then
  full_args=" $* "
  cleanup_parent=""
  local_source=""
  local_target=""
  restore_archive=""
  restore_target=""
  espo_storage=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -v)
        case "$2" in
          *:/cleanup-parent)
            cleanup_parent="${2%%:/cleanup-parent}"
            ;;
          *:/archive-source:ro)
            local_source="${2%%:/archive-source:ro}"
            ;;
          *:/archive-target)
            local_target="${2%%:/archive-target}"
            ;;
          *:/restore-archive:ro)
            restore_archive="${2%%:/restore-archive:ro}"
            ;;
          *:/restore-target)
            restore_target="${2%%:/restore-target}"
            ;;
          *:/espo-storage)
            espo_storage="${2%%:/espo-storage}"
            ;;
        esac
        shift 2
        ;;
      -e|--entrypoint|--user)
        shift 2
        ;;
      --pull=never|--rm)
        shift
        ;;
      *)
        shift
        ;;
    esac
    done

  if [[ -n "$cleanup_parent" ]]; then
    [[ -n "${CLEANUP_BASENAME:-}" ]] || exit 90
    PATH=/usr/bin:/bin rm -rf -- "$cleanup_parent/$CLEANUP_BASENAME"
    exit 0
  fi

  if [[ -n "$local_source" || -n "$local_target" ]]; then
    [[ -n "$local_source" ]] || exit 97
    [[ -n "$local_target" ]] || exit 96
    [[ -n "${ARCHIVE_SOURCE_BASENAME:-}" ]] || exit 95
    [[ -n "${ARCHIVE_OUTPUT_BASENAME:-}" ]] || exit 94

    PATH=/usr/bin:/bin tar -C "$local_source" -czf "$local_target/$ARCHIVE_OUTPUT_BASENAME" "$ARCHIVE_SOURCE_BASENAME"
    exit 0
  fi

  if [[ -n "$espo_storage" ]]; then
    if [[ "${MOCK_DOCKER_FAIL_RECONCILE:-0}" == "1" ]]; then
      echo "mock reconcile failure" >&2
      exit 99
    fi

    if [[ -n "${MOCK_DOCKER_STATE_DIR:-}" ]]; then
      mkdir -p "$MOCK_DOCKER_STATE_DIR"
      printf '%s:%s\n' "${ESPO_RUNTIME_UID:-}" "${ESPO_RUNTIME_GID:-}" > "$MOCK_DOCKER_STATE_DIR/reconcile-owner"
    fi

    PATH=/usr/bin:/bin chmod 0755 "$espo_storage"
    for bootstrap_dir in data custom client upload; do
      if [[ -d "$espo_storage/$bootstrap_dir" ]]; then
        PATH=/usr/bin:/bin chmod 0755 "$espo_storage/$bootstrap_dir"
      fi
    done

    PATH=/usr/bin:/bin find "$espo_storage" -type d -exec chmod 0755 {} +

    for relative in data custom client/custom upload; do
      path="$espo_storage/$relative"
      if [[ -d "$path" ]]; then
        PATH=/usr/bin:/bin chmod 0775 "$path"
        PATH=/usr/bin:/bin find "$path" -type d -exec chmod 0775 {} +
        PATH=/usr/bin:/bin find "$path" -type f -exec chmod 0664 {} +
      fi
    done

    exit 0
  fi

  if [[ "$full_args" == *"/var/www/html"* ]]; then
    printf '%s:%s\n' "${MOCK_ESPO_RUNTIME_UID:-33}" "${MOCK_ESPO_RUNTIME_GID:-33}"
    exit 0
  fi

  [[ -n "$restore_archive" ]] || exit 93
  [[ -n "$restore_target" ]] || exit 92
  [[ -n "${RESTORE_ARCHIVE_BASENAME:-}" ]] || exit 91

  PATH=/usr/bin:/bin tar -C "$restore_target" -xzf "$restore_archive/$RESTORE_ARCHIVE_BASENAME"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
EOF
  chmod +x "$docker_bin"
}

create_mock_rm_failure() {
  local target_dir="$1"
  local rm_bin="$target_dir/rm"

  mkdir -p "$target_dir"
  cat > "$rm_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

last_arg="${*: -1}"
if [[ -n "${MOCK_RM_FAIL_PATH:-}" && "$last_arg" == "$MOCK_RM_FAIL_PATH" ]]; then
  printf '%s\n' "${MOCK_RM_FAIL_MESSAGE:-mock rm failure}" >&2
  exit 1
fi

PATH=/usr/bin:/bin exec rm "$@"
EOF
  chmod +x "$rm_bin"
}

create_mock_du_with_partial_failure() {
  local target_dir="$1"
  local du_bin="$target_dir/du"

  mkdir -p "$target_dir"
  cat > "$du_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

printf '42M\t%s\n' "${*: -1}"
exit 1
EOF
  chmod +x "$du_bin"
}

create_mock_tar_create_failure() {
  local target_dir="$1"
  local tar_bin="$target_dir/tar"

  mkdir -p "$target_dir"
  cat > "$tar_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ " $* " == *" -czf "* ]]; then
  echo "mock tar create failure" >&2
  exit 1
fi

PATH=/usr/bin:/bin exec tar "$@"
EOF
  chmod +x "$tar_bin"
}

create_mock_tar_extract_failure() {
  local target_dir="$1"
  local tar_bin="$target_dir/tar"

  mkdir -p "$target_dir"
  cat > "$tar_bin" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ " $* " == *" -xzf "* ]]; then
  echo "mock tar extract failure" >&2
  exit 1
fi

PATH=/usr/bin:/bin exec tar "$@"
EOF
  chmod +x "$tar_bin"
}
