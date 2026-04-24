# Contributing

## Core Expectation

The retained Go product lives only in:

- `cmd/espops/`
- `internal/v3/cli/`
- `internal/v3/config/`
- `internal/v3/ops/`
- `internal/v3/runtime/`
- `internal/v3/manifest/`

Do not reintroduce `internal/app`, `internal/cli`, `internal/contract`, `internal/domain`, `internal/platform`, `internal/model`, `internal/runtime`, `internal/store`, or `internal/opsconfig`.

Do not add:

- a second runtime
- compatibility shims
- fallback paths
- auto-repair
- hidden normalization
- new product commands

## Local Setup

Recommended prerequisites:

- Go `1.26.x`
- Docker with Compose v2
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

- Keep the product surface limited to `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- `cmd/espops/` owns only the process entrypoint.
- `internal/v3/cli/` owns command wiring, argument validation, JSON envelopes, and exit mapping.
- `internal/v3/config/` owns env-file parsing and config loading.
- `internal/v3/ops/` owns retained operational semantics and explicit post-checks.
- `internal/v3/runtime/` owns Docker Compose and MariaDB process execution.
- `internal/v3/manifest/` owns manifest validation and artifact path resolution.
- Keep shell execution and `os.Environ()` confined to `internal/v3/runtime/docker.go`.
- Prefer deletion over wrappers.
- Fail closed when correctness is ambiguous.
- Keep authority docs and compliance docs in sync with the retained code.
- Do not claim a reliability improvement without end-to-end evidence.

## Review Gate

- Reject any PR that reintroduces removed legacy packages or a dual-path product root.
- Reject any PR that expands the command surface beyond the retained five commands.
- Reject any PR that moves shell ownership outside `internal/v3/runtime/docker.go`.
- Reject any PR that splits retained ownership without updating [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
- Reject any PR that leaves stale operator or contributor docs after changing product behavior.

## Typical Change Flow

1. Make the Go change.
2. Run the smallest relevant tests while iterating.
3. Run `make ci` before claiming repository health.
4. Update `ARCHITECTURE.md`, `MICRO_MONOLITHS.md`, `README.md`, `CONTRIBUTING.md`, and compliance docs when the retained graph or command behavior changes.

## Repo Notes

- Example contour env files live under `env/`.
- `compose.yaml` and `deploy/` describe the runtime shape the tool operates against.
- Repo-wide guards live in [repository_test.go](repository_test.go).
