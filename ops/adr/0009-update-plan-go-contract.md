# Update Dry-Run Uses the Canonical Go Planning Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0008, readiness validation already had a canonical Go-owned boundary through `doctor`.

The standard update entrypoint still lacked a safe operator-facing planning mode:

- `scripts/update.sh` owned the user-facing update flow, but there was no canonical Go dry-run contract describing what the update would do before stateful work
- operators could not inspect whether doctor, recovery-point creation, runtime apply, and readiness checks would run without executing part of the update
- adding dry-run logic directly in shell would have created a second planning path beside the Go-owned doctor/update stack

## Decision

Introduce a canonical public Go command, `update-plan`, backed by a Go update planning usecase.

`update-plan` is now the authoritative owner of update dry-run planning:

- resolved contour, project, compose file, and env file selection
- doctor-backed read-only readiness inspection before stateful update work
- update-specific step planning for doctor, recovery-point creation, runtime apply, and runtime readiness
- explicit `would_run`, `skipped`, `blocked`, and `unknown` step status reporting
- canonical JSON, error, and exit-code behavior for blocked plans

`scripts/update.sh` remains the operator compatibility entrypoint, but `--dry-run` is now a thin passthrough to the Go command instead of a shell-owned planner.

## Consequences

- Operators can inspect the real update path safely before any state mutation.
- The update dry-run contract now shares the Go-owned doctor/update boundary instead of creating a parallel shell planner.
- The machine contract surface changes in this pass because the public `update-plan` command and its JSON fixture join the governed baseline.

## Rules

- Do not add shell-owned update planning logic beyond thin argument passthrough.
- Extend the Go `update-plan` usecase when new update phases or prerequisites need to appear in dry-run output.
- Keep blocked-plan JSON and exit-code behavior aligned with the canonical Go CLI contract.
