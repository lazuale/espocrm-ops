# shellcheck shell=bash

test_env_leakage_is_blocked() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.missing-db"
  local output_file="$TEST_TMP_ROOT/env-leakage.out"

  copy_example_env dev "$env_file"
  sed -i '/^DB_NAME=/d' "$env_file"

  if run_command_capture "$output_file" env DB_NAME=leaked_from_parent ENV_FILE="$env_file" bash "$SCRIPT_DIR/doctor.sh" dev --json; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "DB_NAME is required" "runtime output"
  assert_file_not_contains "$output_file" "leaked_from_parent" "runtime output"
  pass_test "Regression case passed"
}

test_load_env_rejects_insecure_env_files() {
  announce_test "Regression case"

  local insecure_env_file="$TEST_TMP_ROOT/env.insecure-mode"
  local symlink_target="$TEST_TMP_ROOT/env.symlink-target"
  local symlink_env_file="$TEST_TMP_ROOT/env.symlink"
  local output_file="$TEST_TMP_ROOT/env-security.out"

  copy_example_env dev "$insecure_env_file"
  chmod 644 "$insecure_env_file"

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    validate_env_file_for_loading '$insecure_env_file'
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Env file must not be broader than 640 and must not have execute bits" "runtime output"

  copy_example_env dev "$symlink_target"
  ln -s "$symlink_target" "$symlink_env_file"

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    validate_env_file_for_loading '$symlink_env_file'
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Env file must not be a symlink" "runtime output"
  pass_test "Regression case passed"
}

test_load_env_rejects_contour_mismatch() {
  announce_test "Regression case"
  local env_file="$TEST_TMP_ROOT/env.contour-mismatch"
  local output_file="$TEST_TMP_ROOT/env-contour-mismatch.out"

  copy_example_env prod "$env_file"

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    ESPO_ENV=dev
    ENV_FILE='$env_file'
    resolve_env_file
    load_env
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "belongs to contour 'prod', but the command was run for 'dev'" "runtime output"
  pass_test "Regression case passed"
}

test_load_env_supports_legacy_contour_detection_by_filename() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.legacy.prod"
  local output_file="$TEST_TMP_ROOT/env-legacy-contour.out"

  copy_example_env prod "$env_file"
  sed -i '/^ESPO_CONTOUR=/d' "$env_file"

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    ESPO_ENV=prod
    ENV_FILE='$env_file'
    resolve_env_file
    load_env
    printf 'resolved=%s\n' \"\$RESOLVED_ENV_CONTOUR\"
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "resolved=prod" "runtime output"
  pass_test "Regression case passed"
}

test_load_env_rejects_shell_substitutions() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.shell-syntax"
  local output_file="$TEST_TMP_ROOT/env-shell-syntax.out"
  local touched_file="$TEST_TMP_ROOT/env-shell-syntax.touched"
  local key="" line=""
  local -a case_names=("command-subst" "variable-expansion" "backticks")
  local -a injected_lines=(
    "SITE_URL=\$(touch $touched_file)"
    "DB_PASSWORD=\${HOME}"
    "ADMIN_PASSWORD=\`whoami\`"
  )
  local index

  for index in "${!case_names[@]}"; do
    copy_example_env dev "$env_file"
    rm -f -- "$touched_file"
    line="${injected_lines[$index]}"
    key="${line%%=*}"
    sed -i "/^${key}=/d" "$env_file"
    printf '%s\n' "$line" >> "$env_file"

    if run_command_capture "$output_file" bash -lc "
      source '$SCRIPT_DIR/lib/common.sh'
      ESPO_ENV=dev
      ENV_FILE='$env_file'
      resolve_env_file
      load_env
    "; then
      fail_test "Regression case failed"
    fi

    assert_file_contains "$output_file" "shell expansions" "runtime output"
    [[ ! -e "$touched_file" ]] || fail_test "Regression case failed"
  done

  pass_test "Regression case passed"
}

test_maintenance_lock_is_global_per_contour() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.lock"
  local output_file="$TEST_TMP_ROOT/maintenance-lock.out"
  local holder_pid

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/lock-backups"

  (
    trap - EXIT
    export ENV_FILE="$env_file"
    export ESPO_ENV=dev
    resolve_env_file
    load_env
    ensure_runtime_dirs
    acquire_maintenance_lock backup
    sleep 3
  ) &
  holder_pid=$!

  sleep 1

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    ENV_FILE='$env_file'
    ESPO_ENV=dev
    resolve_env_file
    load_env
    ensure_runtime_dirs
    acquire_maintenance_lock restore-db
  "; then
    kill "$holder_pid" 2>/dev/null || true
    wait "$holder_pid" 2>/dev/null || true
    fail_test "Regression case failed"
  fi

  wait "$holder_pid"
  assert_file_contains "$output_file" "another maintenance operation is already running" "runtime output"
  pass_test "Regression case passed"
}

test_append_trap_preserves_all_registered_handlers() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/append-trap.out"

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    append_trap 'echo one' EXIT
    append_trap 'echo two' EXIT
    append_trap 'echo three' EXIT
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "one" "runtime output"
  assert_file_contains "$output_file" "two" "runtime output"
  assert_file_contains "$output_file" "three" "runtime output"
  pass_test "Regression case passed"
}

test_json_escape_escapes_control_chars() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/json-escape.out"

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    payload=\$'prefix\\x01\\x02suffix\\b\\t\\n\\f\\r\\x1f\"\\\\'
    printf 'escaped=%s\n' \"\$(json_escape \"\$payload\")\"
  "; then
    fail_test "Regression case failed"
  fi

  local expected_escape="escaped=prefix\\u0001\\u0002suffix\\b\\t\\n\\f\\r\\u001f\\\"\\\\"
  assert_file_contains "$output_file" "$expected_escape" "runtime output"
  pass_test "Regression case passed"
}

test_lock_owner_is_alive_uses_ps_fallback_without_proc_entry() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/lock-owner-alive.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-ps-lock-owner"
  local alive_pid="424242"

  create_mock_ps_reports_pid_alive "$mock_bin_dir" "$alive_pid"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:/usr/bin:/bin" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    lock_owner_is_alive $alive_pid
    printf 'alive=yes\n'
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "alive=yes" "runtime output"
  pass_test "Regression case passed"
}

test_collect_lock_entries_marks_legacy_metadata_lock_as_unverified() {
  announce_test "Regression case"

  local locks_dir="$TEST_TMP_ROOT/legacy-locks"
  local maintenance_lock="$locks_dir/maintenance.lock"
  local output_file="$TEST_TMP_ROOT/legacy-locks.out"

  mkdir -p "$locks_dir"
  cat > "$maintenance_lock" <<'EOF'
424242
backup
2026-04-13_12-00-00
EOF

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    collect_lock_entries '$locks_dir'
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "maintenance.lock|424242|legacy_unverified|" "runtime output"
  pass_test "Regression case passed"
}

test_operation_lock_rejects_unverified_legacy_metadata() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/operation-legacy-lock.out"
  local operation_lock_file
  local operation_lock_handle_file

  operation_lock_file="$(repo_operation_lock_file)"
  operation_lock_handle_file="$(repo_operation_lock_handle_file)"
  rm -f -- "$operation_lock_file" "$operation_lock_handle_file"

  cat > "$operation_lock_file" <<EOF
424242
backup
2026-04-13_12-00-00
$ROOT_DIR
EOF

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    acquire_operation_lock verify-backup
  "; then
    rm -f -- "$operation_lock_file" "$operation_lock_handle_file"
    fail_test "Regression case failed"
  fi

  rm -f -- "$operation_lock_file" "$operation_lock_handle_file"
  assert_file_contains "$output_file" "legacy lock file without a flock handle for the toolkit shared operations lock" "runtime output"
  pass_test "Regression case passed"
}

test_maintenance_lock_rejects_unverified_legacy_metadata() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.legacy-maintenance-lock"
  local output_file="$TEST_TMP_ROOT/maintenance-legacy-lock.out"
  local backup_root="$TEST_TMP_ROOT/legacy-maintenance-lock-backups"
  local locks_dir="$backup_root/locks"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"
  mkdir -p "$locks_dir"

  cat > "$locks_dir/maintenance.lock" <<'EOF'
424242
backup
2026-04-13_12-00-00
EOF

  if run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    ENV_FILE='$env_file'
    ESPO_ENV=dev
    resolve_env_file
    load_env
    ensure_runtime_dirs
    acquire_maintenance_lock restore-db
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "legacy lock file without a flock handle for contour maintenance operation 'dev'" "runtime output"
  pass_test "Regression case passed"
}

test_collect_lock_entries_detects_stale_flock_metadata() {
  announce_test "Regression case"

  local locks_dir="$TEST_TMP_ROOT/stale-locks"
  local maintenance_lock="$locks_dir/maintenance.lock"
  local output_file="$TEST_TMP_ROOT/stale-locks.out"

  mkdir -p "$locks_dir"
  cat > "$maintenance_lock" <<'EOF'
424242
backup
2026-04-13_12-00-00
EOF
  : > "$locks_dir/.maintenance.lock.flock"

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    collect_lock_entries '$locks_dir'
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "maintenance.lock|424242|stale|" "runtime output"
  pass_test "Regression case passed"
}

test_operation_lock_serializes_toolkit_steps() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/operation-lock.out"
  local holder_pid elapsed

  (
    trap - EXIT
    # shellcheck disable=SC1091
    source "$SCRIPT_DIR/lib/common.sh"
    # shellcheck disable=SC1091
    source "$SCRIPT_DIR/lib/locks.sh"
    acquire_operation_lock backup
    sleep 3
  ) &
  holder_pid=$!

  sleep 1

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/locks.sh'
    start_epoch=\$(date +%s)
    acquire_operation_lock verify-backup
    end_epoch=\$(date +%s)
    echo elapsed=\$((end_epoch - start_epoch))
  "; then
    kill "$holder_pid" 2>/dev/null || true
    wait "$holder_pid" 2>/dev/null || true
    fail_test "Regression case failed"
  fi

  wait "$holder_pid"
  assert_file_contains "$output_file" "Detected another active toolkit operation" "runtime output"
  elapsed="$(awk -F= '/^elapsed=/{print $2}' "$output_file" | tail -n 1)"
  [[ "$elapsed" =~ ^[0-9]+$ ]] || fail_test "Regression case failed"
  (( elapsed >= 1 )) || fail_test "Regression case failed"
  pass_test "Regression case passed"
}

test_backup_audit_requires_coherent_set() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/audit-backups"
  local env_file="$TEST_TMP_ROOT/env.audit"
  local output_file="$TEST_TMP_ROOT/backup-audit.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-latest-only"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_00-00-00" "files-older"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_00-00-00" "dev" "espo-dev"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-audit.sh" dev --json; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"success\": false" "runtime output"
  assert_file_contains "$output_file" "Expected file from the selected backup set was not found" "runtime output"
  pass_test "Regression case passed"
}

test_backup_audit_can_skip_checksum_verification() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/backup-audit-skip-checksum"
  local env_file="$TEST_TMP_ROOT/env.backup-audit-skip-checksum"
  local output_file="$TEST_TMP_ROOT/backup-audit-skip-checksum.out"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-audit"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-audit"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "audit-dev"
  printf 'wrong-checksum\n' > "$backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz.sha256"
  printf 'wrong-checksum\n' > "$backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz.sha256"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-audit.sh" dev --json --no-verify-checksum; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"success\": true" "runtime output"
  assert_file_contains "$output_file" "Checksum verification skipped because of --no-verify-checksum" "runtime output"
  pass_test "Regression case passed"
}

test_backup_audit_supports_scope_switches_and_age_overrides() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/backup-audit-scope"
  local env_file="$TEST_TMP_ROOT/env.backup-audit-scope"
  local output_file="$TEST_TMP_ROOT/backup-audit-scope.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-scope"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-scope"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "scope-dev"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-audit.sh" dev --json --skip-db --max-files-age-hours 72; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"success\": true" "runtime output"
  assert_file_contains "$output_file" "\"status\": \"skipped\"" "runtime output"
  assert_file_contains "$output_file" "\"max_age_hours\": 72" "runtime output"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-audit.sh" dev --json --skip-files --max-db-age-hours 24; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"success\": true" "runtime output"
  assert_file_contains "$output_file" "Check skipped because of --skip-files" "runtime output"
  assert_file_contains "$output_file" "\"max_age_hours\": 24" "runtime output"
  pass_test "Regression case passed"
}

test_verify_backup_selects_coherent_pair() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/verify-backup-root"
  local env_file="$TEST_TMP_ROOT/env.verify-backup"
  local output_file="$TEST_TMP_ROOT/verify-backup.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-complete"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-complete"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "verify-dev"
  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "db-newer-without-files"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/verify-backup.sh" dev; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Database backup: $backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz" "runtime output"
  assert_file_contains "$output_file" "Files backup: $backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz" "runtime output"
  assert_file_not_contains "$output_file" "$backup_root/db/espocrm-dev_2026-04-07_02-00-00.sql.gz" "runtime output"
  pass_test "Regression case passed"
}

test_verify_backup_supports_partial_explicit_selection() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/verify-backup-partial-root"
  local env_file="$TEST_TMP_ROOT/env.verify-backup-partial"
  local output_file="$TEST_TMP_ROOT/verify-backup-partial.out"
  local db_backup="$backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz"
  local files_backup="$backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-partial"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-partial"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/verify-backup.sh" dev --skip-files --db-backup "$db_backup"; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "[ok] Database: checksum verified" "runtime output"
  assert_file_not_contains "$output_file" "runtime output" "runtime output"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/verify-backup.sh" dev --skip-db --files-backup "$files_backup"; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "[ok] Files: checksum verified" "runtime output"
  assert_file_not_contains "$output_file" "runtime output" "runtime output"
  pass_test "Regression case passed"
}

test_backup_catalog_ready_only_selects_verified_set() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/catalog-backups"
  local env_file="$TEST_TMP_ROOT/env.catalog"
  local output_file="$TEST_TMP_ROOT/backup-catalog.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-complete"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-complete"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "espo-dev"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "db-incomplete-newer"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_03-00-00" "db-corrupted-newest"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_03-00-00" "files-corrupted-newest"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_03-00-00" "dev" "espo-dev"
  printf '%s\n' 'files-corrupted-after-sidecar' > "$backup_root/files/espocrm-dev_files_2026-04-07_03-00-00.tar.gz"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-catalog.sh" dev --ready-only --latest-only --verify-checksum; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Total sets:        3" "runtime output"
  assert_file_contains "$output_file" "Shown sets:        1" "runtime output"
  assert_file_contains "$output_file" "[1] 2026-04-07_01-00-00 | prefix=espocrm-dev | readiness=ready, checksums verified" "runtime output"
  assert_file_not_contains "$output_file" "2026-04-07_03-00-00 | prefix=espocrm-dev | readiness=corrupted" "runtime output"
  pass_test "Regression case passed"
}

test_backup_catalog_supports_json_limit_and_latest_only() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/backup-catalog-json-root"
  local env_file="$TEST_TMP_ROOT/env.backup-catalog-json"
  local output_file="$TEST_TMP_ROOT/backup-catalog-json.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-older"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-older"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "catalog-json-old"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "db-latest"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "files-latest"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "dev" "catalog-json-new"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-catalog.sh" dev --json --latest-only --verify-checksum; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"total_sets\": 2" "runtime output"
  assert_file_contains "$output_file" "\"stamp\": \"2026-04-07_02-00-00\"" "runtime output"
  assert_file_not_contains "$output_file" "\"stamp\": \"2026-04-07_01-00-00\"" "runtime output"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup-catalog.sh" dev --json --limit 1 --ready-only; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "\"limit\": 1" "runtime output"
  assert_file_contains "$output_file" "\"restore_readiness\": \"ready_unverified\"" "runtime output"
  pass_test "Regression case passed"
}

test_directory_size_human_tolerates_du_errors() {
  announce_test "Human-Readable Directory Size Tolerates du Errors"

  local mock_bin_dir="$TEST_TMP_ROOT/mock-du-bin"
  local output_file="$TEST_TMP_ROOT/directory-size-human.out"
  local target_dir="$TEST_TMP_ROOT/directory-size-human-target"

  mkdir -p "$target_dir"
  create_mock_du_with_partial_failure "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    source '$SCRIPT_DIR/lib/artifacts.sh'
    result=\"\$(directory_size_human '$target_dir')\"
    printf 'result=%s\n' \"\$result\"
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "result=42M" "runtime output"
  pass_test "Regression case passed"
}

test_safe_remove_tree_uses_docker_fallback_for_permission_errors() {
  announce_test "Regression case"

  local mock_bin_dir="$TEST_TMP_ROOT/mock-safe-remove-permission-bin"
  local output_file="$TEST_TMP_ROOT/safe-remove-permission.out"
  local target_dir="$TEST_TMP_ROOT/safe-remove-permission-target"

  mkdir -p "$target_dir/nested"
  printf 'remove-me\n' > "$target_dir/nested/file.txt"

  create_mock_docker_fs_helper "$mock_bin_dir"
  create_mock_rm_failure "$mock_bin_dir"

  if ! run_command_capture "$output_file" env \
    PATH="$mock_bin_dir:$PATH" \
    MOCK_RM_FAIL_PATH="$target_dir" \
    MOCK_RM_FAIL_MESSAGE="rm: cannot remove '$target_dir/nested/file.txt': Permission denied" \
    bash -lc "
      set -Eeuo pipefail
      source '$SCRIPT_DIR/lib/common.sh'
      source '$SCRIPT_DIR/lib/fs.sh'
      safe_remove_tree '$target_dir' '$TEST_TMP_ROOT'
    "; then
    fail_test "Regression case failed"
  fi

  [[ ! -e "$target_dir" ]] || fail_test "Regression case failed"
  assert_file_contains "$output_file" "permission/ownership scenario" "runtime output"
  pass_test "Regression case passed"
}

test_safe_remove_tree_rejects_non_permission_failures() {
  announce_test "Regression case"

  local mock_bin_dir="$TEST_TMP_ROOT/mock-safe-remove-generic-bin"
  local output_file="$TEST_TMP_ROOT/safe-remove-generic.out"
  local target_dir="$TEST_TMP_ROOT/safe-remove-generic-target"

  mkdir -p "$target_dir/nested"
  printf 'keep-me\n' > "$target_dir/nested/file.txt"

  create_mock_docker_forbidden "$mock_bin_dir"
  create_mock_rm_failure "$mock_bin_dir"

  if run_command_capture "$output_file" env \
    PATH="$mock_bin_dir:$PATH" \
    MOCK_RM_FAIL_PATH="$target_dir" \
    MOCK_RM_FAIL_MESSAGE="rm: cannot remove '$target_dir': Input/output error" \
    bash -lc "
      set -Eeuo pipefail
      source '$SCRIPT_DIR/lib/common.sh'
      source '$SCRIPT_DIR/lib/fs.sh'
      safe_remove_tree '$target_dir' '$TEST_TMP_ROOT'
    "; then
    fail_test "Regression case failed"
  fi

  [[ -d "$target_dir" ]] || fail_test "Regression case failed"
  assert_file_contains "$output_file" "without signs of a permission/ownership scenario" "runtime output"
  assert_file_not_contains "$output_file" "docker must not be called" "runtime output"
  pass_test "Regression case passed"
}

test_set_env_value_preserves_special_chars() {
  announce_test "Regression case"
  local env_file="$TEST_TMP_ROOT/env.special-values"
  local load_output="$TEST_TMP_ROOT/set-env-value.load.out"
  local compose_output="$TEST_TMP_ROOT/set-env-value.compose.out"
  local expected_site='http://127.0.0.1:8088/?a=1&b=two|three\four'
  local expected_password=$'o\'clock pa$$w`rd\\tail "x" &|'
  local expected_site_b64 expected_password_b64

  copy_example_env dev "$env_file"
  set_env_value "$env_file" SITE_URL "$expected_site"
  set_env_value "$env_file" DB_PASSWORD "$expected_password"

  expected_site_b64="$(python3 -c 'import base64, sys; print(base64.b64encode(sys.argv[1].encode()).decode())' "$expected_site")"
  expected_password_b64="$(python3 -c 'import base64, sys; print(base64.b64encode(sys.argv[1].encode()).decode())' "$expected_password")"

  if ! run_command_capture "$load_output" bash -lc "
    set -Eeuo pipefail
    source '$SCRIPT_DIR/lib/common.sh'
    ESPO_ENV=dev
    ENV_FILE='$env_file'
    resolve_env_file
    load_env
    python3 - <<'PY'
import base64
import os

print('site_b64=' + base64.b64encode(os.environ['SITE_URL'].encode()).decode())
print('password_b64=' + base64.b64encode(os.environ['DB_PASSWORD'].encode()).decode())
PY
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$load_output" "site_b64=$expected_site_b64" "runtime output"
  assert_file_contains "$load_output" "password_b64=$expected_password_b64" "runtime output"

  if ! run_command_capture "$compose_output" docker compose --env-file "$env_file" -f "$ROOT_DIR/compose.yaml" config; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$compose_output" "$expected_site" "runtime output"
  pass_test "Regression case passed"
}

test_next_unique_stamp_waits_past_busy_second() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/next-unique-stamp.out"
  local stamp_now stamp_next stamp_after_next stamp_fourth
  local blocked_now blocked_next blocked_after_next blocked_fourth

  stamp_now="$(date +%F_%H-%M-%S)"
  stamp_next="$(date -d '+1 second' +%F_%H-%M-%S)"
  stamp_after_next="$(date -d '+2 seconds' +%F_%H-%M-%S)"
  stamp_fourth="$(date -d '+3 seconds' +%F_%H-%M-%S)"
  blocked_now="$TEST_TMP_ROOT/stamp_${stamp_now}.txt"
  blocked_next="$TEST_TMP_ROOT/stamp_${stamp_next}.txt"
  blocked_after_next="$TEST_TMP_ROOT/stamp_${stamp_after_next}.txt"
  blocked_fourth="$TEST_TMP_ROOT/stamp_${stamp_fourth}.txt"

  : > "$blocked_now"
  : > "$blocked_next"
  : > "$blocked_after_next"
  : > "$blocked_fourth"

  if ! run_command_capture "$output_file" bash -lc "
    source '$SCRIPT_DIR/lib/common.sh'
    stamp=\$(next_unique_stamp '$TEST_TMP_ROOT/stamp___STAMP__.txt')
    echo stamp=\$stamp
  "; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Detected a name collision on second-level stamp" "runtime output"
  assert_file_not_contains "$output_file" "stamp=$stamp_now" "runtime output"
  assert_file_not_contains "$output_file" "stamp=$stamp_next" "runtime output"
  assert_file_not_contains "$output_file" "stamp=$stamp_after_next" "runtime output"
  assert_file_not_contains "$output_file" "stamp=$stamp_fourth" "runtime output"
  pass_test "Regression case passed"
}

test_backup_releases_maintenance_lock() {
  announce_test "Regression case"
  local env_file="$TEST_TMP_ROOT/env.backup-lock"
  local output_file="$TEST_TMP_ROOT/backup-lock.out"
  local storage_root="$TEST_TMP_ROOT/backup-lock-storage"
  local storage_dir="$storage_root/espo"
  local backup_root="$TEST_TMP_ROOT/backup-lock-backups"

  mkdir -p "$storage_dir"
  printf 'backup-probe\n' > "$storage_dir/probe.txt"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$storage_root/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup.sh" dev --skip-db --no-stop; then
    fail_test "Regression case failed"
  fi

  [[ ! -e "$backup_root/locks/maintenance.lock" ]] || fail_test "Regression case failed"
  pass_test "Regression case passed"
}

test_backup_rejects_empty_selection() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/backup-empty-selection.out"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/backup.sh" dev --skip-db --skip-files; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Nothing to back up: --skip-db and --skip-files cannot both be set" "runtime output"
  pass_test "Regression case passed"
}

test_backup_files_only_no_stop_runs_without_docker() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.backup-files-only"
  local output_file="$TEST_TMP_ROOT/backup-files-only.out"
  local backup_root="$TEST_TMP_ROOT/backup-files-only-backups"
  local storage_root="$TEST_TMP_ROOT/backup-files-only-storage/espo"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-forbidden"
  local files_backup=""

  copy_example_env dev "$env_file"
  set_env_value "$env_file" COMPOSE_PROJECT_NAME files-only-dev
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/backup-files-only-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_root"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"
  set_env_value "$env_file" BACKUP_NAME_PREFIX "espocrm-files-only"

  mkdir -p "$storage_root/data"
  printf 'files-only-content\n' > "$storage_root/data/file.txt"

  create_mock_docker_forbidden "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup.sh" dev --skip-db --no-stop; then
    fail_test "Regression case failed"
  fi

  files_backup="$(latest_backup_file "$backup_root/files" '*.tar.gz' || true)"
  [[ -n "$files_backup" && -f "$files_backup" ]] || fail_test "Regression case failed"

  if find "$backup_root/db" -maxdepth 1 -type f -name '*.sql.gz' -print -quit | grep -q .; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Backup is being created without stopping application services" "runtime output"
  assert_file_not_contains "$output_file" "docker must not be called" "runtime output"
  pass_test "Regression case passed"
}

test_backup_files_fallbacks_to_docker_helper() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.backup-files-fallback"
  local output_file="$TEST_TMP_ROOT/backup-files-fallback.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-backup-fallback-bin"
  local storage_root="$TEST_TMP_ROOT/backup-files-fallback-storage"
  local storage_dir="$storage_root/espo"
  local backup_root="$TEST_TMP_ROOT/backup-files-fallback-backups"
  local archive_file

  mkdir -p "$storage_dir"
  printf 'fallback-test\n' > "$storage_dir/file.txt"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$storage_root/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  create_mock_docker_fs_helper "$mock_bin_dir"
  create_mock_tar_create_failure "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/backup.sh" dev --skip-db --no-stop; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Local archiving failed, trying Docker fallback" "runtime output"
  archive_file="$(find "$backup_root/files" -maxdepth 1 -type f -name '*.tar.gz' | head -n 1)"
  [[ -n "$archive_file" && -f "$archive_file" ]] || fail_test "Regression case failed"

  if ! PATH=/usr/bin:/bin tar -tzf "$archive_file" | grep -qx 'espo/file.txt'; then
    fail_test "Regression case failed"
  fi

  pass_test "Regression case passed"
}

test_restore_files_supports_absolute_storage_path() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files"
  local output_file="$TEST_TMP_ROOT/restore-files.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-restore-files-absolute-bin"
  local storage_dir="$TEST_TMP_ROOT/absolute-espo-storage"
  local source_parent="$TEST_TMP_ROOT/archive-src"
  local archive_file="$TEST_TMP_ROOT/restore-files.tar.gz"
  local storage_basename

  storage_basename="$(basename "$storage_dir")"
  mkdir -p "$source_parent/$storage_basename"
  printf 'restored\n' > "$source_parent/$storage_basename/file.txt"
  tar -C "$source_parent" -czf "$archive_file" "$storage_basename"
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/restore-files-backups"

  create_mock_docker_fs_helper "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  [[ -f "$storage_dir/file.txt" ]] || fail_test "Regression case failed"
  pass_test "Regression case passed"
}

test_restore_files_reconciles_permissions_after_replace() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files-permissions"
  local output_file="$TEST_TMP_ROOT/restore-files-permissions.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-restore-files-permissions-bin"
  local mock_state_dir="$TEST_TMP_ROOT/mock-restore-files-permissions-state"
  local storage_dir="$TEST_TMP_ROOT/restore-files-permissions-storage/espo"
  local source_parent="$TEST_TMP_ROOT/restore-files-permissions-source"
  local archive_file="$TEST_TMP_ROOT/restore-files-permissions.tar.gz"
  local storage_basename
  local current_uid current_gid

  current_uid="$(id -u)"
  current_gid="$(id -g)"
  storage_basename="$(basename "$storage_dir")"

  mkdir -p \
    "$source_parent/$storage_basename/data/nested" \
    "$source_parent/$storage_basename/custom/modules" \
    "$source_parent/$storage_basename/client/custom" \
    "$source_parent/$storage_basename/upload/tmp"
  printf 'restored-data\n' > "$source_parent/$storage_basename/data/nested/file.txt"
  printf 'restored-custom\n' > "$source_parent/$storage_basename/custom/modules/module.txt"
  printf 'restored-client-custom\n' > "$source_parent/$storage_basename/client/custom/app.js"
  printf 'restored-upload\n' > "$source_parent/$storage_basename/upload/tmp/blob.txt"

  tar --mode='u=rw,g=rw,o=r' -C "$source_parent" -czf "$archive_file" "$storage_basename"
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/restore-files-permissions-backups"

  create_mock_docker_fs_helper "$mock_bin_dir"

  if ! run_command_capture "$output_file" env \
    PATH="$mock_bin_dir:$PATH" \
    MOCK_DOCKER_STATE_DIR="$mock_state_dir" \
    MOCK_ESPO_RUNTIME_UID="$current_uid" \
    MOCK_ESPO_RUNTIME_GID="$current_gid" \
    ENV_FILE="$env_file" \
    bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Aligning owner and mode of the restored tree with the image runtime contract" "runtime output"
  [[ -f "$storage_dir/data/nested/file.txt" ]] || fail_test "Regression case failed"
  [[ -f "$storage_dir/client/custom/app.js" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir")" == "755" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/client")" == "755" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/data")" == "775" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/custom")" == "775" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/client/custom")" == "775" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/upload")" == "775" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/data/nested/file.txt")" == "664" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/custom/modules/module.txt")" == "664" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/client/custom/app.js")" == "664" ]] || fail_test "Regression case failed"
  [[ "$(stat -c '%a' "$storage_dir/upload/tmp/blob.txt")" == "664" ]] || fail_test "Regression case failed"
  [[ -x "$storage_dir/data" ]] || fail_test "Regression case failed"
  [[ -x "$storage_dir/client/custom" ]] || fail_test "Regression case failed"
  [[ -f "$mock_state_dir/reconcile-owner" ]] || fail_test "Regression case failed"
  [[ "$(cat "$mock_state_dir/reconcile-owner")" == "$current_uid:$current_gid" ]] || fail_test "Regression case failed"
  pass_test "Regression case passed"
}

test_restore_files_fails_when_permission_reconcile_fails() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files-reconcile-fail"
  local output_file="$TEST_TMP_ROOT/restore-files-reconcile-fail.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-restore-files-reconcile-fail-bin"
  local storage_dir="$TEST_TMP_ROOT/restore-files-reconcile-fail-storage/espo"
  local source_parent="$TEST_TMP_ROOT/restore-files-reconcile-fail-source"
  local archive_file="$TEST_TMP_ROOT/restore-files-reconcile-fail.tar.gz"
  local storage_basename

  storage_basename="$(basename "$storage_dir")"
  mkdir -p "$source_parent/$storage_basename/data"
  printf 'restored\n' > "$source_parent/$storage_basename/data/file.txt"
  tar -C "$source_parent" -czf "$archive_file" "$storage_basename"
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/restore-files-reconcile-fail-backups"

  create_mock_docker_fs_helper "$mock_bin_dir"

  if run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" MOCK_DOCKER_FAIL_RECONCILE=1 ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Could not align the restored tree permissions with runtime-image expectations" "runtime output"
  assert_file_not_contains "$output_file" "Files restore completed" "runtime output"
  pass_test "Regression case passed"
}

test_restore_files_fallbacks_to_docker_helper() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files-fallback"
  local output_file="$TEST_TMP_ROOT/restore-files-fallback.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-restore-fallback-bin"
  local storage_dir="$TEST_TMP_ROOT/restore-fallback-storage/espo"
  local source_parent="$TEST_TMP_ROOT/restore-fallback-archive-src"
  local archive_file="$TEST_TMP_ROOT/restore-files-fallback.tar.gz"
  local storage_basename

  storage_basename="$(basename "$storage_dir")"
  mkdir -p "$source_parent/$storage_basename" "$(dirname "$storage_dir")"
  printf 'restored-via-docker-fallback\n' > "$source_parent/$storage_basename/file.txt"

  tar -C "$source_parent" -czf "$archive_file" "$storage_basename"
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/restore-files-fallback-backups"

  create_mock_docker_fs_helper "$mock_bin_dir"
  create_mock_tar_extract_failure "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Files restore via the Go backend" "runtime output"
  assert_file_not_contains "$output_file" "runtime output" "runtime output"
  [[ -f "$storage_dir/file.txt" ]] || fail_test "Regression case failed"
  if [[ "$(cat "$storage_dir/file.txt")" != "restored-via-docker-fallback" ]]; then
    fail_test "Regression case failed"
  fi

  pass_test "Regression case passed"
}

test_restore_files_rejects_unsafe_archive_layout() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files-unsafe"
  local output_file="$TEST_TMP_ROOT/restore-files-unsafe.out"
  local target_root="$TEST_TMP_ROOT/unsafe-target/storage"
  local payload_dir="$TEST_TMP_ROOT/unsafe-payload"
  local escaped_dir="$TEST_TMP_ROOT/escaped-parent"
  local archive_file="$TEST_TMP_ROOT/unsafe-restore-files.tar.gz"

  mkdir -p "$payload_dir" "$escaped_dir"
  printf 'unsafe\n' > "$escaped_dir/owned.txt"
  tar -C "$payload_dir" -czf "$archive_file" ../escaped-parent/owned.txt
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$target_root"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/restore-files-unsafe-backups"

  if run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "archive root must be exactly" "runtime output"
  [[ ! -e "$(dirname "$target_root")/escaped-parent/owned.txt" ]] || fail_test "Regression case failed"
  pass_test "Regression case passed"
}

test_restore_db_takes_snapshot_by_default() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-db-snapshot"
  local output_file="$TEST_TMP_ROOT/restore-db-snapshot.out"
  local backup_root="$TEST_TMP_ROOT/restore-db-snapshot-backups"
  local db_backup="$TEST_TMP_ROOT/restore-db-snapshot.sql.gz"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-restore-db-snapshot"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.restore-db.sh"
  local plan_file=""

  restore_replaced_repo_files
  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/restore-db-snapshot-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/restore-db-snapshot-storage/espo"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  printf 'restore-db-snapshot\n' > "$db_backup"
  write_sha256_sidecar "$db_backup" "$(sha256_file "$db_backup")"

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock backup args: $*"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  create_mock_docker_restore_db_snapshot "$mock_bin_dir"

  if run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-db.sh" dev "$db_backup" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "The pre-restore emergency snapshot will be created without stopping application services" "runtime output"
  assert_file_contains "$output_file" "Creating an emergency snapshot before the database restore" "runtime output"
  assert_file_contains "$output_file" "mock backup args: dev --skip-files --no-stop" "runtime output"
  plan_file="$(find "$backup_root/reports" -maxdepth 1 -type f -name '*_restore-db-plan_*.txt' | sort | tail -n 1)"
  [[ -n "$plan_file" && -f "$plan_file" ]] || fail_test "Regression case failed"
  assert_file_contains "$plan_file" "snapshot_enabled=true" "runtime output"
  assert_file_contains "$plan_file" "no_stop=true" "runtime output"
  assert_file_contains "$plan_file" "no_start=true" "runtime output"
  assert_file_contains "$plan_file" "db_backup=$db_backup" "runtime output"
  assert_file_contains "$output_file" "Database restore via the Go backend" "runtime output"
  assert_file_contains "$output_file" "db backup gzip validation failed" "runtime output"
  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_restore_files_takes_snapshot_by_default() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.restore-files-snapshot"
  local output_file="$TEST_TMP_ROOT/restore-files-snapshot.out"
  local storage_dir="$TEST_TMP_ROOT/restore-files-snapshot-storage/espo"
  local restore_source_root="$TEST_TMP_ROOT/restore-files-snapshot-source"
  local backup_root="$TEST_TMP_ROOT/restore-files-snapshot-backups"
  local archive_file="$TEST_TMP_ROOT/restore-files-snapshot.tar.gz"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.restore-files.sh"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-restore-files-snapshot"
  local plan_file=""

  restore_replaced_repo_files
  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/restore-files-snapshot-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$storage_dir"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  mkdir -p "$restore_source_root/$(basename "$storage_dir")" "$(dirname "$storage_dir")"
  printf 'restored-file\n' > "$restore_source_root/$(basename "$storage_dir")/file.txt"
  tar -C "$restore_source_root" -czf "$archive_file" "$(basename "$storage_dir")"
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock backup args: $*"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  create_mock_docker_fs_helper "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "The pre-restore emergency snapshot will be created without stopping application services" "runtime output"
  assert_file_contains "$output_file" "mock backup args: dev --skip-db --no-stop" "runtime output"
  plan_file="$(find "$backup_root/reports" -maxdepth 1 -type f -name '*_restore-files-plan_*.txt' | sort | tail -n 1)"
  [[ -n "$plan_file" && -f "$plan_file" ]] || fail_test "Regression case failed"
  assert_file_contains "$plan_file" "snapshot_enabled=true" "runtime output"
  assert_file_contains "$plan_file" "no_stop=true" "runtime output"
  assert_file_contains "$plan_file" "no_start=true" "runtime output"
  assert_file_contains "$plan_file" "files_backup=$archive_file" "runtime output"
  assert_file_not_contains "$output_file" "unexpected docker invocation" "runtime output"
  [[ -f "$storage_dir/file.txt" ]] || fail_test "Regression case failed"
  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_restore_db_requires_force_and_prod_confirmation() {
  announce_test "Regression case"
  local dev_env_file="$TEST_TMP_ROOT/env.restore-db-guard.dev"
  local prod_env_file="$TEST_TMP_ROOT/env.restore-db-guard.prod"
  local output_file="$TEST_TMP_ROOT/restore-db-guard.out"
  local db_backup="$TEST_TMP_ROOT/restore-db-guard.sql.gz"

  copy_example_env dev "$dev_env_file"
  copy_example_env prod "$prod_env_file"
  printf 'restore-db-guard\n' > "$db_backup"
  write_sha256_sidecar "$db_backup" "$(sha256_file "$db_backup")"

  if run_command_capture "$output_file" env ENV_FILE="$dev_env_file" bash "$SCRIPT_DIR/restore-db.sh" dev "$db_backup" --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "requires an explicit --force flag" "runtime output"

  if run_command_capture "$output_file" env ENV_FILE="$prod_env_file" bash "$SCRIPT_DIR/restore-db.sh" prod "$db_backup" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "--confirm-prod prod" "runtime output"
  pass_test "Regression case passed"
}

test_restore_files_requires_force_and_prod_confirmation() {
  announce_test "Regression case"

  local dev_env_file="$TEST_TMP_ROOT/env.restore-files-guard.dev"
  local prod_env_file="$TEST_TMP_ROOT/env.restore-files-guard.prod"
  local output_file="$TEST_TMP_ROOT/restore-files-guard.out"
  local source_root="$TEST_TMP_ROOT/restore-files-guard-source"
  local archive_file="$TEST_TMP_ROOT/restore-files-guard.tar.gz"

  copy_example_env dev "$dev_env_file"
  copy_example_env prod "$prod_env_file"
  mkdir -p "$source_root/espo"
  printf 'restore-files-guard\n' > "$source_root/espo/file.txt"
  tar -C "$source_root" -czf "$archive_file" espo
  write_sha256_sidecar "$archive_file" "$(sha256_file "$archive_file")"

  if run_command_capture "$output_file" env ENV_FILE="$dev_env_file" bash "$SCRIPT_DIR/restore-files.sh" dev "$archive_file" --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "requires an explicit --force flag" "runtime output"

  if run_command_capture "$output_file" env ENV_FILE="$prod_env_file" bash "$SCRIPT_DIR/restore-files.sh" prod "$archive_file" --force --no-stop --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "--confirm-prod prod" "runtime output"
  pass_test "Regression case passed"
}

test_restore_drill_selects_complete_set() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/drill-backups"
  local env_file="$TEST_TMP_ROOT/env.restore-drill"
  local output_file="$TEST_TMP_ROOT/restore-drill.out"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-complete"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-complete"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "espo-dev"
  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_02-00-00" "db-incomplete-newer"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  set +e
  env ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-drill.sh" dev >"$output_file" 2>&1
  set -e

  assert_file_contains "$output_file" "Database backup for the restore drill: $backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz" "runtime output"
  assert_file_contains "$output_file" "Files backup for the restore drill: $backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz" "runtime output"
  cleanup_generated_repo_artifacts
  pass_test "Regression case passed"
}

test_restore_drill_rejects_equal_ports() {
  announce_test "Regression case"

  local backup_root="$TEST_TMP_ROOT/restore-drill-equal-ports-backups"
  local env_file="$TEST_TMP_ROOT/env.restore-drill-equal-ports"
  local output_file="$TEST_TMP_ROOT/restore-drill-equal-ports.out"
  local db_backup="$backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz"
  local files_backup="$backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "drill-db"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "drill-files"

  if run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-drill.sh" dev --db-backup "$db_backup" --files-backup "$files_backup" --app-port 28080 --ws-port 28080; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "APP and WS ports for restore-drill must differ" "runtime output"
  pass_test "Regression case passed"
}

test_rollback_delegates_auto_selection_to_go_execution() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.rollback-wrapper-auto"
  local output_file="$TEST_TMP_ROOT/rollback-wrapper-auto.out"
  local status_mock="$TEST_TMP_ROOT/mock.status-report.rollback-auto.sh"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.rollback-auto.sh"
  local restore_db_mock="$TEST_TMP_ROOT/mock.restore-db.rollback-auto.sh"
  local restore_files_mock="$TEST_TMP_ROOT/mock.restore-files.rollback-auto.sh"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.rollback-auto.sh"

  restore_replaced_repo_files
  copy_example_env prod "$env_file"

  cat > "$status_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock status-report should not run"
EOF
  chmod +x "$status_mock"
  replace_repo_file_temporarily "$status_mock" "$SCRIPT_DIR/status-report.sh"

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock backup should not run"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  cat > "$restore_db_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock restore-db should not run"
EOF
  chmod +x "$restore_db_mock"
  replace_repo_file_temporarily "$restore_db_mock" "$SCRIPT_DIR/restore-db.sh"

  cat > "$restore_files_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock restore-files should not run"
EOF
  chmod +x "$restore_files_mock"
  replace_repo_file_temporarily "$restore_files_mock" "$SCRIPT_DIR/restore-files.sh"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock rollback args: $*"
EOF
  chmod +x "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/rollback.sh" prod --force --confirm-prod prod --no-snapshot --timeout 654; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock rollback args: rollback --scope prod --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --force --confirm-prod prod --no-snapshot --timeout 654" "runtime output"
  assert_file_not_contains "$output_file" "mock status-report should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock backup should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock restore-db should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock restore-files should not run" "runtime output"

  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_rollback_delegates_manual_selection_flags_to_go_execution() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.rollback-manual"
  local output_file="$TEST_TMP_ROOT/rollback-manual.out"
  local backup_root="$TEST_TMP_ROOT/rollback-manual-backups"
  local db_file="$backup_root/db/espocrm-prod_2026-04-07_01-00-00.sql.gz"
  local files_file="$backup_root/files/espocrm-prod_files_2026-04-07_01-00-00.tar.gz"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.rollback-manual.sh"

  copy_example_env prod "$env_file"
  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock rollback args: $*"
EOF
  chmod +x "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/rollback.sh" prod --force --confirm-prod prod --db-backup "$db_file" --files-backup "$files_file" --no-snapshot --no-start --skip-http-probe; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock rollback args: rollback --scope prod --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --force --confirm-prod prod --db-backup $db_file --files-backup $files_file --no-snapshot --no-start --skip-http-probe" "runtime output"
  pass_test "Regression case passed"
}

test_rollback_defers_destructive_confirmation_to_go() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.rollback-guard"
  local output_file="$TEST_TMP_ROOT/rollback-guard.out"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.rollback-guard.sh"

  copy_example_env prod "$env_file"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock rollback args: $*"
EOF
  chmod +x "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/rollback.sh" prod --no-snapshot; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock rollback args: rollback --scope prod --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --no-snapshot" "runtime output"
  assert_file_not_contains "$output_file" "requires an explicit --force flag" "runtime output"
  assert_file_not_contains "$output_file" "--confirm-prod prod" "runtime output"
  pass_test "Regression case passed"
}

test_migrate_backup_selects_complete_pair_and_switches_env() {
  announce_test "Regression case"

  local source_backup_root="$TEST_TMP_ROOT/migrate-source"
  local target_backup_root="$TEST_TMP_ROOT/migrate-target"
  local output_file="$TEST_TMP_ROOT/migrate-backup.out"

  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" COMPOSE_PROJECT_NAME source-dev
  set_env_value "$ROOT_DIR/.env.prod" COMPOSE_PROJECT_NAME target-prod
  set_env_value "$ROOT_DIR/.env.dev" BACKUP_ROOT "$source_backup_root"
  set_env_value "$ROOT_DIR/.env.prod" BACKUP_ROOT "$target_backup_root"

  create_db_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "db-complete"
  create_files_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-complete"
  create_db_backup "$source_backup_root" "espocrm-dev" "2026-04-07_02-00-00" "db-newer-incomplete"

  set +e
  bash "$SCRIPT_DIR/migrate-backup.sh" dev prod --force --confirm-prod prod >"$output_file" 2>&1
  set -e

  assert_file_contains "$output_file" "Source env file: $ROOT_DIR/.env.dev" "runtime output"
  assert_file_contains "$output_file" "Target env file: $ROOT_DIR/.env.prod" "runtime output"
  assert_file_contains "$output_file" "Database backup: $source_backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz" "runtime output"
  assert_file_contains "$output_file" "Files backup: $source_backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz" "runtime output"
  pass_test "Regression case passed"
}

test_migrate_backup_supports_partial_selection_and_no_start() {
  announce_test "Regression case"

  local source_backup_root="$TEST_TMP_ROOT/migrate-partial-source"
  local target_backup_root="$TEST_TMP_ROOT/migrate-partial-target"
  local output_file="$TEST_TMP_ROOT/migrate-partial.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-migrate-partial"
  local stack_mock="$TEST_TMP_ROOT/mock.stack.migrate.sh"
  local restore_files_mock="$TEST_TMP_ROOT/mock.restore-files.migrate.sh"

  restore_replaced_repo_files
  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" COMPOSE_PROJECT_NAME source-dev-partial
  set_env_value "$ROOT_DIR/.env.prod" COMPOSE_PROJECT_NAME target-prod-partial
  set_env_value "$ROOT_DIR/.env.dev" BACKUP_ROOT "$source_backup_root"
  set_env_value "$ROOT_DIR/.env.prod" BACKUP_ROOT "$target_backup_root"

  create_files_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "files-partial"

  cat > "$stack_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock stack args: $*"
EOF
  chmod +x "$stack_mock"
  replace_repo_file_temporarily "$stack_mock" "$SCRIPT_DIR/stack.sh"

  cat > "$restore_files_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock restore-files args: $*"
EOF
  chmod +x "$restore_files_mock"
  replace_repo_file_temporarily "$restore_files_mock" "$SCRIPT_DIR/restore-files.sh"

  create_mock_docker_runtime_success "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" bash "$SCRIPT_DIR/migrate-backup.sh" dev prod --force --confirm-prod prod --skip-db --no-start; then
    fail_test "Regression case failed"
  fi

  assert_file_not_contains "$output_file" "[info] Database backup:" "runtime output"
  assert_file_contains "$output_file" "[info] Files backup: $source_backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz" "runtime output"
  assert_file_contains "$output_file" "mock stack args: prod up -d db" "runtime output"
  assert_file_contains "$output_file" "mock restore-files args: prod $source_backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz --force --confirm-prod prod --no-snapshot --no-stop --no-start" "runtime output"
  assert_file_contains "$output_file" "The target contour was left stopped because of --no-start" "runtime output"
  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_migrate_backup_requires_force_and_prod_confirmation() {
  announce_test "Regression case"
  local source_backup_root="$TEST_TMP_ROOT/migrate-guard-source"
  local target_backup_root="$TEST_TMP_ROOT/migrate-guard-target"
  local output_file="$TEST_TMP_ROOT/migrate-guard.out"

  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" BACKUP_ROOT "$source_backup_root"
  set_env_value "$ROOT_DIR/.env.prod" BACKUP_ROOT "$target_backup_root"

  create_db_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "migrate-guard-db"
  create_files_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "migrate-guard-files"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/migrate-backup.sh" dev prod; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "requires an explicit --force flag" "runtime output"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/migrate-backup.sh" dev prod --force; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "--confirm-prod prod" "runtime output"
  pass_test "Regression case passed"
}

test_migrate_backup_rejects_config_contract_drift() {
  announce_test "Regression case"

  local source_backup_root="$TEST_TMP_ROOT/migrate-contract-source"
  local target_backup_root="$TEST_TMP_ROOT/migrate-contract-target"
  local output_file="$TEST_TMP_ROOT/migrate-contract.out"

  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" BACKUP_ROOT "$source_backup_root"
  set_env_value "$ROOT_DIR/.env.prod" BACKUP_ROOT "$target_backup_root"
  set_env_value "$ROOT_DIR/.env.dev" ESPOCRM_IMAGE "espocrm/espocrm:9.3.4-apache"
  set_env_value "$ROOT_DIR/.env.prod" ESPOCRM_IMAGE "espocrm/espocrm:9.4.0-apache"

  create_db_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "migrate-contract-db"
  create_files_backup "$source_backup_root" "espocrm-dev" "2026-04-07_01-00-00" "migrate-contract-files"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/migrate-backup.sh" dev prod --force --confirm-prod prod; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "conflict with the migration compatibility contract" "runtime output"
  assert_file_contains "$output_file" "ESPOCRM_IMAGE ('espocrm/espocrm:9.3.4-apache' vs 'espocrm/espocrm:9.4.0-apache')" "runtime output"
  assert_file_contains "$output_file" "./scripts/doctor.sh all" "runtime output"
  pass_test "Regression case passed"
}
