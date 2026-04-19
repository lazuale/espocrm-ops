# Canonical Restore-Drill Execution Flow Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0025, the repository already had Go-owned backup execution, verification, inventory and inspection, update, rollback, migrate-backup, and contour restore execution.

The remaining recovery drill path still stayed shell-first:

- `scripts/restore-drill.sh` still owned the real restore-drill controller flow
- shell still owned backup-set selection, explicit-vs-automatic source resolution, temporary drill contour shaping, runtime sequencing, readiness probing, warning emission, and failure attribution
- the restore drill path still delegated core operator behavior through multiple shell steps, which left shell as the effective authority for the recovery drill experience
- that kept a parallel restore-drill execution stack even though the repository direction requires canonical execution, machine contract, exit-code behavior, and result reporting to live in Go

## Decision

Introduce a canonical public Go command, `restore-drill`, backed by a Go restore-drill execution usecase.

`restore-drill` is now the authoritative owner of the real operator drill flow:

- source contour preflight, env resolution, and shared operation locking
- automatic latest-complete backup selection for drills
- explicit backup selection with deterministic matching of the missing database or files artifact
- temporary drill contour env generation, port derivation and validation, cleanup, and runtime directory preparation
- temporary database start, restore sequencing, permission reconciliation, runtime return, readiness waits, and final HTTP probe behavior
- warning emission, failure attribution, canonical JSON/text output, and drill report generation

`scripts/restore-drill.sh` remains only as a thin compatibility wrapper. It may keep the legacy shell entrypoint shape, but it must delegate immediately to `espops restore-drill ...` and must not own substantive restore-drill sequencing, selection, or reporting logic.

## Consequences

- The real restore-drill path is now primarily Go-owned instead of shell-orchestrated.
- The contract surface changes in this pass because the public `restore-drill` command joins the governed CLI surface and gains its own JSON fixture and baseline entry.
- The shell debt baseline shrinks because restore-drill selection, contour orchestration, and report generation no longer live in `scripts/restore-drill.sh`.
- Compatibility scripts and `scripts/espo.sh` continue to expose the legacy operator entrypoint, but the shell boundary is limited to argument forwarding and env-file passthrough.

## Rules

- Do not reintroduce meaningful restore-drill sequencing into `scripts/restore-drill.sh`.
- Keep backup selection, temporary contour setup, readiness/probe outcomes, warnings, failure attribution, and canonical machine output in the Go `restore-drill` command/usecase.
- Extend the Go restore-drill flow when new recovery-drill phases or guardrails are needed instead of rebuilding controller logic in shell.
