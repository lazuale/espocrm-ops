# espocrm-ops

`espops` is a Go CLI for strict backup and recovery work around an EspoCRM Docker Compose deployment.

The retained product surface is intentionally small:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

## What It Does

`espops` exists to make stateful operations explicit and fail closed:

- resolve and validate operational inputs
- inspect runtime readiness before stateful work
- run backup, verification, restore, and migration flows
- perform explicit post-checks before reporting success

This repository is not a general EspoCRM management toolkit. It is a small operational CLI.

## Prerequisites

- Go `1.26.x`
- Docker with Compose v2
- An EspoCRM project layout that matches `compose.yaml`
- A contour env file at the repo root such as `.env.dev` or `.env.prod`

Example env files live under `env/`:

- `env/.env.dev.example`
- `env/.env.prod.example`

## Build

```bash
make build
```

That produces `bin/espops`.

You can also run the CLI without a prior build:

```bash
go run ./cmd/espops --help
```

## Test

The default repository health path is Go-focused:

```bash
make ci
```

Useful individual targets:

```bash
make test
make test-race
make integration
make staticcheck
make lint
```

## Command Surface

General help:

```bash
./bin/espops --help
```

Check runtime readiness for one contour or both:

```bash
./bin/espops doctor --scope dev
./bin/espops doctor --scope all --json
```

Create a backup:

```bash
./bin/espops backup --scope dev
./bin/espops backup --scope prod --no-stop
```

Verify a backup:

```bash
./bin/espops backup verify --manifest /path/to/manifest.json
./bin/espops backup verify --backup-root /path/to/backups/dev
```

Run a restore:

```bash
./bin/espops restore --scope dev --manifest /path/to/manifest.json --force
./bin/espops restore --scope prod --manifest /path/to/manifest.json --force --confirm-prod prod
```

Run a migration:

```bash
./bin/espops migrate --from dev --to prod --force --confirm-prod prod
./bin/espops migrate --from prod --to dev --db-backup /path/to/db.sql.gz --files-backup /path/to/files.tar.gz --force
```

## Repository Layout

- `cmd/espops/`: program entrypoint
- `internal/cli/`: Cobra command surface, input validation, result rendering
- `internal/usecase/`: retained product behavior
- `internal/platform/`: filesystem, Docker, config, backup storage, and lock adapters
- `internal/opsconfig/`: shared Go authority for path and env-derived runtime semantics
- `internal/contract/`: JSON/output contract and exit codes
- `deploy/`: container tuning files used by `compose.yaml`
- `env/`: example env files

## More Context

Repository rules and cleanup constraints live in [AGENTS.md](AGENTS.md).
