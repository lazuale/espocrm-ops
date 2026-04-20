# Canonical Backup Migration Flow Uses `migrate`

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The retained product keeps migration as one of only four operator-facing
commands. That makes it important to keep migration execution fully Go-owned
instead of letting shell wrappers or sibling command names become controllers.

The retained migration path must own:

- source backup selection
- compatibility validation between contours
- restore sequencing into the target contour
- canonical warnings, failures, JSON, and exit codes

## Decision

Keep migration on one canonical public Go command, `migrate`, backed by the Go
migration execution usecase.

`migrate` is the authoritative owner of the real execution path:

- source contour env resolution and source backup root discovery
- automatic and explicit source backup selection, including DB/files pairing and partial selection modes
- migration compatibility validation between source and target contours
- target operation preflight, lock acquisition, and runtime preparation
- direct database and files restore execution through the Go restore usecases
- target contour restart handling, warnings, failure attribution, and canonical JSON/text result output

`scripts/migrate.sh` may keep the positional `<from> <to>` shell UX, but it
must delegate immediately to `espops migrate ...` and must not own substantive
migration sequencing or fallback behavior.

## Consequences

- The real migration path is now primarily Go-owned instead of shell
  orchestrated.
- The retained product surface stays aligned with the operator noun: `migrate`.
- Backup and restore building blocks remain focused subpaths, but they are not
  parallel controllers for migration.

## Rules

- Do not reintroduce meaningful migration sequencing into `scripts/migrate.sh`.
- Keep source selection, pairing validation, warnings, and failure attribution
  in the Go `migrate` command/usecase.
- Extend the Go migration usecase when new migration phases or guardrails need to appear instead of rebuilding controller logic in shell.
