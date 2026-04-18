# Canonical Rollback Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0010, rollback dry-run planning, target selection, and the rollback machine contract were already Go-owned.

The real rollback execution path still remained split:

- `scripts/rollback.sh` still owned the actual destructive sequencing
- shell still selected targets, enforced confirmations, prepared runtime state, invoked emergency snapshots, called restore scripts, and returned the contour to service
- Go owned rollback planning and restore building blocks, but shell remained the true controller for the user-facing rollback path
- that left parallel rollback execution stacks and made shell the effective source of failure attribution

That split violated the repository direction that canonical execution, machine contract, warnings, exit-code behavior, and result reporting belong to Go.

## Decision

Introduce a canonical public Go command, `rollback`, backed by a Go rollback execution usecase.

`rollback` is now the authoritative owner of the real execution path:

- shared operation preflight, env resolution, and lock acquisition for the real rollback run
- doctor sequencing and rollback-specific blocking logic
- rollback target selection, including automatic latest-valid selection and explicit pair handling
- runtime preparation, including DB startup when needed and application-service stop sequencing
- emergency recovery-point creation through the Go backup contract
- direct database and files restore execution through the Go restore usecases
- contour restart, readiness waits, HTTP probe handling, warnings, failure attribution, and JSON/text result output for the full rollback run
- `--dry-run` delegation onto the Go rollback planning contract from the public `rollback` command
- destructive execution confirmation guardrails as part of the Go command surface instead of shell prechecks

`scripts/rollback.sh` remains only as a thin compatibility wrapper. It may parse the first contour token and forward bootstrap arguments, but it must delegate immediately to `espops rollback ...` and must not own substantive rollback sequencing or fallback behavior.

## Consequences

- The real `rollback` path is now primarily Go-owned instead of shell-orchestrated.
- The rollback contract surface changes in this pass because the public `rollback` command becomes part of the governed CLI surface and gains its own JSON fixture/baseline entry.
- `rollback-plan`, `restore-db`, and `restore-files` remain valid focused building blocks, but they are no longer parallel controllers for the main rollback path.

## Rules

- Do not reintroduce meaningful rollback sequencing into `scripts/rollback.sh`.
- Extend the Go `rollback` usecase when new rollback phases, warnings, or failure cases need to appear.
- Keep destructive approval, failure attribution, and canonical rollback result reporting in Go.
