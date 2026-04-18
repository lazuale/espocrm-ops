# Doctor Uses the Canonical Go Readiness Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0007, backup and update already crossed a shared Go-owned preflight boundary before stateful shell choreography ran.

One operator-facing prerequisite check still remained shell-owned:

- `scripts/doctor.sh` owned env validation, path checks, Docker/Compose probing, lock inspection, and cross-contour compatibility checks
- the readiness report was not backed by the canonical Go command/usecase/contract stack
- the same operational preconditions risked drifting between the doctor report and the Go-owned execution path used by stateful commands

## Decision

Introduce a canonical public Go command, `doctor`, backed by a Go readiness usecase.

`doctor` is now the authoritative owner of operational readiness validation:

- env/config resolution for `dev`, `prod`, and `all`
- actionable env contract validation for the current runtime settings
- writable path and free-space readiness checks
- shared-operation and maintenance-lock readiness checks
- Docker daemon and Compose availability checks
- per-contour `docker compose config` validation and safe runtime health probing
- cross-contour isolation and compatibility validation for `all`

`scripts/doctor.sh` becomes a thin compatibility wrapper that delegates to the Go command instead of owning the readiness logic itself.

## Consequences

- Readiness validation now follows the canonical Go JSON, error, and exit-code contract.
- Stateful operations and operator-facing diagnostics can build on one readiness implementation instead of a parallel shell path.
- The machine contract surface changes in this pass because the public `doctor` command is added to the Go CLI and its JSON fixture joins the governed baseline.

## Rules

- Do not reintroduce shell-owned readiness validation into `scripts/doctor.sh`.
- Future readiness checks must extend the shared Go `doctor` usecase instead of adding parallel validation paths.
- Non-canonical shell doctor output may wrap the Go result for compatibility, but it must not reconstruct the validation logic.
