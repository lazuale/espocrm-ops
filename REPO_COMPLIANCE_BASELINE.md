# Repo Compliance Baseline

Accepted baseline date: `2026-04-24`.

This repository now keeps only the retained v3 product root.

## Accepted Product Surface

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

## Accepted Production Layout

- `cmd/espops/main.go`
- `internal/v3/cli/*.go`
- `internal/v3/config/config.go`
- `internal/v3/ops/*.go`
- `internal/v3/runtime/docker.go`
- `internal/v3/manifest/manifest.go`

No other production package family under `internal/` is accepted.

## Accepted Shell And Env Surface

- production shell execution lives only in `internal/v3/runtime/docker.go`
- production `os.Environ()` usage lives only in `internal/v3/runtime/docker.go`
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
