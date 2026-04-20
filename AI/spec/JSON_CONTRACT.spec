{
  "name": "json_contract",
  "version": 1,
  "canonical_fixture_glob": "internal/cli/testdata/*.json",
  "shell_parse_smoke": [
    "scripts/doctor.sh --json"
  ],
  "rules": [
    "all governed json files must parse cleanly",
    "shell json must not contaminate stdout with lock or info lines",
    "shell-generated json is transitional and non-canonical unless promoted",
    "shell passthrough json must not construct its own json envelope"
  ]
}
