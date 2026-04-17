{
  "name": "json_contract",
  "version": 1,
  "canonical_fixture_glob": "internal/cli/testdata/*.json",
  "shell_parse_smoke": [
    "scripts/doctor.sh --json",
    "scripts/status-report.sh --json",
    "scripts/backup-audit.sh --json",
    "scripts/backup-catalog.sh --json",
    "scripts/contour-overview.sh --json"
  ],
  "rules": [
    "all governed json files must parse cleanly",
    "shell json must not contaminate stdout with lock or info lines",
    "shell-generated json is transitional and non-canonical unless promoted",
    "non-canonical shell json must say machine_contract false",
    "shell passthrough json must not construct its own json envelope"
  ]
}
