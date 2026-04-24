# Architecture

## Authority

Repository authority is:

1. `AGENTS.md`
2. `ARCHITECTURE.md` and `MICRO_MONOLITHS.md`
3. Go code under `cmd/espops/` and `internal/`
4. `Makefile`
5. `.github/workflows/ci.yml`

## Product

The shipped product is exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

There is one production path.
There is no second runtime.
There is no namespaced operator path.

## Retained Graph

Only this internal graph is valid:

- `cmd/espops` -> `internal/cli`
- `internal/cli` -> `internal/config`, `internal/ops`, `internal/runtime`
- `internal/ops` -> `internal/config`, `internal/manifest`, `internal/runtime`
- `internal/config` -> stdlib only
- `internal/manifest` -> stdlib only
- `internal/runtime` -> stdlib only

No other production package family is allowed under `internal/`.

## Ownership

- `cmd/espops/` owns process startup and handoff into the root command only.
- `internal/cli/` owns commands, flags, JSON envelopes, and exit mapping.
- `internal/config/` owns env parsing, path resolution, and config validation.
- `internal/ops/` owns backup, verify, restore, migrate, and doctor workflows.
- `internal/runtime/` owns Docker Compose and MariaDB execution, plus the only production shell and `os.Environ()` seam.
- `internal/manifest/` owns manifest validation and artifact path resolution.

## Constraints

- No compatibility shim.
- No hidden fallback.
- No auto-repair or auto-normalization.
- No silent recovery.
- No shell-owned validation or recovery semantics outside `internal/runtime/docker.go`.
- No new product surface without updating authority docs first.

If correctness is ambiguous, fail.
