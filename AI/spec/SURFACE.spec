{
  "name": "surface",
  "version": 1,
  "canonical_owner": "Go CLI",
  "canonical_paths": [
    "cmd/espops",
    "internal/contract",
    "internal/cli",
    "internal/cli/testdata"
  ],
  "baseline_paths": [
    "cmd/espops",
    "internal/contract",
    "internal/cli"
  ],
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
  "json_fixture_glob": "internal/cli/testdata/*.json",
  "shell_parse_smoke": [
    "scripts/doctor.sh --json"
  ],
  "non_canonical_shell_json": [],
  "passthrough_shell_json": [
    "scripts/doctor.sh --json"
  ]
}
