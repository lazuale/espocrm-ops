{
  "name": "contract",
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
  "rules": [
    "machine contract must not be derived from prose",
    "machine contract must not be derived from shell text parsing",
    "contract drift requires baseline refresh",
    "contract semantics change requires ADR",
    "command metadata drift must stay visible in the baseline"
  ]
}
