# Doctor Uses the Canonical Go Readiness Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The repository keeps `doctor` as the single readiness surface for the retained
backup and recovery product.

That readiness path must stay canonical because:

- `scripts/doctor.sh` owned env validation, path checks, Docker/Compose probing, lock inspection, and cross-contour compatibility checks
- the readiness report was not backed by the canonical Go command/usecase/contract stack
- the same readiness rules must stay aligned with the retained `backup`,
  `restore`, and `migrate` commands

## Decision

Introduce a canonical public Go command, `doctor`, backed by a Go readiness usecase.

`doctor` is now the authoritative owner of operational readiness validation:

- env/config resolution for `dev`, `prod`, and `all`
- actionable env contract validation for the current runtime settings
- writable path and free-space readiness checks
- shared-operation and contour-operation lock readiness checks
- Docker daemon and Compose availability checks
- per-contour `docker compose config` validation and safe runtime health probing
- cross-contour isolation and compatibility validation for `all`

`scripts/doctor.sh` remains only a thin wrapper that delegates immediately to
the Go command.

## Consequences

- Readiness validation now follows the canonical Go JSON, error, and exit-code contract.
- Stateful backup and recovery commands build on one readiness implementation
  instead of a parallel shell path.
- The machine contract surface changes in this pass because the public `doctor` command is added to the Go CLI and its JSON fixture joins the governed baseline.

## Rules

- Do not reintroduce shell-owned readiness validation into `scripts/doctor.sh`.
- Future readiness checks must extend the shared Go `doctor` usecase instead of adding parallel validation paths.
- Keep shell doctor output as a thin passthrough to the Go result.
