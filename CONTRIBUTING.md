# Contributing

## Core Expectation

The Go code is the product core.

Retained product behavior belongs in:

- `cmd/espops/` for the binary entrypoint
- `internal/cli/` for flags, command wiring, and result rendering
- `internal/usecase/` for retained workflows
- `internal/platform/` for side-effecting adapters

Do not move product behavior into shell wrappers.

## Local Setup

Recommended prerequisites:

- Go `1.26.x`
- Docker with Compose v2
- `staticcheck`
- `golangci-lint`

You can install the Go-side health tools with:

```bash
make install-health-tools
```

## Build And Test

Build the binary:

```bash
make build
```

Run the main Go test suite:

```bash
make test
make test-race
make integration
```

Run the default repository health check:

```bash
make ci
```

`make ci` is intentionally Go-focused. It validates the retained product directly without making shell wrappers or AI/governance machinery part of the mandatory path.

## Working Rules

- Keep the retained product surface limited to `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- Prefer deletion over compatibility shims.
- Prefer one source of truth over mirrored validation layers.
- Keep shell thin. Do not add parsing, validation, path resolution, or fallback behavior to `scripts/`.
- Preflight should inspect. Execution should mutate.
- Do not claim reliability improvements without end-to-end evidence.

## Transitional Areas

These paths still exist, but they are legacy or transitional:

- `scripts/`: deprecated shell wrappers
- `ops/systemd/`: deprecated units that still target shell entrypoints
- `AI/`: optional governance machinery, not part of default CI

If you touch those areas, treat the work as containment or cleanup. Do not grow them.

## Typical Change Flow

1. Make the Go change.
2. Run the smallest relevant tests while iterating.
3. Run `make ci` before you call the repository healthy.
4. Update docs when command behavior, setup, or contributor workflow changes.

## Repo Notes

- Example contour env files live under `ops/env/`.
- `compose.yaml` and `deploy/` describe the runtime the tool operates against.
- Repository rules and cleanup constraints live in [AGENTS.md](AGENTS.md).
