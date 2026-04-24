# Architecture

## Authority

Repository authority is:

1. `AGENTS.md`
2. `ARCHITECTURE.md` and `MICRO_MONOLITHS.md`
3. Go code under `cmd/espops/` and `internal/v3/`
4. `Makefile`
5. `.github/workflows/ci.yml`

## Retained Product

The retained product is exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

There is no legacy production path.
There is no second runtime.
There is no `espops v3` operator path.

## Retained Package Graph

Only this internal graph is valid:

- `cmd/espops` -> `internal/v3/cli`
- `internal/v3/cli` -> `internal/v3/config`, `internal/v3/ops`, `internal/v3/runtime`
- `internal/v3/ops` -> `internal/v3/config`, `internal/v3/manifest`, `internal/v3/runtime`
- `internal/v3/config` -> stdlib only
- `internal/v3/manifest` -> stdlib only
- `internal/v3/runtime` -> stdlib only

No other production package family is allowed under `internal/`.

## Responsibilities

### `cmd/espops/`

Owns:

- process entrypoint only
- handoff into the retained root command

Must not own:

- workflow branching
- config loading
- operational semantics

### `internal/v3/cli/`

Owns:

- Cobra root and subcommands
- flag parsing and command-local validation
- structured JSON envelopes
- exit-code mapping
- injection of the retained runtime implementation

Must not own:

- Docker or MariaDB command execution
- manifest integrity rules
- hidden fallback behavior

### `internal/v3/config/`

Owns:

- `.env.dev` and `.env.prod` parsing
- project-dir and compose/env file resolution
- required config validation

### `internal/v3/ops/`

Owns:

- retained backup, verification, restore, migrate, and doctor workflows
- coherence checks before mutation
- explicit post-check or health-check before success
- failure classification used by the CLI envelope

### `internal/v3/runtime/`

Owns:

- Docker Compose command construction
- MariaDB dump, restore, and ping execution
- process environment handoff to native tooling

This is the only allowed shell/process seam.

### `internal/v3/manifest/`

Owns:

- manifest schema validation
- artifact name rules
- artifact path resolution relative to `manifests/`, `db/`, and `files/`

## Core Flow

Every retained command follows the same shape:

1. Resolve CLI input.
2. Validate input.
3. Load and verify retained config or manifest state.
4. Execute native side effects when required.
5. Run explicit verification or post-check.
6. Return one explicit structured result.

If correctness is ambiguous, the command must fail.

## Global Constraints

- No legacy production packages.
- No compatibility shim.
- No hidden fallback.
- No auto-repair.
- No silent recovery.
- No shell-owned validation or recovery semantics.
- No new product surface without updating authority docs first.
