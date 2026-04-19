{
  "name": "contract_surface",
  "version": 1,
  "canonical": {
    "json": [
      "espops --json"
    ],
    "exit_codes": [
      "internal/contract/exitcode",
      "internal/cli/errors.go"
    ],
    "fixtures": [
      "internal/cli/testdata/*.golden.json"
    ]
  },
  "non_canonical_shell_json": [
    "scripts/doctor.sh --json",
    "scripts/status-report.sh --json",
    "scripts/contour-overview.sh --json"
  ],
  "passthrough_shell_json": [
    "scripts/backup-audit.sh --json",
    "scripts/backup-catalog.sh --json"
  ],
  "frozen_shell_debt": {
    "scripts/backup-audit.sh": [
      "run_espops \"${args[@]}\" 2>&1"
    ],
    "scripts/lib/artifacts.sh": [
      "latest_complete_backup_group_key",
      "matching_db_backup_for_files",
      "matching_files_backup_for_db"
    ],
    "scripts/restore-db.sh": [
      "write_restore_db_plan_reports("
    ],
    "scripts/restore-drill.sh": [
      "json_extract_string_field(",
      "latest_complete_backup_group_key",
      "matching_db_backup_for_files",
      "matching_files_backup_for_db"
    ],
    "scripts/restore-files.sh": [
      "write_restore_files_plan_reports("
    ],
    "scripts/rollback.sh": [
      "json_extract_string_field(",
      "latest_complete_backup_group_key",
      "select_latest_valid_backup_set(",
      "write_rollback_plan_reports("
    ],
    "scripts/verify-backup.sh": [
      "matching_db_backup_for_files",
      "matching_files_backup_for_db"
    ]
  }
}
