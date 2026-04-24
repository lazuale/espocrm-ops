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

Operator prerequisites:

- `BACKUP_ROOT` must already exist before running `doctor`, `backup`, `restore`, or `migrate`
- `BACKUP_ROOT` must be writable by the operator account; `doctor` checks it but does not create or repair it
- `ESPO_STORAGE_DIR` must already exist and must be the real storage directory for the selected scope
- MariaDB native tooling target is `11.4 LTS`
- Native tooling only: `docker compose`, `mariadb-dump`, `mariadb`, and Go stdlib archive/checksum handling
- No compatibility path, no fallback path, and no service-name guessing

## Minimal Safe Workflow

Prepare the target scope first:

1. Ensure `compose.yaml` and `.env.<scope>` exist.
2. Ensure `BACKUP_ROOT` already exists and is writable.
3. Ensure `ESPO_STORAGE_DIR` already exists and points at the correct scope storage.
4. Run `espops doctor`.

Then use the retained commands in this order:

1. `espops backup`
   Backup stops app services while it creates a full backup set.
2. `espops backup verify`
   Verify the manifest you plan to trust.
3. `espops restore`
   Restore is destructive for the target scope. It verifies the source manifest first and creates a target snapshot before mutation.
4. `espops migrate`
   Migrate is thin composition over verified restore flow, not a separate engine.

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
- `internal/cli/`: root command surface, envelopes, and exit mapping
- `internal/config/`: env-file loading and config validation
- `internal/ops/`: retained operation workflows and post-checks
- `internal/runtime/`: Docker Compose and MariaDB command execution
- `internal/manifest/`: backup manifest contract and artifact resolution
- `deploy/`: runtime tuning files used by `compose.yaml`
- `env/`: example env files
