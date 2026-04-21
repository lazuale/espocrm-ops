# Contributing

## Core Expectation

The Go code is the product.

Retained product behavior belongs in:

- `cmd/espops/` for the binary entrypoint
- `internal/cli/` for flags, command wiring, and result rendering
- `internal/app/` for retained workflows and operation lifecycle
- `internal/domain/` for policy and shared operational meaning
- `internal/platform/` for side-effecting adapters
- `internal/opsconfig/` for shared operational semantics that must stay Go-owned

Do not add a second runtime. Do not reintroduce shell-owned behavior.

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

## Working Rules

- Keep the retained product surface limited to `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- Prefer deletion over compatibility shims.
- Prefer one source of truth over mirrored validation layers.
- Keep operational semantics in Go.
- Do not add process-env-only switches. Operator-facing behavior must come from flags or the contour env file.
- Preflight should inspect. Execution should mutate.
- Do not claim reliability improvements without end-to-end evidence.

## Operational Style

- CLI is edge-only: validate flags, normalize input, call one usecase boundary, render one structured result.
- Mutating app modules expose `Execute(req)`, run a linear workflow, and return structured `Info`, `Warnings`, `Steps`, `Counts()`, and `Ready()`.
- Final `apperr` wrapping belongs to the `Execute()` boundary. Helpers return raw errors or lightweight local typed failures.
- Keep helpers package-local. Do not add framework packages, generic engines, or unnecessary shared helper packages.
- Prefer explicit request-level injection or small local interfaces in tests. Do not add mutable package-global hooks.

## Typical Change Flow

1. Make the Go change.
2. Run the smallest relevant tests while iterating.
3. Run `make ci` before you call the repository healthy.
4. Update docs when command behavior, setup, or contributor workflow changes.

## Repo Notes

- Example contour env files live under `env/`.
- `compose.yaml` and `deploy/` describe the runtime the tool operates against.
- Architecture rules and the layer model live in [ARCHITECTURE.md](ARCHITECTURE.md).
- Repository rules and cleanup constraints live in [AGENTS.md](AGENTS.md).
