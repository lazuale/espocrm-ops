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
    "scripts/doctor.sh --json"
  ],
  "passthrough_shell_json": [
    "scripts/backup-audit.sh --json",
    "scripts/backup-catalog.sh --json",
    "scripts/support-bundle.sh --json",
    "scripts/contour-overview.sh --json",
    "scripts/status-report.sh --json"
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
