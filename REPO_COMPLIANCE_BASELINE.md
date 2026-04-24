# Repo Compliance Baseline

Accepted baseline date: `2026-04-24`.

## Accepted Product Surface

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

## Accepted Production Layout

- `cmd/espops/main.go`
- `internal/cli/*.go`
- `internal/config/config.go`
- `internal/manifest/manifest.go`
- `internal/ops/*.go`
- `internal/runtime/docker.go`

No other production package family under `internal/` is accepted.

## Accepted Shell And Env Surface

- production shell execution lives only in `internal/runtime/docker.go`
- production `os.Environ()` usage lives only in `internal/runtime/docker.go`
- config loading comes from scope env files, not hidden process-env toggles

## Accepted Repository Health Path

- dependency checks against `./cmd/espops`
- focused scenario tests for changed behavior
- `make ci`

Any PR that changes this baseline must update:

- `ARCHITECTURE.md`
- `MICRO_MONOLITHS.md`
- `README.md`
- `CONTRIBUTING.md`
- `REPO_COMPLIANCE_CHECKLIST.md`
- this file
