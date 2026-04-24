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
make test-readonly
make integration
```

Run the repository health check:

```bash
make ci
```

Contract:

- `make test` is the fast unit layer and may use fake docker scripts inside tests where command wiring or failure shaping is the subject under test.
- `make integration` is the real Docker integration layer. It requires a live Docker daemon, the Compose plugin, and network/image pull capability. It must not pass by running zero real integration tests.
- `make ci` is the repository health claim. It covers build, module verification, readonly tests, race tests, `go vet`, `staticcheck`, `golangci-lint`, real Docker integration, and a clean `go.mod`/`go.sum` check.

## Working Rules

- Keep the product surface limited to `doctor`, `backup`, `backup verify`, `restore`, `migrate`, and `smoke`.
- `cmd/espops/` owns only the process entrypoint.
- `internal/cli/` owns command wiring, argument validation, JSON envelopes, and exit mapping.
- `internal/config/` owns env-file parsing and config loading.
- `internal/ops/` owns operational semantics and explicit post-checks.
- `internal/runtime/` owns Docker Compose and MariaDB process execution.
- `internal/manifest/` owns manifest validation and artifact path resolution.
- Keep shell execution and `os.Environ()` confined to `internal/runtime/docker.go`.
- Keep `DB_SERVICE` and `APP_SERVICES` explicit in the env contract; do not reintroduce guessed or defaulted service names.
- Keep inline `DB_PASSWORD` and `DB_ROOT_PASSWORD` explicit in the shipped compose/env contract; do not claim file-based runtime secrets that `compose.yaml` does not consume.
- Keep `DB_ROOT_PASSWORD` explicit for restore-capable flows; do not fall back to `DB_USER` credentials for database reset.
- Keep `ESPO_RUNTIME_UID` and `ESPO_RUNTIME_GID` explicit for restore-capable flows; do not guess runtime ownership from the image, container user, or current operator account.
- Keep `compose.yaml`, `env/.env.*.example`, `README.md`, and `internal/config/` on one literal runtime contract; when one changes, update the others and the repository guards in `repository_test.go`.
- Do not add decorative env keys to examples.
- Prefer deletion over wrappers.
- Fail closed when correctness is ambiguous.
- Keep `README.md`, `CONTRIBUTING.md`, and `AGENTS.md` in sync with the code.
- Do not claim a reliability improvement without end-to-end evidence.
- Do not claim integration coverage from fake docker tests; integration evidence must come from the tagged real Docker layer.

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
