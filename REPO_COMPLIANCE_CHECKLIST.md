# Repo Compliance Checklist

Use this checklist when the retained graph or operator behavior changes.

## Product Surface

- `espops` exposes only `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- `espops v3` is not a supported operator path.
- There is no compatibility shim or fallback to removed legacy code.

## Retained Layout

- `cmd/espops/` exists and owns only the entrypoint.
- `internal/` contains only `internal/v3/cli`, `internal/v3/config`, `internal/v3/ops`, `internal/v3/runtime`, and `internal/v3/manifest`.
- Removed legacy package families stay removed.

## Ownership

- `internal/v3/cli/` owns command wiring and JSON envelopes.
- `internal/v3/config/` owns env-file loading and config validation.
- `internal/v3/ops/` owns retained workflow semantics and post-checks.
- `internal/v3/runtime/` owns Docker Compose and MariaDB process execution.
- `internal/v3/manifest/` owns manifest validation and artifact path resolution.

## Explicit Seams

- `os.Environ()` appears only in `internal/v3/runtime/docker.go`.
- `exec.Command` or `exec.CommandContext` appears only in `internal/v3/runtime/docker.go`.
- No hidden shell-owned validation, recovery, or path switching exists outside the runtime monolith.

## Docs

- `ARCHITECTURE.md` matches the retained package graph.
- `MICRO_MONOLITHS.md` matches the retained ownership and caller map.
- `README.md` and `CONTRIBUTING.md` match the shipped command surface and repo layout.
- Compliance docs describe the current retained world, not the deleted legacy one.

## Proof

- Focused scenario tests cover the changed behavior.
- `make ci` passes before repository health is claimed.
