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
  "non_canonical_shell_json": [],
  "passthrough_shell_json": [
    "scripts/doctor.sh --json"
  ],
  "frozen_shell_debt": {}
}
