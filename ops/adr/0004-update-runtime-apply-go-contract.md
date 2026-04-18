# Update Runtime Apply Moves to the Go Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The `update` flow still owned its runtime choreography entirely in shell after the rollback-selection consolidation. The shell wrapper still decided and executed image pulling, stack restart, readiness waiting, and the final HTTP probe.

Those steps are safety-critical operational behavior and they sat outside the canonical Go command/contract stack even though they already form one coherent runtime-apply subpath inside `update`.

## Decision

The runtime apply segment of `update` moves behind a hidden Go CLI contract command, `update-runtime`.

Shell keeps the surrounding wrapper responsibilities that remain transitional in this pass: contour and env loading, pre-update status capture, optional doctor execution, pre-update backup orchestration, failure bundle capture, and post-update status capture. The Go command now owns the post-backup runtime execution boundary: optional image pull, `docker compose up -d`, sequential readiness waiting with one shared timeout budget, and the optional HTTP probe.

## Consequences

- `update` no longer owns docker compose pull/up and readiness waiting logic directly in shell.
- The canonical Go CLI contract now defines the runtime-apply JSON, error code, and exit behavior for this subpath.
- The shell wrapper becomes thinner without introducing a second runtime implementation.
- Remaining `update` shell ownership still exists around preflight, backup, reporting, and failure-handling choreography.

## Rules

- update runtime apply must flow through the Go `update-runtime` command;
- shell `update.sh` must not keep its own compose pull, stack-up, readiness wait, or HTTP probe implementation for this subpath;
- Go owns the timeout-budget and readiness semantics for update runtime apply;
- future `update` consolidation should delete more shell choreography rather than reintroduce runtime logic outside the Go contract.
