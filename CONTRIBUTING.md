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
Do not change the approved internal micro-monolith map implicitly; the binding micro-monolith constitution lives in [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).

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
- Keep the approved micro-monolith list, caller matrix, access classes, and ownership map aligned with [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
- Do not add process-env-only switches. Operator-facing behavior must come from flags or the contour env file.
- Preflight should inspect. Execution should mutate.
- Keep repo-wide architectural guards in `repository_test.go` and owner-local rules in package-local `architecture_test.go`; do not add guard theatre.
- Do not claim reliability improvements without end-to-end evidence.

## Operational Style

- CLI is edge-only: validate flags, normalize input, call one usecase boundary, route the boundary result or failure through the canonical journal/result/error path, and render one structured result or structured error output.
- Mutating app modules expose `Execute(req)`, run a linear workflow, and return structured `Info`, `Warnings`, `Steps`, `Counts()`, and `Ready()`.
- Final public error meaning belongs to the canonical error path: the boundary owns final app-level wrapping, and the CLI edge owns final transport mapping. Helpers return raw errors or lightweight local typed failures; diagnostic non-ready reports stay report-owned until the CLI maps them to transport semantics.
- Public `ErrorCode()` ownership belongs to final app/transport carriers only. Adapter/local typed causes may be typed, but must not masquerade as final public error codes.
- Keep helpers package-local. Do not add framework packages, generic engines, or unnecessary shared helper packages.
- Prefer explicit request-level injection or small local interfaces in tests. Do not add mutable package-global hooks.

## PR Review Gate

- Reject any PR that expands top-level `internal/app/*` production surface beyond the canonical boundary shape.
- Reject any PR that adds, removes, splits, or merges an approved micro-monolith without updating [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
- Reject any PR that introduces a direct caller edge forbidden by the micro-monolith interaction model.
- Reject any PR that moves privileged access to a weaker access class or introduces a hidden side channel.
- Reject any PR that reintroduces direct `internal/app -> internal/platform/*` imports.
- Reject any PR that introduces a second owner for an existing operational semantic.
- If a change intentionally moves the architecture baseline, require an updated formal audit and an updated [REPO_COMPLIANCE_BASELINE.md](REPO_COMPLIANCE_BASELINE.md).

## Typical Change Flow

1. Make the Go change.
2. Run the smallest relevant tests while iterating.
3. Run `make ci` before you call the repository healthy.
4. Update docs when command behavior, setup, or contributor workflow changes.

## Repo Notes

- Example contour env files live under `env/`.
- `compose.yaml` and `deploy/` describe the runtime the tool operates against.
- Architecture rules and the layer model live in [ARCHITECTURE.md](ARCHITECTURE.md).
- The binding internal micro-monolith constitution lives in [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
- The current accepted compliance baseline lives in [REPO_COMPLIANCE_BASELINE.md](REPO_COMPLIANCE_BASELINE.md).
- Repository rules and cleanup constraints live in [AGENTS.md](AGENTS.md).
