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
        "scripts/backup.sh",
        "scripts/restore.sh",
        "scripts/migrate.sh",
        "scripts/espo.sh",
        "scripts/regression-test.sh",
        "scripts/lib/common.sh"
      ],
      "requires_any_prefix": [
        ".github/workflows/ai-governance.yml",
        "AI/compiled/"
      ]
    }
  ]
}
