# Canonical Restore Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0024, operators already had Go-owned backup execution, verification, inventory, inspection, update, rollback, migrate-backup, and operation reporting.

The next destructive recovery path still remained split:

- `scripts/restore-db.sh` and `scripts/restore-files.sh` still owned the real restore sequencing for contour restores
- shell still owned stop/start handling, emergency recovery-point creation, destructive approval flow, and files permission reconciliation
- Go-owned restore usecases existed for artifact-level database and files restore work, but shell remained the true controller for the operator-facing restore path
- that left parallel restore execution semantics and made shell the effective source of warning and failure attribution for the main restore flow

That split violated the repository direction that canonical execution, machine contract, exit-code behavior, and result reporting belong to Go.

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

`scripts/restore-db.sh` and `scripts/restore-files.sh` remain only as thin compatibility wrappers. They may keep the legacy shell UX shapes, but they must delegate immediately to `espops restore ...` for contour restore execution and must not own substantive restore sequencing or fallback behavior.

## Consequences

- The real contour restore path is now primarily Go-owned instead of shell-orchestrated.
- The contract surface changes in this pass because the public `restore` command joins the governed CLI surface and gains its own JSON fixture/baseline entry.
- The shell debt baseline shrinks because restore planning, runtime orchestration, and permission reconciliation no longer live in the restore shell scripts.
- The lower-level `restore-db` and `restore-files` Go commands remain available for direct artifact-oriented compatibility use, but they are no longer parallel controllers for the main operator restore flow.

## Rules

- Do not reintroduce meaningful restore sequencing into `scripts/restore-db.sh` or `scripts/restore-files.sh`.
- Keep source resolution, warnings, failure attribution, and canonical machine output in the Go `restore` command/usecase.
- Extend the Go restore execution path when new restore phases or guardrails are needed instead of rebuilding controller logic in shell.
