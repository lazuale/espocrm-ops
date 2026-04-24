# espocrm-ops

`espops` is a small Go CLI for strict backup and recovery work around an EspoCRM Docker Compose deployment.

The product surface is exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`
- `smoke`

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

`make test` is the fast unit layer. It includes CLI and ops tests that intentionally use fake docker scripts where isolation is the point.

`make integration` is the real Docker integration layer. It requires:

- a reachable Docker daemon
- the Docker Compose plugin
- network access to pull the minimal MariaDB and Alpine images used by the generated integration fixture

It does not silently skip when Docker is unavailable; it fails closed instead.

The repository health path is:

```bash
make ci
```

`make ci` runs build, `go mod verify`, `go test -mod=readonly ./...`, race tests, `go vet`, `staticcheck`, `golangci-lint`, the real Docker integration target, and a clean `go.mod`/`go.sum` check.

Useful individual targets:

```bash
make test
make test-race
make test-readonly
make integration
make vet
make mod-verify
make staticcheck
make lint
```

## Runtime Contract

`espops` expects a project directory that contains:

- `compose.yaml`
- `.env.dev` and/or `.env.prod`

Required env keys per scope:

- `BACKUP_ROOT`
- `BACKUP_NAME_PREFIX`
- `BACKUP_RETENTION_DAYS`
- `MIN_FREE_DISK_MB`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_NAME`
- one of `DB_PASSWORD` or `DB_PASSWORD_FILE`

Additional required env keys for `restore`, `migrate`, and `smoke`:

- one of `DB_ROOT_PASSWORD` or `DB_ROOT_PASSWORD_FILE`
- `ESPO_RUNTIME_UID`
- `ESPO_RUNTIME_GID`

Optional env keys:

- `COMPOSE_FILE`
- `ESPO_CONTOUR`

Example env files live under `env/`.

Operator prerequisites:

- `BACKUP_ROOT` must already exist before running `doctor`, `backup`, `restore`, or `migrate`
- `BACKUP_ROOT` must be writable by the operator account; `doctor` checks it but does not create or repair it
- `BACKUP_NAME_PREFIX` is required for every backup-capable scope and is used directly for artifact names: `<BACKUP_NAME_PREFIX>_<YYYY-MM-DD_HH-MM-SS>.sql.gz`, `.tar.gz`, and `.manifest.json`
- `MIN_FREE_DISK_MB` is required for every backup-capable scope, must be an integer greater than zero, and is checked before `backup` stops app services or creates backup artifacts
- `BACKUP_RETENTION_DAYS` is required for every backup-capable scope, must be an integer greater than or equal to zero, and `0` disables retention cleanup explicitly
- `ESPO_STORAGE_DIR` must already exist, must be the real storage directory for the selected scope, and must be clearable by the operator account before `restore` or `migrate`
- `smoke` does no setup, no pull, no cleanup, no retry, and no fallback; if a step fails, `smoke` fails
- MariaDB native tooling target is `11.4 LTS`
- Native tooling only: `docker compose`, `mariadb-dump`, `mariadb`, and Go stdlib archive/checksum handling
- One command path only, with no service-name guessing or implicit service defaults
- `DB_SERVICE` must name the exact Compose database service
- `APP_SERVICES` must list the exact Compose application services as a comma-separated contract
- Retention cleanup runs only after the freshly created backup set passes self-verify, deletes only complete same-prefix sets from the current `BACKUP_ROOT` layout, and refuses incomplete or suspicious sets instead of deleting them automatically
- `restore` fails closed unless `manifest.scope` matches `--scope`; use `migrate` for intentional cross-scope restore
- `restore`, `migrate`, and `smoke` reset the target database as MariaDB root before importing the dump
- `restore` and `migrate` restore files through staged extraction: the archive is validated, extracted into staging, the staged tree is validated, and target storage is cleared only after staging succeeds
- `espops` never guesses `ESPO_RUNTIME_UID` or `ESPO_RUNTIME_GID` from the image or the container runtime
- `restore`, `migrate`, and `smoke` apply the restored storage tree to the explicit runtime uid/gid and fail closed if the operator cannot apply that ownership

## Minimal Safe Workflow

Prepare the target scope first:

1. Ensure `compose.yaml` and `.env.<scope>` exist.
2. Ensure `BACKUP_ROOT` already exists and is writable.
3. Ensure `ESPO_STORAGE_DIR` already exists, points at the correct scope storage, and is clearable by the operator account for `restore` and `migrate`.
4. Run `espops doctor`.

Then use the commands in this order:

1. `espops backup`
   Backup checks `MIN_FREE_DISK_MB`, stops app services only after that guard passes, writes prefix-based artifacts under `BACKUP_ROOT`, self-verifies the new set, then applies strict same-prefix retention if `BACKUP_RETENTION_DAYS` is greater than zero.
2. `espops backup verify`
   Verify the manifest you plan to trust.
3. `espops restore`
   Restore is destructive for the target scope. It verifies the source manifest first, requires a same-scope manifest, creates a target snapshot before mutation, resets the target database, imports into the clean database, then restores files through staged extraction, clears target storage only after staging succeeds, and applies the explicit runtime uid/gid before the final file post-check.
4. `espops migrate`
   Migrate is thin composition over verified restore flow, not a separate engine. It is the only supported cross-scope restore path.

For one explicit fixed-path operator check across both scopes:

1. `espops smoke`
   Smoke runs `doctor source`, `doctor target`, `backup`, `backup verify`, `restore`, and `migrate` in that order. It uses the fresh backup manifest it just created and stops on the first failure.

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

Run the fixed smoke path:

```bash
./bin/espops smoke --from-scope dev --to-scope prod --project-dir /path/to/project
```

## Repository Layout

- `cmd/espops/`: program entrypoint only
- `internal/cli/`: root command surface, envelopes, and exit mapping
- `internal/config/`: env-file loading and config validation
- `internal/ops/`: operation workflows and post-checks
- `internal/runtime/`: Docker Compose and MariaDB command execution
- `internal/manifest/`: backup manifest contract and artifact resolution
- `deploy/`: runtime tuning files used by `compose.yaml`
- `env/`: example env files
