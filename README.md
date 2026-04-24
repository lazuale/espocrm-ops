# espocrm-ops

`espops` is a small Go CLI for strict backup and recovery work around an EspoCRM Docker Compose deployment.

The retained product surface is exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

Every product command writes one structured JSON result to `stdout` on success or failure.

## Build

```bash
make build
```

That produces `bin/espops`.

You can also run the CLI directly:

```bash
go run ./cmd/espops --help
```

## Test

The repository health path is:

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

## Runtime Contract

`espops` expects a project directory that contains:

- `compose.yaml`
- `.env.dev` and/or `.env.prod`

Required env keys per scope:

- `BACKUP_ROOT`
- `ESPO_STORAGE_DIR`
- `DB_USER`
- `DB_NAME`
- one of `DB_PASSWORD` or `DB_PASSWORD_FILE`

Optional env keys:

- `COMPOSE_FILE`
- `APP_SERVICES`
- `DB_SERVICE`
- `ESPO_CONTOUR`

Example env files live under `env/`.

## Command Surface

General help:

```bash
./bin/espops --help
```

Check runtime readiness:

```bash
./bin/espops doctor --scope dev --project-dir /path/to/project
./bin/espops doctor --scope prod --project-dir /path/to/project
```

Create a backup:

```bash
./bin/espops backup --scope dev --project-dir /path/to/project
./bin/espops backup --scope prod --project-dir /path/to/project
```

Verify a backup set:

```bash
./bin/espops backup verify --manifest /path/to/manifest.json
```

Restore from a verified manifest:

```bash
./bin/espops restore --scope dev --project-dir /path/to/project --manifest /path/to/manifest.json
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
```

Migrate from one scope into another:

```bash
./bin/espops migrate --from-scope dev --to-scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
```

## Repository Layout

- `cmd/espops/`: program entrypoint only
- `internal/v3/cli/`: root command surface, envelopes, and exit mapping
- `internal/v3/config/`: env-file loading and config validation
- `internal/v3/ops/`: retained operation workflows and post-checks
- `internal/v3/runtime/`: Docker Compose and MariaDB command execution
- `internal/v3/manifest/`: backup manifest contract and artifact resolution
- `deploy/`: runtime tuning files used by `compose.yaml`
- `env/`: example env files

## More Context

- Repository rules live in [AGENTS.md](AGENTS.md).
- Layer rules live in [ARCHITECTURE.md](ARCHITECTURE.md).
- Retained ownership and caller rules live in [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
- Current retained product notes live in [V3.md](V3.md).
