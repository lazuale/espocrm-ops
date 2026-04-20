# Canonical Restore Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The retained product keeps `restore` as one of only four operator-facing
commands. Restore therefore needs one canonical controller instead of separate
shell entrypoints for database and files work.

The retained restore path must own:

- manifest and direct-artifact source resolution
- pre-restore safety snapshot behavior
- destructive runtime preparation and restart handling
- database and files restore sequencing
- canonical warnings, failures, JSON, and exit codes

## Decision

Introduce a canonical public Go command, `restore`, backed by a Go restore execution usecase.

`restore` is now the authoritative owner of the real contour restore path:

- contour preflight, env resolution, and operational lock handling
- manifest-backed and direct backup source resolution for restore execution
- emergency recovery-point snapshot behavior before destructive restore work
- runtime preparation, including database readiness and application-service stop handling
- database and files restore sequencing through the existing Go restore building blocks
- runtime permission reconciliation for restored files storage
- post-restore contour return behavior, warnings, failure attribution, and canonical JSON/text result output

`scripts/restore.sh` remains only a thin wrapper around `espops restore ...`
and must not own substantive restore sequencing or fallback behavior.

## Consequences

- The real contour restore path is now primarily Go-owned instead of shell-orchestrated.
- The contract surface changes in this pass because the public `restore` command joins the governed CLI surface and gains its own JSON fixture/baseline entry.
- The shell debt baseline shrinks because restore planning, runtime
  orchestration, and permission reconciliation no longer live in shell.
- The operator-facing product surface stays on the single noun `restore`.

## Rules

- Do not reintroduce meaningful restore sequencing into `scripts/restore.sh`.
- Keep source resolution, warnings, failure attribution, and canonical machine output in the Go `restore` command/usecase.
- Extend the Go restore execution path when new restore phases or guardrails are needed instead of rebuilding controller logic in shell.
