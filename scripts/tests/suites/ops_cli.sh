# shellcheck shell=bash

test_update_can_skip_optional_steps() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.update-skip-flags"
  local output_file="$TEST_TMP_ROOT/update-skip-flags.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-update"
  local status_mock="$TEST_TMP_ROOT/mock.status-report.update.sh"
  local doctor_mock="$TEST_TMP_ROOT/mock.doctor.update.sh"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.update.sh"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.update.sh"

  restore_replaced_repo_files
  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/update-skip-backups"
  set_env_value "$env_file" SITE_URL "https://dev-update.test"

  cat > "$status_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
output_path=""
json_mode=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    dev|prod)
      shift
      ;;
    --json)
      json_mode=1
      shift
      ;;
    --output)
      output_path="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ $json_mode -eq 1 ]]; then
  content='{"status":"mock-ok"}'
else
  content='mock status ok'
fi

if [[ -n "$output_path" ]]; then
  mkdir -p "$(dirname "$output_path")"
  printf '%s\n' "$content" > "$output_path"
else
  printf '%s\n' "$content"
fi
EOF
  chmod +x "$status_mock"
  replace_repo_file_temporarily "$status_mock" "$SCRIPT_DIR/status-report.sh"

  cat > "$doctor_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock doctor should not run"
EOF
  chmod +x "$doctor_mock"
  replace_repo_file_temporarily "$doctor_mock" "$SCRIPT_DIR/doctor.sh"

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock backup should not run"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  run-operation)
    echo "mock run-operation args: $*"
    scope=""
    env_file=""
    shift
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --scope)
          scope="$2"
          shift 2
          ;;
        --operation|--project-dir|--env-file)
          if [[ "$1" == "--env-file" ]]; then
            env_file="$2"
          fi
          shift 2
          ;;
        --)
          shift
          break
          ;;
        *)
          echo "unexpected run-operation arg: $1" >&2
          exit 97
          ;;
      esac
    done

    export ESPO_ENV="$scope"
    export RESOLVED_ENV_CONTOUR="$scope"
    export ENV_FILE="$env_file"
    export ESPO_OPERATION_LOCK=1
    export ESPO_MAINTENANCE_LOCK=1
    export ESPO_SHELL_EXEC_CONTEXT=1
    if [[ -n "$env_file" ]]; then
      set -a
      # shellcheck disable=SC1090
      source "$env_file"
      set +a
    fi
    "$@"
    ;;
  update-runtime)
    echo "mock update-runtime args: $*"
    ;;
  *)
    echo "unexpected espops args: $*" >&2
    exit 98
    ;;
esac
EOF
  chmod +x "$mock_espops"

  create_mock_docker_update_success "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/update.sh" dev --skip-doctor --skip-backup --skip-pull --skip-http-probe --timeout 321; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Environment check skipped because of --skip-doctor" "runtime output"
  assert_file_contains "$output_file" "Backup skipped because of --skip-backup" "runtime output"
  assert_file_contains "$output_file" "mock run-operation args: run-operation --scope dev --operation update --project-dir $ROOT_DIR --env-file $env_file -- bash $SCRIPT_DIR/update.sh dev --skip-doctor --skip-backup --skip-pull --skip-http-probe --timeout 321" "runtime output"
  assert_file_contains "$output_file" "mock update-runtime args: update-runtime --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --site-url https://dev-update.test --timeout 321 --skip-pull --skip-http-probe" "runtime output"
  assert_file_contains "$output_file" "Update completed successfully" "runtime output"
  assert_file_not_contains "$output_file" "mock doctor should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock backup should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock docker pull should not run" "runtime output"
  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_update_propagates_go_runtime_timeout_failure() {
  announce_test "Regression case"
  local env_file="$TEST_TMP_ROOT/env.update-timeout-budget"
  local output_file="$TEST_TMP_ROOT/update-timeout-budget.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-update-timeout-budget"
  local state_dir="$TEST_TMP_ROOT/mock-docker-update-timeout-state"
  local status_mock="$TEST_TMP_ROOT/mock.status-report.update-timeout.sh"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.update-timeout.sh"

  restore_replaced_repo_files
  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/update-timeout-budget-backups"
  set_env_value "$env_file" SITE_URL "https://dev-update-timeout.test"

  cat > "$status_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
output_path=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output_path="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -n "$output_path" ]]; then
  mkdir -p "$(dirname "$output_path")"
  printf '%s\n' 'mock status ok' > "$output_path"
else
  printf '%s\n' 'mock status ok'
fi
EOF
  chmod +x "$status_mock"
  replace_repo_file_temporarily "$status_mock" "$SCRIPT_DIR/status-report.sh"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  run-operation)
    echo "mock run-operation args: $*"
    scope=""
    env_file=""
    shift
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --scope)
          scope="$2"
          shift 2
          ;;
        --operation|--project-dir|--env-file)
          if [[ "$1" == "--env-file" ]]; then
            env_file="$2"
          fi
          shift 2
          ;;
        --)
          shift
          break
          ;;
        *)
          echo "unexpected run-operation arg: $1" >&2
          exit 97
          ;;
      esac
    done

    export ESPO_ENV="$scope"
    export RESOLVED_ENV_CONTOUR="$scope"
    export ENV_FILE="$env_file"
    export ESPO_OPERATION_LOCK=1
    export ESPO_MAINTENANCE_LOCK=1
    export ESPO_SHELL_EXEC_CONTEXT=1
    if [[ -n "$env_file" ]]; then
      set -a
      # shellcheck disable=SC1090
      source "$env_file"
      set +a
    fi
    "$@"
    ;;
  update-backup)
    echo "mock update-backup args: $*"
    exit 0
    ;;
  update-runtime)
    echo "mock update-runtime args: $*"
    echo "ERROR [update_runtime_failed]: timed out while waiting for service readiness 'espocrm-daemon' (10 sec.)" >&2
    exit 5
    ;;
  *)
    echo "unexpected espops args: $*" >&2
    exit 98
    ;;
esac
EOF
  chmod +x "$mock_espops"

  create_mock_docker_shared_timeout_budget "$mock_bin_dir"

  mkdir -p "$state_dir"
  printf 'db\n' > "$state_dir/running-services"

  if run_command_capture "$output_file" env \
    PATH="$mock_bin_dir:$PATH" \
    MOCK_DOCKER_STATE_DIR="$state_dir" \
    ENV_FILE="$env_file" \
    ESPOPS_BIN="$mock_espops" \
    bash "$SCRIPT_DIR/update.sh" dev --skip-doctor --skip-pull --skip-http-probe --timeout 10; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock run-operation args: run-operation --scope dev --operation update --project-dir $ROOT_DIR --env-file $env_file -- bash $SCRIPT_DIR/update.sh dev --skip-doctor --skip-pull --skip-http-probe --timeout 10" "runtime output"
  assert_file_contains "$output_file" "mock update-backup args: update-backup --scope dev --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --timeout 10" "runtime output"
  assert_file_contains "$output_file" "mock update-runtime args: update-runtime --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --site-url https://dev-update-timeout.test --timeout 10 --skip-pull --skip-http-probe" "runtime output"
  assert_file_contains "$output_file" "timed out while waiting for service readiness 'espocrm-daemon' (10 sec.)" "runtime output"
  assert_file_not_contains "$output_file" "Update completed successfully" "runtime output"

  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_update_dry_run_delegates_to_go_update_plan() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.update-dry-run"
  local output_file="$TEST_TMP_ROOT/update-dry-run.out"
  local status_mock="$TEST_TMP_ROOT/mock.status-report.update-dry-run.sh"
  local doctor_mock="$TEST_TMP_ROOT/mock.doctor.update-dry-run.sh"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.update-dry-run.sh"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.update-dry-run.sh"

  restore_replaced_repo_files
  copy_example_env dev "$env_file"

  cat > "$status_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock status-report should not run"
EOF
  chmod +x "$status_mock"
  replace_repo_file_temporarily "$status_mock" "$SCRIPT_DIR/status-report.sh"

  cat > "$doctor_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock doctor should not run"
EOF
  chmod +x "$doctor_mock"
  replace_repo_file_temporarily "$doctor_mock" "$SCRIPT_DIR/doctor.sh"

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock backup should not run"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  update-plan)
    echo "mock update-plan args: $*"
    ;;
  *)
    echo "unexpected espops args: $*" >&2
    exit 98
    ;;
esac
EOF
  chmod +x "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/update.sh" dev --dry-run --skip-doctor --skip-backup --skip-pull --skip-http-probe --timeout 321; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock update-plan args: update-plan --scope dev --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --timeout 321 --env-file $env_file --skip-doctor --skip-backup --skip-pull --skip-http-probe" "runtime output"
  assert_file_not_contains "$output_file" "mock status-report should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock doctor should not run" "runtime output"
  assert_file_not_contains "$output_file" "mock backup should not run" "runtime output"

  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_backup_delegates_to_go_backup_exec() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.backup-go-delegation"
  local output_file="$TEST_TMP_ROOT/backup-go-delegation.out"
  local mock_espops="$TEST_TMP_ROOT/mock.espops.backup.sh"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/backup-go-delegation-backups"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/backup-go-delegation-storage/espo"

  mkdir -p "$TEST_TMP_ROOT/backup-go-delegation-storage/espo"
  printf 'backup-go-delegation\n' > "$TEST_TMP_ROOT/backup-go-delegation-storage/espo/file.txt"

  cat > "$mock_espops" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  run-operation)
    echo "mock run-operation args: $*"
    scope=""
    env_file=""
    shift
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --scope)
          scope="$2"
          shift 2
          ;;
        --operation|--project-dir|--env-file)
          if [[ "$1" == "--env-file" ]]; then
            env_file="$2"
          fi
          shift 2
          ;;
        --)
          shift
          break
          ;;
        *)
          echo "unexpected run-operation arg: $1" >&2
          exit 97
          ;;
      esac
    done

    export ESPO_ENV="$scope"
    export RESOLVED_ENV_CONTOUR="$scope"
    export ENV_FILE="$env_file"
    export ESPO_OPERATION_LOCK=1
    export ESPO_MAINTENANCE_LOCK=1
    export ESPO_SHELL_EXEC_CONTEXT=1
    if [[ -n "$env_file" ]]; then
      set -a
      # shellcheck disable=SC1090
      source "$env_file"
      set +a
    fi
    "$@"
    ;;
  backup-exec)
    echo "mock backup-exec args: $*"
    ;;
  *)
    echo "unexpected espops args: $*" >&2
    exit 98
    ;;
esac
EOF
  chmod +x "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/backup.sh" dev --skip-db --no-stop; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "mock run-operation args: run-operation --scope dev --operation backup --project-dir $ROOT_DIR --env-file $env_file -- bash $SCRIPT_DIR/backup.sh dev --skip-db --no-stop" "runtime output"
  assert_file_contains "$output_file" "mock backup-exec args: backup-exec --scope dev --project-dir $ROOT_DIR --compose-file $ROOT_DIR/compose.yaml --env-file $env_file --skip-db --no-stop" "runtime output"
  pass_test "Regression case passed"
}

test_smoke_test_can_keep_artifacts() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/smoke-keep-artifacts.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-smoke"
  local backup_mock="$TEST_TMP_ROOT/mock.backup.smoke.sh"
  local verify_mock="$TEST_TMP_ROOT/mock.verify.smoke.sh"
  local smoke_env_file=""

  restore_replaced_repo_files
  cleanup_generated_repo_artifacts

  cat > "$backup_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock smoke backup args: $*"
EOF
  chmod +x "$backup_mock"
  replace_repo_file_temporarily "$backup_mock" "$SCRIPT_DIR/backup.sh"

  cat > "$verify_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock smoke verify args: $*"
EOF
  chmod +x "$verify_mock"
  replace_repo_file_temporarily "$verify_mock" "$SCRIPT_DIR/verify-backup.sh"

  create_mock_docker_runtime_success "$mock_bin_dir"
  create_mock_curl_success "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" bash "$SCRIPT_DIR/smoke-test.sh" dev --from-example --keep-artifacts --timeout 123; then
    fail_test "Regression case failed"
  fi

  smoke_env_file="$(find "$ROOT_DIR/.cache/env" -maxdepth 1 -type f -name 'smoke.dev.*.env' | sort | tail -n 1)"
  [[ -n "$smoke_env_file" && -f "$smoke_env_file" ]] || fail_test "Regression case failed"
  [[ -d "$ROOT_DIR/storage/smoke/dev" ]] || fail_test "Regression case failed"
  [[ -d "$ROOT_DIR/backups/smoke/dev" ]] || fail_test "Regression case failed"

  assert_file_contains "$output_file" "Temporary smoke environment preserved because of --keep-artifacts" "runtime output"
  assert_file_contains "$output_file" "mock smoke backup args: dev" "runtime output"
  assert_file_contains "$output_file" "mock smoke verify args: dev" "runtime output"

  restore_replaced_repo_files
  cleanup_generated_repo_artifacts
  pass_test "Regression case passed"
}

test_restore_drill_supports_explicit_selection_skip_probe_and_keep_artifacts() {
  announce_test "Regression case"
  local backup_root="$TEST_TMP_ROOT/restore-drill-explicit-backups"
  local env_file="$TEST_TMP_ROOT/env.restore-drill-explicit"
  local output_file="$TEST_TMP_ROOT/restore-drill-explicit.out"
  local db_backup="$backup_root/db/espocrm-dev_2026-04-07_01-00-00.sql.gz"
  local files_backup="$backup_root/files/espocrm-dev_files_2026-04-07_01-00-00.tar.gz"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-restore-drill-explicit"
  local restore_db_mock="$TEST_TMP_ROOT/mock.restore-db.drill.sh"
  local restore_files_mock="$TEST_TMP_ROOT/mock.restore-files.drill.sh"
  local status_mock="$TEST_TMP_ROOT/mock.status-report.drill.sh"
  local drill_env_file=""

  restore_replaced_repo_files
  cleanup_generated_repo_artifacts

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/restore-drill-source-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/restore-drill-source-storage/espo"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"
  set_env_value "$env_file" APP_PORT "18088"
  set_env_value "$env_file" WS_PORT "18089"
  set_env_value "$env_file" SITE_URL "http://127.0.0.1:18088"
  set_env_value "$env_file" WS_PUBLIC_URL "ws://127.0.0.1:18089"

  create_db_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "drill-explicit-db"
  create_files_backup "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "drill-explicit-files"
  create_manifest_pair "$backup_root" "espocrm-dev" "2026-04-07_01-00-00" "dev" "drill-explicit"

  cat > "$restore_db_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock restore-db args: $*"
EOF
  chmod +x "$restore_db_mock"
  replace_repo_file_temporarily "$restore_db_mock" "$SCRIPT_DIR/restore-db.sh"

  cat > "$restore_files_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "mock restore-files args: $*"
EOF
  chmod +x "$restore_files_mock"
  replace_repo_file_temporarily "$restore_files_mock" "$SCRIPT_DIR/restore-files.sh"

  cat > "$status_mock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
output_path=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output_path="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -n "$output_path" ]]; then
  mkdir -p "$(dirname "$output_path")"
  printf '%s\n' 'mock drill status' > "$output_path"
else
  printf '%s\n' 'mock drill status'
fi
EOF
  chmod +x "$status_mock"
  replace_repo_file_temporarily "$status_mock" "$SCRIPT_DIR/status-report.sh"

  create_mock_docker_runtime_success "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/restore-drill.sh" dev --db-backup "$db_backup" --files-backup "$files_backup" --timeout 123 --app-port 28080 --ws-port 28081 --skip-http-probe --keep-artifacts; then
    fail_test "Regression case failed"
  fi

  drill_env_file="$(find "$ROOT_DIR/.cache/env" -maxdepth 1 -type f -name 'restore-drill.dev.*.env' | sort | tail -n 1)"
  [[ -n "$drill_env_file" && -f "$drill_env_file" ]] || fail_test "Regression case failed"
  [[ -d "$ROOT_DIR/storage/restore-drill/dev/db" ]] || fail_test "Regression case failed"
  [[ -d "$ROOT_DIR/storage/restore-drill/dev/espo" ]] || fail_test "Regression case failed"
  [[ -d "$ROOT_DIR/backups/restore-drill/dev" ]] || fail_test "Regression case failed"

  assert_file_contains "$output_file" "HTTP probe skipped because of --skip-http-probe" "runtime output"
  assert_file_contains "$output_file" "mock restore-db args: dev $db_backup --force --no-snapshot --no-stop --no-start" "runtime output"
  assert_file_contains "$output_file" "mock restore-files args: dev $files_backup --force --no-snapshot --no-stop --no-start" "runtime output"
  assert_file_contains "$output_file" "Temporary restore-drill contour preserved because of --keep-artifacts" "runtime output"

  restore_replaced_repo_files
  cleanup_generated_repo_artifacts
  pass_test "Regression case passed"
}

test_status_report_writes_output_files() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.status-report"
  local text_output="$TEST_TMP_ROOT/status-report.txt"
  local json_output="$TEST_TMP_ROOT/status-report.json"
  local command_output="$TEST_TMP_ROOT/status-report-command.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-status-report"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/status-report-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/status-report-storage/espo"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/status-report-backups"

  create_mock_docker_cli "$mock_bin_dir"

  if ! run_command_capture "$command_output" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/status-report.sh" dev --output "$text_output"; then
    fail_test "Regression case failed"
  fi

  [[ -f "$text_output" ]] || fail_test "Regression case failed"
  assert_file_contains "$command_output" "Report saved: $text_output" "runtime output"
  assert_file_contains "$text_output" "Contour:          dev" "runtime output"
  assert_file_contains "$text_output" "db:                 daemon_unavailable" "runtime output"

  if ! run_command_capture "$command_output" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/status-report.sh" dev --json --output "$json_output"; then
    fail_test "Regression case failed"
  fi

  [[ -f "$json_output" ]] || fail_test "Regression case failed"
  assert_file_contains "$command_output" "Report saved: $json_output" "runtime output"
  assert_file_contains "$json_output" "\"contour\": \"dev\"" "runtime output"
  assert_file_contains "$json_output" "\"db\": \"daemon_unavailable\"" "runtime output"
  pass_test "Regression case passed"
}

test_docker_cleanup_rejects_invalid_duration() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/docker-cleanup-invalid-duration.out"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/docker-cleanup.sh" --container-age invalid; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Parameter --container-age must use a format such as 30m, 12h, 7d, or 2w: invalid" "runtime output"
  pass_test "Regression case passed"
}

test_docker_cleanup_supports_explicit_report_dir() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/docker-cleanup-report-dir.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-cleanup-report-dir"
  local report_dir="$TEST_TMP_ROOT/docker-cleanup-reports"
  local report_file=""

  create_mock_docker_cleanup_plan_success "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" bash "$SCRIPT_DIR/docker-cleanup.sh" --report-dir "$report_dir" --skip-build-cache; then
    fail_test "Regression case failed"
  fi

  report_file="$(find "$report_dir" -maxdepth 1 -type f -name 'docker-cleanup_plan_*.txt' | sort | tail -n 1)"
  [[ -n "$report_file" && -f "$report_file" ]] || fail_test "Regression case failed"

  assert_file_contains "$output_file" "Report: $report_file" "runtime output"
  assert_file_contains "$report_file" "Mode: plan" "runtime output"
  pass_test "Regression case passed"
}

test_doctor_does_not_require_ripgrep_for_own_published_ports() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.doctor-no-rg"
  local output_file="$TEST_TMP_ROOT/doctor-no-rg.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-doctor-no-rg"
  local app_port="19080"

  copy_example_env prod "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/doctor-no-rg-runtime/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/doctor-no-rg-runtime/espo"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/doctor-no-rg-backups"
  set_env_value "$env_file" APP_PORT "$app_port"
  set_env_value "$env_file" SITE_URL "http://127.0.0.1:$app_port"
  set_env_value "$env_file" DB_ROOT_PASSWORD "doctor-no-rg-root"
  set_env_value "$env_file" DB_PASSWORD "doctor-no-rg-db"
  set_env_value "$env_file" ADMIN_PASSWORD "doctor-no-rg-admin"

  mkdir -p "$mock_bin_dir"

  cat > "$mock_bin_dir/docker" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail

args=" \$* "

if [[ "\${1:-}" == "info" ]]; then
  exit 0
fi

if [[ "\${1:-}" == "version" && "\${2:-}" == "--format" ]]; then
  echo "24.0.7"
  exit 0
fi

if [[ "\${1:-}" == "compose" && "\$args" == *" version "* && "\$args" == *" --short "* ]]; then
  echo "2.24.0"
  exit 0
fi

if [[ "\${1:-}" == "compose" && "\$args" == *" version "* ]]; then
  echo "Docker Compose version v2.24.0"
  exit 0
fi

if [[ "\${1:-}" == "compose" && "\$args" == *" config "* ]]; then
  echo "services: {}"
  exit 0
fi

if [[ "\${1:-}" == "compose" && "\$args" == *" ps "* && "\$args" == *" --status running "* ]]; then
  cat <<'EOF2'
NAME                 IMAGE                COMMAND   SERVICE   CREATED   STATUS          PORTS
espo-prod-espocrm-1  espocrm/mock:latest  "httpd"   espocrm   1m ago    Up 1m (healthy) 0.0.0.0:${app_port}->80/tcp
EOF2
  exit 0
fi

echo "unexpected docker invocation: \$*" >&2
exit 98
EOF
  chmod +x "$mock_bin_dir/docker"

  cat > "$mock_bin_dir/ss" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail

cat <<'EOF2'
State  Recv-Q Send-Q Local Address:Port Peer Address:PortProcess
LISTEN 0      511          0.0.0.0:${app_port}      0.0.0.0:*
EOF2
EOF
  chmod +x "$mock_bin_dir/ss"

  cat > "$mock_bin_dir/rg" <<'EOF'
#!/usr/bin/env bash
echo "rg: command not found" >&2
exit 127
EOF
  chmod +x "$mock_bin_dir/rg"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:/usr/bin:/bin" ENV_FILE="$env_file" bash "$SCRIPT_DIR/doctor.sh" prod; then
    true
  fi

  assert_file_contains "$output_file" "[prod] Port APP_PORT=$app_port is already published by the current contour" "runtime output"
  assert_file_not_contains "$output_file" "rg: command not found" "runtime output"
  pass_test "Regression case passed"
}

test_doctor_all_switches_between_contours() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/doctor-all.out"

  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" COMPOSE_PROJECT_NAME doctor-dev
  set_env_value "$ROOT_DIR/.env.prod" COMPOSE_PROJECT_NAME doctor-prod

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/doctor.sh" all --json; then
    true
  fi

  assert_file_contains "$output_file" "[prod] Env file loaded successfully" "runtime output"
  assert_file_contains "$output_file" "[dev] Env file loaded successfully" "runtime output"
  assert_file_contains "$output_file" "[cross] COMPOSE_PROJECT_NAME differs between prod and dev" "runtime output"
  pass_test "Regression case passed"
}

test_doctor_all_rejects_migration_contract_drift() {
  announce_test "Regression case"
  local output_file="$TEST_TMP_ROOT/doctor-all-migration-contract.out"

  prepare_repo_env_pair
  set_env_value "$ROOT_DIR/.env.dev" ESPOCRM_IMAGE "espocrm/espocrm:9.3.4-apache"
  set_env_value "$ROOT_DIR/.env.prod" ESPOCRM_IMAGE "espocrm/espocrm:9.4.0-apache"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/doctor.sh" all; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "[cross] ESPOCRM_IMAGE must match between prod and dev" "runtime output"
  pass_test "Regression case passed"
}

test_docker_daemon_preflight_is_fail_fast() {
  announce_test "Regression case"

  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-bin"
  local runtime_output="$TEST_TMP_ROOT/docker-daemon-runtime.out"
  local config_output="$TEST_TMP_ROOT/docker-daemon-config.out"

  prepare_repo_env_pair
  create_mock_docker_cli "$mock_bin_dir"

  if run_command_capture "$runtime_output" env PATH="$mock_bin_dir:$PATH" bash "$SCRIPT_DIR/stack.sh" dev up -d; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$runtime_output" "Docker daemon is unavailable." "runtime output"
  assert_file_not_contains "$runtime_output" "compose up must not run without daemon preflight" "runtime output"

  if ! run_command_capture "$config_output" env PATH="$mock_bin_dir:$PATH" bash "$SCRIPT_DIR/stack.sh" dev config; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$config_output" "services: {}" "runtime output"
  pass_test "Regression case passed"
}

test_espo_cli_dispatches_commands() {
  announce_test "Regression case"
  local output_file="$TEST_TMP_ROOT/espo-cli-route.out"
  local help_output="$TEST_TMP_ROOT/espo-cli-help.out"
  local mock_file=""
  local script_name
  local mocked_scripts=(
    doctor.sh
    contour-overview.sh
    bootstrap.sh
    stack.sh
    backup.sh
    verify-backup.sh
    backup-audit.sh
    backup-catalog.sh
    restore-db.sh
    restore-files.sh
    restore-drill.sh
    rollback.sh
    migrate-backup.sh
    status-report.sh
    support-bundle.sh
    update.sh
    smoke-test.sh
    docker-cleanup.sh
    regression-test.sh
  )

  restore_replaced_repo_files

  for script_name in "${mocked_scripts[@]}"; do
    mock_file="$TEST_TMP_ROOT/mock.$script_name"
    create_mock_echo_script "$script_name" "$mock_file"
    replace_repo_file_temporarily "$mock_file" "$SCRIPT_DIR/$script_name"
  done

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" doctor dev --json; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "doctor.sh: dev --json" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" bootstrap prod; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "bootstrap.sh: prod" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" overview dev; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "contour-overview.sh: dev" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" stack dev ps; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: dev ps" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" up prod -d; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: prod up -d" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" down dev; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: dev down" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" stop prod; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: prod stop" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" restart dev espocrm; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: dev restart espocrm" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" ps prod; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: prod ps" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" logs dev --tail=5 espocrm; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: dev logs --tail=5 espocrm" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" pull prod; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: prod pull" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" config dev; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: dev config" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" exec prod -T espocrm php -v; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "stack.sh: prod exec -T espocrm php -v" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" backup prod --no-stop; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "backup.sh: prod --no-stop" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" backup verify dev --skip-db --files-backup /tmp/files.tar.gz; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "verify-backup.sh: dev --skip-db --files-backup /tmp/files.tar.gz" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" backup audit prod --json --skip-files --max-db-age-hours 24 --no-verify-checksum; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "backup-audit.sh: prod --json --skip-files --max-db-age-hours 24 --no-verify-checksum" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" backup catalog dev --json --limit 1 --latest-only --ready-only --verify-checksum; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "backup-catalog.sh: dev --json --limit 1 --latest-only --ready-only --verify-checksum" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" restore db prod /tmp/db.sql.gz --force --confirm-prod prod --snapshot-before-restore --no-start; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "restore-db.sh: prod /tmp/db.sql.gz --force --confirm-prod prod --snapshot-before-restore --no-start" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" restore files dev /tmp/files.tar.gz --force --snapshot-before-restore --no-stop --no-start; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "restore-files.sh: dev /tmp/files.tar.gz --force --snapshot-before-restore --no-stop --no-start" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" restore drill prod --db-backup /tmp/db.sql.gz --files-backup /tmp/files.tar.gz --timeout 123 --app-port 28080 --ws-port 28081 --skip-http-probe --keep-artifacts; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "restore-drill.sh: prod --db-backup /tmp/db.sql.gz --files-backup /tmp/files.tar.gz --timeout 123 --app-port 28080 --ws-port 28081 --skip-http-probe --keep-artifacts" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" rollback prod --force --confirm-prod prod --db-backup /tmp/db.sql.gz --files-backup /tmp/files.tar.gz --no-snapshot --no-start --skip-http-probe --timeout 321; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "rollback.sh: prod --force --confirm-prod prod --db-backup /tmp/db.sql.gz --files-backup /tmp/files.tar.gz --no-snapshot --no-start --skip-http-probe --timeout 321" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" migrate dev prod --force --confirm-prod prod --skip-db --no-start; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "migrate-backup.sh: dev prod --force --confirm-prod prod --skip-db --no-start" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" status dev --json --output /tmp/status.json; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "status-report.sh: dev --json --output /tmp/status.json" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" support prod --tail 123 --output /tmp/support.tar.gz; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "support-bundle.sh: prod --tail 123 --output /tmp/support.tar.gz" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" update dev --skip-doctor --skip-backup --skip-pull --skip-http-probe --timeout 456; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "update.sh: dev --skip-doctor --skip-backup --skip-pull --skip-http-probe --timeout 456" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" smoke prod --from-example --keep-artifacts --timeout 789; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "smoke-test.sh: prod --from-example --keep-artifacts --timeout 789" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" cleanup --apply --include-unused-images --skip-build-cache --container-age 1h --image-age 2h --unused-image-age 3h --network-age 4h --builder-age 5h; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "docker-cleanup.sh: --apply --include-unused-images --skip-build-cache --container-age 1h --image-age 2h --unused-image-age 3h --network-age 4h --builder-age 5h" "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" regression --help; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "regression-test.sh: --help" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "Usage: ./scripts/espo.sh" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help up; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "Usage: ./scripts/espo.sh" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help doctor; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "Unified operator entrypoint:" "runtime output"
  assert_file_contains "$help_output" "./scripts/espo.sh doctor [dev|prod|all] [args...]" "runtime output"
  assert_file_contains "$help_output" "doctor.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help bootstrap; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "bootstrap.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help overview; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh overview <dev|prod> [args...]" "runtime output"
  assert_file_contains "$help_output" "contour-overview.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help stack; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "stack.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help backup; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh backup <dev|prod> [args...]" "runtime output"
  assert_file_contains "$help_output" "backup.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help backup verify; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh backup verify <dev|prod> [args...]" "runtime output"
  assert_file_contains "$help_output" "verify-backup.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help backup audit; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "backup-audit.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help backup catalog; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "backup-catalog.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help restore db; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh restore db <dev|prod> <file> [args...]" "runtime output"
  assert_file_contains "$help_output" "restore-db.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help restore files; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "restore-files.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help restore drill; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "restore-drill.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help rollback; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "rollback.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help migrate; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "migrate-backup.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help status; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "status-report.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help support; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "support-bundle.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help update; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh update <dev|prod> [args...]" "runtime output"
  assert_file_contains "$help_output" "update.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help smoke; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "smoke-test.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help cleanup; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "./scripts/espo.sh cleanup [args...]" "runtime output"
  assert_file_contains "$help_output" "docker-cleanup.sh: -h" "runtime output"

  if ! run_command_capture "$help_output" bash "$SCRIPT_DIR/espo.sh" help regression; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$help_output" "regression-test.sh: -h" "runtime output"

  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_contour_overview_runs_read_only_checks() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/contour-overview.out"
  local json_file="$TEST_TMP_ROOT/contour-overview.json"
  local mock_file=""
  local script_name
  local mocked_scripts=(
    doctor.sh
    status-report.sh
    backup-audit.sh
    backup-catalog.sh
  )

  restore_replaced_repo_files

  for script_name in "${mocked_scripts[@]}"; do
    mock_file="$TEST_TMP_ROOT/mock.overview.$script_name"
    create_mock_echo_script "$script_name" "$mock_file"
    replace_repo_file_temporarily "$mock_file" "$SCRIPT_DIR/$script_name"
  done

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/contour-overview.sh" dev; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Contour: dev" "runtime output"
  assert_file_contains "$output_file" "Mode: read-only overview" "runtime output"
  assert_file_contains "$output_file" "doctor.sh: dev" "runtime output"
  assert_file_contains "$output_file" "status-report.sh: dev" "runtime output"
  assert_file_contains "$output_file" "backup-audit.sh: dev" "runtime output"
  assert_file_contains "$output_file" "backup-catalog.sh: dev --latest-only --verify-checksum" "runtime output"
  assert_file_contains "$output_file" "Read-only overview completed without errors." "runtime output"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/contour-overview.sh" dev --json --output "$json_file"; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Overview saved: $json_file" "runtime output"
  assert_file_contains "$json_file" "\"contour\": \"dev\"" "runtime output"
  assert_file_contains "$json_file" "\"mode\": \"read_only\"" "runtime output"
  assert_file_contains "$json_file" "\"success\": true" "runtime output"
  assert_file_contains "$json_file" "\"failed_sections\": 0" "runtime output"
  assert_file_contains "$json_file" "\"id\": \"backup_catalog\"" "runtime output"
  assert_file_contains "$json_file" "\"command\": \"./scripts/espo.sh backup catalog dev --json --latest-only --verify-checksum\"" "runtime output"
  assert_file_contains "$json_file" "backup-catalog.sh: dev --json --latest-only --verify-checksum" "runtime output"

  cat > "$mock_file" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
echo "backup-audit.sh: $*"
exit 7
EOF
  chmod +x "$mock_file"
  replace_repo_file_temporarily "$mock_file" "$SCRIPT_DIR/backup-audit.sh"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/contour-overview.sh" prod; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "backup-audit.sh: prod" "runtime output"
  assert_file_contains "$output_file" "backup-catalog.sh: prod --latest-only --verify-checksum" "runtime output"
  assert_file_contains "$output_file" "Overview found problems" "runtime output"

  restore_replaced_repo_files
  pass_test "Regression case passed"
}

test_support_bundle_redacts_secrets() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.support-bundle"
  local output_file="$TEST_TMP_ROOT/support-bundle.out"
  local bundle_file="$TEST_TMP_ROOT/support-bundle.tar.gz"
  local unpack_dir="$TEST_TMP_ROOT/support-bundle-unpack"
  local secret_root="ROOT_SECRET_12345"
  local secret_db="APP_SECRET_67890"
  local secret_admin="ADMIN_SECRET_24680"

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/support-bundle-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/support-bundle-storage/espo"
  set_env_value "$env_file" BACKUP_ROOT "$TEST_TMP_ROOT/support-bundle-backups"
  set_env_value "$env_file" DB_ROOT_PASSWORD "$secret_root"
  set_env_value "$env_file" DB_PASSWORD "$secret_db"
  set_env_value "$env_file" ADMIN_PASSWORD "$secret_admin"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" bash "$SCRIPT_DIR/support-bundle.sh" dev --output "$bundle_file"; then
    fail_test "Regression case failed"
  fi

  mkdir -p "$unpack_dir"
  tar -C "$unpack_dir" -xzf "$bundle_file"

  if rg -Fq "$secret_root" "$unpack_dir" || rg -Fq "$secret_db" "$unpack_dir" || rg -Fq "$secret_admin" "$unpack_dir"; then
    fail_test "Regression case failed"
  fi

  pass_test "Regression case passed"
}

test_support_bundle_supports_tail_and_default_output() {
  announce_test "Regression case"

  local env_file="$TEST_TMP_ROOT/env.support-bundle-default"
  local output_file="$TEST_TMP_ROOT/support-bundle-default.out"
  local mock_bin_dir="$TEST_TMP_ROOT/mock-docker-support-default"
  local backup_root="$TEST_TMP_ROOT/support-bundle-default-backups"
  local support_bundle=""

  copy_example_env dev "$env_file"
  set_env_value "$env_file" DB_STORAGE_DIR "$TEST_TMP_ROOT/support-default-storage/db"
  set_env_value "$env_file" ESPO_STORAGE_DIR "$TEST_TMP_ROOT/support-default-storage/espo"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"

  create_mock_docker_cli "$mock_bin_dir"

  if ! run_command_capture "$output_file" env PATH="$mock_bin_dir:$PATH" ENV_FILE="$env_file" bash "$SCRIPT_DIR/support-bundle.sh" dev --tail 42; then
    fail_test "Regression case failed"
  fi

  support_bundle="$(find "$backup_root/support" -maxdepth 1 -type f -name '*.tar.gz' | sort | tail -n 1)"
  [[ -n "$support_bundle" && -f "$support_bundle" ]] || fail_test "Regression case failed"

  assert_file_contains "$output_file" "Building support bundle: $support_bundle" "runtime output"
  assert_file_contains "$output_file" "Support bundle created: $support_bundle" "runtime output"
  pass_test "Regression case passed"
}

test_status_and_support_fail_cleanly_without_env() {
  announce_test "Regression case"

  local status_output="$TEST_TMP_ROOT/status-missing-env.out"
  local support_output="$TEST_TMP_ROOT/support-missing-env.out"

  rm -f -- "$ROOT_DIR/.env.dev" "$ROOT_DIR/.env.prod"

  if run_command_capture "$status_output" bash "$SCRIPT_DIR/status-report.sh" dev --json; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$status_output" "Missing $ROOT_DIR/.env.dev" "runtime output"
  assert_file_not_contains "$status_output" "unbound variable" "runtime output"

  if run_command_capture "$support_output" bash "$SCRIPT_DIR/support-bundle.sh" prod; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$support_output" "Missing $ROOT_DIR/.env.prod" "runtime output"
  assert_file_not_contains "$support_output" "unbound variable" "runtime output"
  restore_repo_env_files
  pass_test "Regression case passed"
}

test_contour_commands_require_explicit_contour() {
  announce_test "Regression case"
  local output_file="$TEST_TMP_ROOT/explicit-contour.out"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/backup.sh" --no-stop; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "You must explicitly pass dev or prod as the first argument" "runtime output"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/status-report.sh" --json; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "You must explicitly pass dev or prod as the first argument" "runtime output"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/contour-overview.sh"; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "You must explicitly pass dev or prod as the first argument" "runtime output"

  if run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" up -d; then
    fail_test "Regression case failed"
  fi
  assert_file_contains "$output_file" "Command up requires an explicit dev or prod contour" "runtime output"

  pass_test "Regression case passed"
}

test_doctor_json_reports_missing_env_files() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/doctor-missing-env.out"

  rm -f -- "$ROOT_DIR/.env.dev" "$ROOT_DIR/.env.prod"

  set +e
  bash "$SCRIPT_DIR/doctor.sh" all --json >"$output_file" 2>&1
  local status=$?
  set -e

  [[ $status -ne 0 ]] || fail_test "Regression case failed"
  assert_file_contains "$output_file" "\"target_scope\": \"all\"" "runtime output"
  assert_file_contains "$output_file" "[prod] Env file for contour not found" "runtime output"
  assert_file_contains "$output_file" "[dev] Env file for contour not found" "runtime output"
  restore_repo_env_files
  pass_test "Regression case passed"
}

test_cli_help_contract() {
  announce_test "Regression case"

  local output_file="$TEST_TMP_ROOT/cli-help.out"
  local script
  local scripts=(
    backup-audit.sh
    backup-catalog.sh
    backup.sh
    bootstrap.sh
    contour-overview.sh
    docker-cleanup.sh
    doctor.sh
    espo.sh
    migrate-backup.sh
    regression-test.sh
    restore-db.sh
    restore-drill.sh
    restore-files.sh
    rollback.sh
    smoke-test.sh
    stack.sh
    status-report.sh
    support-bundle.sh
    update.sh
    verify-backup.sh
  )

  rm -f -- "$ROOT_DIR/.env.dev" "$ROOT_DIR/.env.prod"

  for script in "${scripts[@]}"; do
    if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/$script" --help; then
      fail_test "Regression case failed"
    fi
    assert_file_contains "$output_file" "Usage:" "runtime output"
  done

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" help regression; then
    fail_test "Regression case failed"
  fi

  assert_file_contains "$output_file" "Usage: ./scripts/regression-test.sh" "runtime output"
  restore_repo_env_files
  pass_test "Regression case passed"
}
