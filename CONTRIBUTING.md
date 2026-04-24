# Contributing

## Core Expectation

The Go product lives only in:

- `cmd/espops/`
- `internal/cli/`
- `internal/config/`
- `internal/ops/`
- `internal/runtime/`
- `internal/manifest/`

Do not reintroduce a nested package namespace or any deleted package family.

Do not add:

- a second runtime
- auto-repair
- hidden normalization
- product commands beyond the shipped set

## Local Setup

Recommended prerequisites:

- Go `1.26.x`
- Docker with the Compose plugin
- `staticcheck`
- `golangci-lint`

Install the Go-side health tools with:

```bash
make install-health-tools
```

## Build And Test

Build the binary:

```bash
make build
```

Run the main test paths:

```bash
make test
make test-race
make integration
```

Run the repository health check:

```bash
make ci
```

## Working Rules

- Keep the product surface limited to `doctor`, `backup`, `backup verify`, `restore`, `migrate`, and `smoke`.
- `cmd/espops/` owns only the process entrypoint.
- `internal/cli/` owns command wiring, argument validation, JSON envelopes, and exit mapping.
- `internal/config/` owns env-file parsing and config loading.
- `internal/ops/` owns operational semantics and explicit post-checks.
- `internal/runtime/` owns Docker Compose and MariaDB process execution.
- `internal/manifest/` owns manifest validation and artifact path resolution.
- Keep shell execution and `os.Environ()` confined to `internal/runtime/docker.go`.
- Prefer deletion over wrappers.
- Fail closed when correctness is ambiguous.
- Keep `README.md`, `CONTRIBUTING.md`, and `AGENTS.md` in sync with the code.
- Do not claim a reliability improvement without end-to-end evidence.

## Review Gate

- Reject any PR that reintroduces deleted packages or a second product root.
- Reject any PR that expands the command surface beyond the six shipped commands.
- Reject any PR that moves shell ownership outside `internal/runtime/docker.go`.
- Reject any PR that leaves stale operator or contributor docs after changing product behavior.

## Typical Change Flow

1. Make the Go change.
2. Run the smallest relevant tests while iterating.
3. Run `make ci` before claiming repository health.
4. Update `README.md`, `CONTRIBUTING.md`, and `AGENTS.md` when the graph or command behavior changes.

## Repo Notes

- Example contour env files live under `env/`.
- `compose.yaml` and `deploy/` describe the runtime shape the tool operates against.
- Repo-wide guards live in [repository_test.go](repository_test.go).
