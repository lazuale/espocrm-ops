# Canonical Update Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0009, update dry-run planning was Go-owned, and earlier ADRs had already moved backup, runtime apply, and shared preflight segments behind Go boundaries.

The real update execution path still remained split:

- `scripts/update.sh` still sequenced the operator-visible flow
- shell still decided whether to run doctor, recovery-point creation, runtime apply, post-update readiness, and dry-run delegation
- Go owned helper segments, but shell remained the effective controller for the canonical update run
- that left parallel execution stacks for the same user-facing operation

That split violated the repository direction that canonical execution, machine contract, failure attribution, and result reporting belong to Go.

## Decision

Introduce a canonical public Go command, `update`, backed by a Go update execution usecase.

`update` is now the authoritative owner of the real execution path:

- shared operation preflight, env resolution, and lock acquisition for the real update run
- doctor sequencing inside the update flow
- pre-update recovery-point creation through the Go backup/update contract
- runtime apply and readiness sequencing through the Go runtime contract
- step-by-step status reporting, warnings, failure attribution, and JSON/text result output for the full update execution
- `--dry-run` delegation onto the Go planning contract from the public `update` command

`scripts/update.sh` remains only as a thin compatibility wrapper. It may parse the first contour token and forward bootstrap arguments, but it must delegate immediately to `espops update ...` and must not own substantive sequencing or fallback behavior.

## Consequences

- The real `update` path is now primarily Go-owned instead of being shell-orchestrated.
- The update contract surface changes in this pass because the public `update` command becomes part of the governed CLI surface and gains its own JSON fixture/baseline entry.
- `run-operation`, `update-backup`, and `update-runtime` remain valid internal building blocks, but they are no longer parallel user-facing execution stacks for the main update path.

## Rules

- Do not reintroduce meaningful update sequencing into `scripts/update.sh`.
- Extend the Go `update` usecase when new update phases, warnings, or failure cases need to appear.
- If doctor runs under an inherited update operation context, Go must account for inherited lock ownership instead of forcing shell to work around it.
