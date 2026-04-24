# Micro Monoliths

This repository keeps a small fixed ownership map.

## Units

- `cmd/espops/main.go`
  Owns process startup and delegation into `internal/cli`.
- `internal/cli/*.go`
  Owns command wiring, flags, JSON envelopes, and exit mapping.
- `internal/config/config.go`
  Owns env parsing, config resolution, and config validation.
- `internal/manifest/manifest.go`
  Owns manifest schema, validation, and artifact path resolution.
- `internal/runtime/docker.go`
  Owns Docker Compose and MariaDB execution, plus the only production shell and `os.Environ()` seam.
- `internal/ops/*.go`
  Owns retained workflows, verification, post-checks, and failure classification.

## Allowed Internal Edges

- `cmd/espops` -> `internal/cli`
- `internal/cli` -> `internal/config`
- `internal/cli` -> `internal/ops`
- `internal/cli` -> `internal/runtime`
- `internal/ops` -> `internal/config`
- `internal/ops` -> `internal/manifest`
- `internal/ops` -> `internal/runtime`

Everything else is forbidden until this document changes.

## Forbidden Drift

- reintroducing a nested retained-package namespace
- reintroducing deleted package families
- restoring a dual-path root
- adding a shell seam outside `internal/runtime/docker.go`
- creating a second config or manifest authority
- adding new product commands without updating this document
