# Canonical Backup Migration Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0023, operators already had Go-owned backup execution, verification, inventory, inspection, rollback, update, and operation reporting.

One high-value destructive path still remained split:

- `scripts/migrate-backup.sh` still owned the real migration sequencing
- shell still selected source backups, paired DB and files archives, enforced the migration compatibility contract, and invoked restore scripts directly
- Go-owned backup and restore building blocks existed, but shell remained the true migration controller for the operator-facing path
- that left parallel migration execution semantics and made shell the effective source of warning and failure attribution

That split violated the repository direction that canonical execution, machine contract, exit-code behavior, and result reporting belong to Go.

## Decision

Introduce a canonical public Go command, `migrate-backup`, backed by a Go migration execution usecase.

`migrate-backup` is now the authoritative owner of the real execution path:

- source contour env resolution and source backup root discovery
- automatic and explicit source backup selection, including DB/files pairing and partial selection modes
- migration compatibility validation between source and target contours
- target operation preflight, lock acquisition, and runtime preparation
- direct database and files restore execution through the Go restore usecases
- target contour restart handling, warnings, failure attribution, and canonical JSON/text result output

`scripts/migrate-backup.sh` remains only as a thin compatibility wrapper. It may keep the legacy positional `<from> <to>` shell UX, but it must delegate immediately to `espops migrate-backup ...` and must not own substantive migration sequencing or fallback behavior.

## Consequences

- The real `migrate-backup` path is now primarily Go-owned instead of shell-orchestrated.
- The contract surface changes in this pass because the public `migrate-backup` command joins the governed CLI surface and gains its own JSON fixture/baseline entry.
- The shell debt baseline shrinks because migration selection and sequencing no longer live in `scripts/migrate-backup.sh`.
- Backup and restore building blocks remain focused subpaths, but they are no longer parallel controllers for the main migration flow.

## Rules

- Do not reintroduce meaningful migration sequencing into `scripts/migrate-backup.sh`.
- Keep source selection, pairing validation, warnings, and failure attribution in the Go `migrate-backup` command/usecase.
- Extend the Go migration usecase when new migration phases or guardrails need to appear instead of rebuilding controller logic in shell.
