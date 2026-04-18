# shellcheck shell=bash

test_toolkit_layer_boundaries() {
  announce_test "Toolkit Layer Boundaries"

  local output_file="$TEST_TMP_ROOT/toolkit-boundaries.out"

  if ! run_command_capture "$output_file" bash -lc "
    set -Eeuo pipefail

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/stack.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/stack.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/stack.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/stack.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/stack.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/backup.sh'
    rg -Fq 'run-operation' '$SCRIPT_DIR/backup.sh'
    rg -Fq 'run_espops \"\${args[@]}\"' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'acquire_operation_lock backup' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'resolve_env_file' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'load_env' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'ensure_runtime_dirs' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'acquire_maintenance_lock backup' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'use_go_backend() {' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'write_backup_metadata_with_go() {' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'create_tar_archive ' '$SCRIPT_DIR/backup.sh'
    ! rg -Fq 'compose exec -T db' '$SCRIPT_DIR/backup.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/restore-db.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/restore-db.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/restore-db.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/restore-db.sh'
    rg -Fq 'wait_for_service_ready db' '$SCRIPT_DIR/restore-db.sh'
    rg -Fq 'run_go_restore_db() {' '$SCRIPT_DIR/restore-db.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/restore-files.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/restore-files.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/restore-files.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/restore-files.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/restore-files.sh'
    rg -Fq 'run_go_restore_files() {' '$SCRIPT_DIR/restore-files.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/restore-drill.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/restore-drill.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/restore-drill.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/restore-drill.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/restore-drill.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/rollback.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/rollback.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/rollback.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/rollback.sh'
    rg -Fq 'run_espops --json verify-backup --backup-root \"\$BACKUP_ROOT_ABS\"' '$SCRIPT_DIR/rollback.sh'
    ! rg -Fq 'use_go_backend() {' '$SCRIPT_DIR/rollback.sh'
    ! rg -q 'json_extract_string.*field\(\) \{' '$SCRIPT_DIR/rollback.sh'
    ! rg -Fq 'select_latest_valid_backup_set_go() {' '$SCRIPT_DIR/rollback.sh'
    ! rg -q 'latest_complete_backup_group_.*key' '$SCRIPT_DIR/rollback.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/migrate-backup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/migrate-backup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/migrate-backup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/migrate-backup.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/update.sh'
    rg -Fq 'run_espops \"\${args[@]}\"' '$SCRIPT_DIR/update.sh'
    rg -Fq 'update' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'acquire_operation_lock update' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'resolve_env_file' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'load_env' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'ensure_runtime_dirs' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'acquire_maintenance_lock update' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'run-operation' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'update-backup' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'update-runtime' '$SCRIPT_DIR/update.sh'
    ! rg -Fq -- '--backup-script' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'service_is_running db' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'run_repo_script \"\$SCRIPT_DIR/backup.sh\" \"\$ESPO_ENV\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'run_repo_script \"\$SCRIPT_DIR/status-report.sh\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'run_repo_script \"\$SCRIPT_DIR/doctor.sh\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'compose pull' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'wait_for_application_stack_with_shared_timeout' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'wait_for_service_ready_with_shared_timeout READINESS_TIMEOUT_BUDGET db \"update\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'http_probe \"\$SITE_URL\"' '$SCRIPT_DIR/update.sh'
    ! rg -Fxq 'compose up -d' '$SCRIPT_DIR/update.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/smoke-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/smoke-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/smoke-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/smoke-test.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/backup-audit.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/backup-audit.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/report.sh\"' '$SCRIPT_DIR/backup-audit.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/backup-audit.sh'
    rg -Fq 'run_espops \"\${args[@]}\"' '$SCRIPT_DIR/backup-audit.sh'
    ! rg -Fq 'report_add() {' '$SCRIPT_DIR/backup-audit.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/report.sh\"' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/doctor.sh'
    rg -Fq 'run_espops \"\${doctor_args[@]}\"' '$SCRIPT_DIR/doctor.sh'
    rg -Fq 'run_espops --json \"\${doctor_args[@]}\"' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'acquire_operation_lock doctor' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'resolve_env_file' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'load_env' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'ensure_runtime_dirs' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'report_add() {' '$SCRIPT_DIR/doctor.sh'
    ! rg -Fq 'file_mode_octal() {' '$SCRIPT_DIR/doctor.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/contour-overview.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/contour-overview.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/contour-overview.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/contour-overview.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/fs.sh\"' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'add_overview_section \"doctor\" \"Doctor\"' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'add_overview_section \"status\" \"Status\"' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'add_overview_section \"backup_audit\" \"Backup Audit\"' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'add_overview_section \"backup_catalog\" \"Latest Valid Backup\"' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'render_overview_json() {' '$SCRIPT_DIR/contour-overview.sh'
    rg -Fq 'emit_overview() {' '$SCRIPT_DIR/contour-overview.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/docker_cleanup.sh\"' '$SCRIPT_DIR/docker-cleanup.sh'
    ! rg -Fq 'cleanup_require_docker() {' '$SCRIPT_DIR/docker-cleanup.sh'
    ! rg -Fq 'cleanup_gather_network_candidates() {' '$SCRIPT_DIR/docker-cleanup.sh'
    ! rg -Fq 'REPORT_DIR=\"\$ROOT_DIR/../espocrm-data/backups/host/reports\"' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq 'DOCKER_CLEANUP_REPORT_DIR' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq -- '--report-dir PATH' '$SCRIPT_DIR/docker-cleanup.sh'
    rg -Fq 'XDG_STATE_HOME' '$SCRIPT_DIR/docker-cleanup.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/status-report.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/status-report.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/compose.sh\"' '$SCRIPT_DIR/status-report.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/status-report.sh'
    ! rg -Fq 'collect_lock_entries() {' '$SCRIPT_DIR/status-report.sh'

    rg -Fq 'source \"\$SCRIPT_DIR/lib/common.sh\"' '$SCRIPT_DIR/backup-catalog.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/lib/locks.sh\"' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/artifacts.sh\"' '$SCRIPT_DIR/backup-catalog.sh'
    rg -Fq 'run_espops \"\${args[@]}\"' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'register_group() {' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'checksum_status_for_file() {' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'collect_db_backups() {' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'collect_files_backups() {' '$SCRIPT_DIR/backup-catalog.sh'
    ! rg -Fq 'collect_manifests() {' '$SCRIPT_DIR/backup-catalog.sh'

    test -f '$ROOT_DIR/ops/env/.env.dev.example'
    test -f '$ROOT_DIR/ops/env/.env.prod.example'
    rg -Fq 'DB_STORAGE_DIR=../espocrm-data/runtime/dev/db' '$ROOT_DIR/ops/env/.env.dev.example'
    rg -Fq 'ESPO_STORAGE_DIR=../espocrm-data/runtime/dev/espo' '$ROOT_DIR/ops/env/.env.dev.example'
    rg -Fq 'BACKUP_ROOT=../espocrm-data/backups/dev' '$ROOT_DIR/ops/env/.env.dev.example'
    rg -Fq 'DB_STORAGE_DIR=../espocrm-data/runtime/prod/db' '$ROOT_DIR/ops/env/.env.prod.example'
    rg -Fq 'ESPO_STORAGE_DIR=../espocrm-data/runtime/prod/espo' '$ROOT_DIR/ops/env/.env.prod.example'
    rg -Fq 'BACKUP_ROOT=../espocrm-data/backups/prod' '$ROOT_DIR/ops/env/.env.prod.example'
    rg -Fq '.support.*/' '$ROOT_DIR/.gitignore'
    rg -Fq '.cache/' '$ROOT_DIR/.gitignore'
    ! rg -Fq 'storage/' '$ROOT_DIR/.gitignore'
    ! rg -Fq 'backups/' '$ROOT_DIR/.gitignore'
    ! rg -Fq '*.sql.gz' '$ROOT_DIR/.gitignore'
    ! rg -Fq '*.tar.gz' '$ROOT_DIR/.gitignore'
    ! rg -n '/home/febinet/code/docker' '$ROOT_DIR/AGENTS.md' '$ROOT_DIR/README.md' '$ROOT_DIR/CONTRIBUTING.md' '$ROOT_DIR/ops/adr'
    ! find '$ROOT_DIR' -maxdepth 1 -type f -name '.env.*.example' -print -quit | grep -q .

    rg -Fq 'source \"\$SCRIPT_DIR/tests/lib.sh\"' '$SCRIPT_DIR/regression-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/tests/suites/backup_restore.sh\"' '$SCRIPT_DIR/regression-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/tests/suites/ops_cli.sh\"' '$SCRIPT_DIR/regression-test.sh'
    rg -Fq 'source \"\$SCRIPT_DIR/tests/suites/contracts.sh\"' '$SCRIPT_DIR/regression-test.sh'
    ! rg -Fq 'create_mock_docker_cli() {' '$SCRIPT_DIR/regression-test.sh'
    ! rg -Fq 'test_backup_audit_requires_coherent_set() {' '$SCRIPT_DIR/regression-test.sh'

    rg -Fq 'create_mock_docker_cli() {' '$SCRIPT_DIR/tests/lib.sh'
    rg -Fq 'test_backup_audit_requires_coherent_set() {' '$SCRIPT_DIR/tests/suites/backup_restore.sh'
    rg -Fq 'test_update_can_skip_optional_steps() {' '$SCRIPT_DIR/tests/suites/ops_cli.sh'
    rg -Fq 'test_toolkit_layer_boundaries() {' '$SCRIPT_DIR/tests/suites/contracts.sh'

    ! rg -Fq 'source \"\$SCRIPT_DIR/lib/' '$SCRIPT_DIR/espo.sh'
    ! rg -Fq 'create_failure_bundle() {' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'create_failure_bundle() {' '$SCRIPT_DIR/rollback.sh'
    ! rg -Fq 'create_failure_bundle() {' '$SCRIPT_DIR/restore-drill.sh'
    ! rg -Fq 'manifest_txt_is_valid() {' '$SCRIPT_DIR/rollback.sh'
    ! rg -Fq 'manifest_json_is_valid() {' '$SCRIPT_DIR/rollback.sh'
    ! rg -Fq 'wait_for_application_stack() {' '$SCRIPT_DIR/update.sh'
    ! rg -Fq 'wait_for_application_stack() {' '$SCRIPT_DIR/rollback.sh'

    ! rg -Fq 'compose() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'lock_owner_is_alive() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'list_lock_files() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'acquire_maintenance_lock() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'sha256_file() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'safe_remove_tree() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'report_add() {' '$SCRIPT_DIR/lib/common.sh'
    rg -Fq 'run_espops() {' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'go build' '$SCRIPT_DIR/lib/common.sh'
    ! rg -Fq 'command_exists go' '$SCRIPT_DIR/lib/common.sh'

    rg -Fq 'compose() {' '$SCRIPT_DIR/lib/compose.sh'
    rg -Fq 'wait_for_application_stack() {' '$SCRIPT_DIR/lib/compose.sh'
    rg -Fq 'lock_owner_is_alive() {' '$SCRIPT_DIR/lib/locks.sh'
    rg -Fq 'collect_lock_entries() {' '$SCRIPT_DIR/lib/locks.sh'
    rg -Fq 'acquire_operation_lock() {' '$SCRIPT_DIR/lib/locks.sh'
    rg -Fq 'acquire_maintenance_lock() {' '$SCRIPT_DIR/lib/locks.sh'
    rg -Fq 'sha256_file() {' '$SCRIPT_DIR/lib/artifacts.sh'
    rg -Fq 'backup_file_checksum_status() {' '$SCRIPT_DIR/lib/artifacts.sh'
    rg -Fq 'backup_manifest_txt_is_valid() {' '$SCRIPT_DIR/lib/artifacts.sh'
    rg -Fq 'backup_manifest_json_is_valid() {' '$SCRIPT_DIR/lib/artifacts.sh'
    rg -Fq 'run_support_bundle_capture() {' '$SCRIPT_DIR/lib/artifacts.sh'
    rg -Fq 'safe_remove_tree() {' '$SCRIPT_DIR/lib/fs.sh'
    rg -Fq 'file_mode_octal() {' '$SCRIPT_DIR/lib/fs.sh'
    rg -Fq 'report_add() {' '$SCRIPT_DIR/lib/report.sh'
    rg -Fq 'cleanup_require_docker() {' '$SCRIPT_DIR/lib/docker_cleanup.sh'
  "; then
    fail_test "Toolkit layer boundaries were violated"
  fi

  pass_test "the dispatcher and libraries stay narrow and domain-specific"
}

test_regression_helpers_preserve_repo_file_modes() {
  announce_test "Regression Helper Preserves File Modes"

  local target_file="$TEST_TMP_ROOT/mode-target.sh"
  local replacement_file="$TEST_TMP_ROOT/mode-replacement.sh"
  local restored_mode

  cat > "$target_file" <<'EOF'
#!/usr/bin/env bash
echo original
EOF
  chmod 755 "$target_file"

  cat > "$replacement_file" <<'EOF'
#!/usr/bin/env bash
echo replacement
EOF
  chmod 644 "$replacement_file"

  replace_repo_file_temporarily "$replacement_file" "$target_file"
  restore_replaced_repo_files

  restored_mode="$(stat -c '%a' "$target_file")"
  [[ "$restored_mode" == "755" ]] || fail_test "restore_replaced_repo_files restored mode $restored_mode instead of 755"
  [[ -x "$target_file" ]] || fail_test "restore_replaced_repo_files lost the executable bit"

  pass_test "temporary repo-file replacement keeps mode bits intact"
}

test_ci_guards_repo_local_artifacts() {
  announce_test "CI Artifact Guardrails"

  local output_file="$TEST_TMP_ROOT/ci-artifact-guard.out"

  if ! run_command_capture "$output_file" bash -lc "
    set -Eeuo pipefail

    test \"\$(find '$ROOT_DIR/.github/workflows' -maxdepth 1 -type f | wc -l)\" -eq 1
    ! find '$ROOT_DIR/.github' -mindepth 1 -maxdepth 1 ! -name 'workflows' -print -quit | grep -q .
    test -f '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'concurrency:' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'cancel-in-progress: true' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'timeout-minutes: 60' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'workflow_dispatch:' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'actions/checkout@v6' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'actions/setup-go@v6' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'actions/setup-python@v6' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'persist-credentials: false' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'go-version-file: go.mod' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'apt-get install --yes --no-install-recommends shellcheck' '$ROOT_DIR/.github/workflows/ai-governance.yml'
    rg -Fq 'make check-full' '$ROOT_DIR/.github/workflows/ai-governance.yml'

    rg -Fq 'ai-refresh:' '$ROOT_DIR/Makefile'
    rg -Fq 'ai-check:' '$ROOT_DIR/Makefile'
    rg -Fq 'check-fast:' '$ROOT_DIR/Makefile'
    rg -Fq 'check-full:' '$ROOT_DIR/Makefile'
    rg -Fq 'ci: check-fast' '$ROOT_DIR/Makefile'

    rg -Fq '[AGENTS.md](AGENTS.md)' '$ROOT_DIR/README.md'
    rg -Fq 'AI/spec/*' '$ROOT_DIR/README.md'
    rg -Fq 'make ai-refresh' '$ROOT_DIR/README.md'
    rg -Fq 'make ai-check' '$ROOT_DIR/README.md'
    rg -Fq 'make ci' '$ROOT_DIR/README.md'

    rg -Fq '[AGENTS.md](AGENTS.md)' '$ROOT_DIR/CONTRIBUTING.md'
    rg -Fq 'AI/spec/*' '$ROOT_DIR/CONTRIBUTING.md'
    rg -Fq 'make ai-refresh' '$ROOT_DIR/CONTRIBUTING.md'
    rg -Fq 'make ai-check' '$ROOT_DIR/CONTRIBUTING.md'
    rg -Fq 'make ci' '$ROOT_DIR/CONTRIBUTING.md'
    ! rg -Fq 'Archived human docs' '$ROOT_DIR/CONTRIBUTING.md'

    ! find '$ROOT_DIR' -maxdepth 1 -type f -name '*.md' ! -name 'AGENTS.md' ! -name 'README.md' ! -name 'CONTRIBUTING.md' -print -quit | grep -q .
    ! find '$ROOT_DIR/ops' -type f -name '*.md' ! -path '$ROOT_DIR/ops/adr/*' -print -quit | grep -q .
    rg -Fq 'AGENTS.md' '$SCRIPT_DIR/espo.sh'
  "; then
    fail_test "Governance and onboarding must collapse to one workflow and a minimal AI bootstrap"
  fi

  pass_test "workflow, Makefile, and onboarding point to one AI-authoritative path"
}

test_single_workflow_path_is_enforced() {
  announce_test "Single Workflow Path"

  local output_file="$TEST_TMP_ROOT/workflow-path.out"

  # shellcheck disable=SC2016
  if ! run_command_capture "$output_file" bash -lc '
    set -Eeuo pipefail

    root_dir="$1"
    entries="$(find "$root_dir/.github/workflows" -maxdepth 1 -type f | sort)"
    test "$entries" = "$root_dir/.github/workflows/ai-governance.yml"
    ! find "$root_dir/.github" -mindepth 1 -maxdepth 1 ! -name workflows -print -quit | grep -q .
    python3 "$root_dir/AI/generators/validate_specs.py"
  ' _ "$ROOT_DIR"; then
    fail_test "the repo must keep exactly one governance workflow path"
  fi

  pass_test "the workflow surface is reduced to one governance gate"
}
