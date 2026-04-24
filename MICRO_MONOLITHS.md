# Micro Monoliths

This repository keeps a small fixed ownership map.

Only the retained units below are allowed.

## 1. Product Root Monolith

Owned files:

- `cmd/espops/main.go`

Owns:

- process startup
- delegation into the retained root command

Allowed internal imports:

- `internal/v3/cli`

## 2. CLI Monolith

Owned files:

- `internal/v3/cli/*.go`

Owns:

- root command surface
- command argument validation
- output envelopes
- exit mapping

Allowed internal imports:

- `internal/v3/config`
- `internal/v3/ops`
- `internal/v3/runtime`

Must not own:

- native command execution policy details
- manifest storage contract
- a second execution path

## 3. Config Monolith

Owned files:

- `internal/v3/config/config.go`

Owns:

- env-file parsing
- config resolution
- config-level validation

Allowed internal imports:

- none

## 4. Manifest Monolith

Owned files:

- `internal/v3/manifest/manifest.go`

Owns:

- manifest schema
- manifest validation
- artifact path resolution

Allowed internal imports:

- none

## 5. Runtime Monolith

Owned files:

- `internal/v3/runtime/docker.go`

Owns:

- Docker Compose command assembly
- MariaDB dump, restore, and ping execution
- the only production shell seam
- the only production `os.Environ()` ownership

Allowed internal imports:

- none

## 6. Ops Monolith

Owned files:

- `internal/v3/ops/*.go`

Owns:

- retained operational workflows
- pre-mutation coherence checks
- post-check and verification semantics
- failure kind selection consumed by the CLI layer

Allowed internal imports:

- `internal/v3/config`
- `internal/v3/manifest`
- `internal/v3/runtime`

## Interaction Model

Only these caller edges are allowed:

- `cmd/espops` -> `internal/v3/cli`
- `internal/v3/cli` -> `internal/v3/config`
- `internal/v3/cli` -> `internal/v3/ops`
- `internal/v3/cli` -> `internal/v3/runtime`
- `internal/v3/ops` -> `internal/v3/config`
- `internal/v3/ops` -> `internal/v3/manifest`
- `internal/v3/ops` -> `internal/v3/runtime`

Everything else is forbidden until this document changes.

## Forbidden Drift

- reintroducing `internal/app`, `internal/cli`, `internal/contract`, `internal/domain`, `internal/platform`, `internal/model`, `internal/runtime`, `internal/store`, or `internal/opsconfig`
- restoring a dual-path root
- adding a shell seam outside `internal/v3/runtime/docker.go`
- creating a second manifest or config authority
- adding new product commands without updating this document
