#!/usr/bin/env bash
set -Eeuo pipefail

# Local regression suite for operational shell scenarios.
# The launcher intentionally stays thin: shared helpers and the test suites
# live in `scripts/tests/`, so the test layer does not turn into one monolith.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/tests/lib.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/tests/suites/backup_restore.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/tests/suites/ops_cli.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/tests/suites/contracts.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/regression-test.sh

This script runs the local regression suite for repository shell scenarios.
EOF
}

main() {
  cd "$ROOT_DIR"

  if [[ $# -gt 0 ]]; then
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      *)
        usage >&2
        die "Unknown argument: $1"
        ;;
    esac
  fi

  backup_repo_env_files

  test_env_leakage_is_blocked
  test_load_env_rejects_insecure_env_files
  test_load_env_rejects_contour_mismatch
  test_load_env_supports_legacy_contour_detection_by_filename
  test_load_env_rejects_shell_substitutions
  test_maintenance_lock_is_global_per_contour
  test_append_trap_preserves_all_registered_handlers
  test_json_escape_escapes_control_chars
  test_lock_owner_is_alive_uses_ps_fallback_without_proc_entry
  test_collect_lock_entries_marks_legacy_metadata_lock_as_unverified
  test_collect_lock_entries_detects_stale_flock_metadata
  test_operation_lock_rejects_unverified_legacy_metadata
  test_maintenance_lock_rejects_unverified_legacy_metadata
  test_operation_lock_serializes_toolkit_steps
  test_backup_audit_requires_coherent_set
  test_backup_audit_can_skip_checksum_verification
  test_backup_audit_supports_scope_switches_and_age_overrides
  test_verify_backup_selects_coherent_pair
  test_verify_backup_supports_partial_explicit_selection
  test_backup_catalog_ready_only_selects_verified_set
  test_backup_catalog_supports_json_limit_and_latest_only
  test_directory_size_human_tolerates_du_errors
  test_safe_remove_tree_uses_docker_fallback_for_permission_errors
  test_safe_remove_tree_rejects_non_permission_failures
  test_set_env_value_preserves_special_chars
  test_next_unique_stamp_waits_past_busy_second
  test_backup_releases_maintenance_lock
  test_backup_rejects_empty_selection
  test_backup_files_only_no_stop_runs_without_docker
  test_backup_files_fallbacks_to_docker_helper
  test_restore_files_supports_absolute_storage_path
  test_restore_files_reconciles_permissions_after_replace
  test_restore_files_fails_when_permission_reconcile_fails
  test_restore_files_fallbacks_to_docker_helper
  test_restore_files_rejects_unsafe_archive_layout
  test_restore_db_takes_snapshot_by_default
  test_restore_files_takes_snapshot_by_default
  test_restore_db_requires_force_and_prod_confirmation
  test_restore_files_requires_force_and_prod_confirmation
  test_restore_drill_selects_complete_set
  test_restore_drill_rejects_equal_ports
  test_rollback_delegates_auto_selection_to_go_execution
  test_rollback_delegates_manual_selection_flags_to_go_execution
  test_rollback_defers_destructive_confirmation_to_go
  test_migrate_backup_delegates_to_go_execution
  test_migrate_backup_defers_destructive_confirmation_to_go
  test_update_can_skip_optional_steps
  test_update_propagates_go_runtime_timeout_failure
  test_update_dry_run_delegates_to_go_update_plan
  test_rollback_dry_run_delegates_to_go_rollback
  test_backup_delegates_to_go_backup_exec
  test_backup_reuses_inherited_shell_context
  test_smoke_test_can_keep_artifacts
  test_restore_drill_supports_explicit_selection_skip_probe_and_keep_artifacts
  test_status_report_writes_output_files
  test_doctor_does_not_require_ripgrep_for_own_published_ports
  test_doctor_all_switches_between_contours
  test_espo_cli_dispatches_commands
  test_contour_overview_runs_read_only_checks
  test_support_bundle_redacts_secrets
  test_support_bundle_supports_tail_and_default_output
  test_status_and_support_fail_cleanly_without_env
  test_contour_commands_require_explicit_contour
  test_doctor_json_reports_missing_env_files
  test_doctor_all_rejects_migration_contract_drift
  test_cli_help_contract
  test_toolkit_layer_boundaries
  test_regression_helpers_preserve_repo_file_modes
  test_ci_guards_repo_local_artifacts
  test_single_workflow_path_is_enforced
  test_docker_daemon_preflight_is_fail_fast
  test_docker_cleanup_supports_explicit_report_dir
  test_docker_cleanup_rejects_invalid_duration

  echo
  echo "Regression suite completed successfully"
}

main "$@"
