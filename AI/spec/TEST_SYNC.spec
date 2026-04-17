{
  "name": "test_sync",
  "version": 1,
  "rules": [
    {
      "trigger_prefixes": [
        "AI/spec/CONTRACT_SURFACE.spec",
        "AI/spec/DOCS_SYNC.spec",
        "AI/spec/PACKAGE_POLICY.spec",
        "AI/spec/PR_BODY.spec"
      ],
      "requires_any_prefix": [
        "AI/compiled/"
      ]
    },
    {
      "trigger_prefixes": [
        "internal/contract/",
        "internal/cli/"
      ],
      "requires_any_prefix": [
        "AI/compiled/CONTRACT_BASELINE.json",
        "AI/compiled/JSON_CONTRACT_BASELINE/"
      ]
    },
    {
      "trigger_prefixes": [
        "scripts/doctor.sh",
        "scripts/status-report.sh",
        "scripts/backup-audit.sh",
        "scripts/backup-catalog.sh",
        "scripts/contour-overview.sh",
        "scripts/lib/locks.sh"
      ],
      "requires_any_prefix": [
        ".github/workflows/ai-governance.yml",
        "AI/compiled/"
      ]
    },
    {
      "trigger_prefixes": [
        "scripts/restore-db.sh",
        "scripts/restore-files.sh",
        "scripts/rollback.sh",
        "scripts/migrate-backup.sh",
        "scripts/restore-drill.sh",
        "scripts/verify-backup.sh",
        "scripts/lib/artifacts.sh"
      ],
      "requires_any_prefix": [
        "AI/compiled/SHELL_DEBT_BASELINE.json"
      ]
    }
  ]
}
