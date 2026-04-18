# Shared Operation Preflight Uses a Hidden Go Boundary

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0006, backup execution itself was Go-owned, but `scripts/backup.sh` and `scripts/update.sh` still owned the same outer execution preflight:

- env-file resolution
- strict env loading
- runtime-directory preparation
- shared operation locking
- contour maintenance locking

That left a cross-cutting operational control boundary in shell even though the destructive backup and update core had already moved toward the canonical Go contract stack.

## Decision

Introduce a hidden Go command, `run-operation`, backed by a shared Go operation execution usecase and platform preflight support.

`run-operation` is now the authoritative owner of shared backup/update execution preconditions:

- resolve the effective env file for the requested contour
- parse and validate the env file as strict dotenv data
- prepare runtime and backup directories
- acquire the shared repository operation lock
- acquire the contour maintenance lock
- execute the remaining shell body under inherited lock state

`scripts/backup.sh` and `scripts/update.sh` keep their existing operator-facing shell bodies, but they no longer own env/lock setup directly.

## Consequences

- Shared env/lock control is no longer duplicated in both outer wrappers.
- `backup` and `update` now cross a canonical Go-owned preflight boundary before any remaining shell choreography runs.
- The machine contract surface changes in this pass because `run-operation` is added as a hidden Go command and the shell debt baseline changes as the two wrappers become thinner.

## Rules

- Do not reintroduce shared env loading or lock acquisition into `scripts/backup.sh` or `scripts/update.sh`.
- Future backup/update shell reductions should continue to pass through `run-operation` until the remaining outer choreography is also Go-owned.
- Child scripts executed through `run-operation` must rely on inherited lock state instead of reacquiring their own outer wrapper locks.
