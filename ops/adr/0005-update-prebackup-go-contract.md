# Update Pre-Backup Step Uses a Hidden Go Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The `update` flow still kept its pre-update recovery-point step in shell. That step owned one meaningful operational decision:

- decide whether the `db` service must be started temporarily before backup
- wait for the database service to become ready
- invoke the backup flow for the update recovery point

After ADR 0004 moved runtime apply/readiness behind `update-runtime`, this pre-backup segment remained one of the highest-leverage outer `update` choreography gaps.

## Decision

Introduce a hidden Go command, `update-backup`, as the authoritative owner of the pre-update recovery-point segment.

`update-backup` now:

- checks the `db` service state through Go Docker/Compose primitives
- starts `db` when it is not already running
- waits for database readiness with the canonical Go timeout/error path
- invokes the backup step as one update-owned execution segment

`scripts/update.sh` no longer decides whether to start `db` or calls `backup.sh` directly for step 3. It delegates that segment through a single `run_espops update-backup ...` call.

## Consequences

- The `update` wrapper is thinner and no longer owns step-3 DB-start/backup orchestration.
- JSON/error/exit-code handling for the pre-update recovery-point segment now follows the Go contract path.
- The standalone public backup flow remains unchanged in this pass; only the `update`-specific orchestration boundary moved.

## Rules

- Further `update` consolidation should continue pulling outer choreography behind hidden Go commands instead of re-embedding decision logic in shell.
- Do not reintroduce direct shell-owned DB-start or direct `backup.sh` invocation into `scripts/update.sh` for the update recovery-point step.
