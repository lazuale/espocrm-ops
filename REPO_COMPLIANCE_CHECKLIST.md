# Repo Compliance Checklist

Use this checklist when the retained graph or operator behavior changes.

## Product

- `espops` exposes only `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- Commands live at the `espops` root; there is no namespaced operator path.
- There is no compatibility shim or fallback to deleted code.

## Layout

- `cmd/espops/` owns only the entrypoint.
- `internal/` contains only `cli`, `config`, `manifest`, `ops`, and `runtime`.
- The removed nested retained-package namespace and deleted package families stay gone.

## Seams

- `os.Environ()` appears only in `internal/runtime/docker.go`.
- `exec.Command` or `exec.CommandContext` appears only in `internal/runtime/docker.go`.
- No hidden shell-owned validation, recovery, or path switching exists outside the runtime layer.

## Docs

- `ARCHITECTURE.md` matches the retained graph.
- `MICRO_MONOLITHS.md` matches the retained ownership map.
- `README.md` and `CONTRIBUTING.md` match the shipped command surface and repo layout.

## Proof

- Focused scenario tests cover changed behavior.
- `make ci` passes before repository health is claimed.
