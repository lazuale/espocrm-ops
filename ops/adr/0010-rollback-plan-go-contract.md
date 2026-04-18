# Rollback Dry-Run Uses the Canonical Go Planning Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0003, rollback target auto-selection already depended on the Go `verify-backup` contract.

The standard rollback entrypoint still lacked a safe operator-facing planning mode:

- `scripts/rollback.sh` owned the user-facing rollback flow, but there was no canonical Go dry-run contract describing the selected rollback target, prerequisites, or planned restore actions
- operators could not inspect whether rollback target selection, emergency recovery-point creation, database/files restore, and contour restart would run before any stateful action
- adding rollback dry-run logic directly in shell would have created a second planning path beside the Go-owned rollback-selection, backup/restore, and contract stack

## Decision

Introduce a canonical public Go command, `rollback-plan`, backed by a Go rollback planning usecase.

`rollback-plan` is now the authoritative owner of rollback dry-run planning:

- resolved contour, project, compose file, and env file selection
- automatic or explicit rollback target resolution through the existing Go-owned backup verification stack
- read-only prerequisite inspection for runtime paths, locks, Docker/Compose access, and rollback-specific restore readiness
- rollback-specific step planning for target selection, runtime preparation, emergency recovery-point creation, database restore, files restore, and contour restart/readiness
- explicit `would_run`, `skipped`, `blocked`, and `unknown` step status reporting
- canonical JSON, error, and exit-code behavior for blocked plans

`scripts/rollback.sh` remains the operator compatibility entrypoint, but `--dry-run` is now a thin passthrough to the Go command instead of a shell-owned planner.

## Consequences

- Operators can inspect the real rollback path safely before any state mutation.
- The rollback dry-run contract now shares the Go-owned rollback-selection and restore boundaries instead of creating a parallel shell planner.
- The machine contract surface changes in this pass because the public `rollback-plan` command and its JSON fixture join the governed baseline.

## Rules

- Do not add shell-owned rollback planning logic beyond thin argument passthrough.
- Extend the Go `rollback-plan` usecase when new rollback phases or prerequisites need to appear in dry-run output.
- Keep blocked-plan JSON and exit-code behavior aligned with the canonical Go CLI contract.
