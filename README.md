# espocrm-ops

`espops` is a strict Go CLI for EspoCRM Docker Compose backup and recovery.

The product commands are exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

Every command writes one structured JSON result to `stdout` on success or failure.

## Build

```bash
make build
```

The binary is written to `bin/espops`.

## Project Contract

Run `espops` against a project directory containing:

- `compose.yaml`
- `.env.dev` and/or `.env.prod`

Start from `env/.env.dev.example` and `env/.env.prod.example`.

For `prod`, keep `.env.prod` as a regular file with mode `0600` when practical. `espops` warns when that hygiene is not met, but it only blocks when the env file cannot be read or parsed.

```bash
chmod 600 .env.prod
```

Env files are parsed as literal `KEY=VALUE` lines only: no quotes, no spaces, and no shell expansion syntax.

Required `espops` env keys:

- `BACKUP_ROOT`
- `BACKUP_NAME_PREFIX`
- `BACKUP_RETENTION_DAYS`
- `MIN_FREE_DISK_MB`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`

Additional keys required by `restore` and `migrate`:

- `DB_ROOT_PASSWORD`
- `ESPO_RUNTIME_UID`
- `ESPO_RUNTIME_GID`

Optional `espops` keys:

- `COMPOSE_FILE`
- `ESPO_CONTOUR`

Operator requirements:

- `BACKUP_ROOT` should be writable by the operator account. `doctor` reports write-probe failures as warnings; `backup` still fails if it cannot create artifacts.
- `ESPO_STORAGE_DIR` must already exist, point at the selected scope storage, and have a writable parent for adjacent staging during `restore` and `migrate`.
- `BACKUP_NAME_PREFIX` is used directly in artifact names: `<prefix>_<YYYY-MM-DD_HH-MM-SS>.sql.gz`, `.tar.gz`, and `.manifest.json`.
- `MIN_FREE_DISK_MB` is checked before `backup` stops app services and is kept as the free-space reserve when `restore` or `migrate` preflights files staging next to `ESPO_STORAGE_DIR`.
- `BACKUP_RETENTION_DAYS=0` disables retention cleanup.
- `APP_SERVICES` must explicitly list the application services; the shipped Compose runtime expects `espocrm,espocrm-daemon,espocrm-websocket`.
- `DB_SERVICE` must name the exact Compose database service.
- `DB_SERVICE` and every service in `APP_SERVICES` must expose a Docker Compose healthcheck.

## Secrets And Images

The shipped Compose contract uses inline `DB_PASSWORD` and `DB_ROOT_PASSWORD`. File-based password env keys are not part of the runtime contract.

The env examples use readable tag-based images:

- `ESPOCRM_IMAGE=espocrm/espocrm:9.3.4-apache`
- `MARIADB_IMAGE=mariadb:11.4`

Digest-pinned image refs such as `prefix@sha256:<64-lower-hex-digest>` improve restore reproducibility. For `--scope prod`, `espops` warns when image refs are not digest-pinned, but it does not block tag-based internal deployments.

Before first production use, pre-pull the exact `ESPOCRM_IMAGE` and `MARIADB_IMAGE` refs from the env file you intend to trust.

The examples bind HTTP and websocket ports to `127.0.0.1`. To expose the stack elsewhere, set `APP_BIND_ADDRESS`, `WS_BIND_ADDRESS`, `SITE_URL`, and `WS_PUBLIC_URL` explicitly.

## Locks And Success

Mutating commands use cross-process operation locks under `PROJECT_DIR/.espops/locks`. `espops` may create that directory.

- `doctor` is read-only and does not lock.
- `backup` locks its scope before runtime validation, disk checks, service stop, and artifact creation.
- `restore` locks the target scope before manifest verify, snapshot backup, service stop, database reset, and storage mutation.
- `migrate` locks both the source scope and target scope.

Success requires explicit health or post-check evidence. A MariaDB ping alone is not success.

- `doctor` succeeds only when config parses, storage prerequisites pass, contract services are present, `running`, and `healthy`, and MariaDB `SELECT 1` succeeds. Non-blocking hygiene issues are reported as warnings.
- `backup` succeeds only after app services are returned, all contract services are `running` and `healthy`, and the new backup set self-verifies.
- `restore` and `migrate` succeed only after restored storage passes post-check, contract services are `running` and `healthy`, and MariaDB `SELECT 1` succeeds.

These checks validate the Compose service contract, not a browser login flow.

## Backup

```bash
./bin/espops doctor --scope dev --project-dir /path/to/project
./bin/espops backup --scope dev --project-dir /path/to/project
./bin/espops backup verify --manifest /path/to/manifest.json
```

`backup` writes manifest version `2` with artifact checksums and runtime metadata: image refs, database name, service names, backup prefix, and storage contract.

Retention cleanup runs only after the new set self-verifies. It deletes only complete same-prefix sets from the current `BACKUP_ROOT` layout and refuses suspicious or incomplete sets.

`backup verify` can diagnose a valid manifest version `1` backup set, but version `1` is verify-only. Destructive commands require manifest version `2` with runtime metadata.

## Restore And Migrate

`restore` and `migrate` are destructive for the target scope. Verify the manifest first.

```bash
./bin/espops backup verify --manifest /path/to/manifest.json
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
./bin/espops migrate --from-scope dev --to-scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
```

`restore` is same-scope only: `manifest.scope` must match `--scope`, and the manifest runtime block must match the target runtime contract.

`migrate` is the supported cross-scope restore path. It requires manifest version `2` and checks the recorded image, service, and storage contract before target mutation. For `dev` to `prod` migration, both scopes must use the same `ESPOCRM_IMAGE` and `MARIADB_IMAGE` refs; digest pinning is recommended and reported as a warning when absent.

Both flows create a target snapshot before mutation, verify target storage parent free space for files staging, reset the target database as MariaDB root, import into a clean database, restore files through staged extraction next to target storage, apply `ESPO_RUNTIME_UID` and `ESPO_RUNTIME_GID`, switch storage by same-parent rename, and run final post-checks before reporting success.

If `restore` or `migrate` fails after the target snapshot exists, the error JSON includes `result.snapshot_manifest`. Database and file rollback is manual: use that snapshot manifest to plan and execute recovery of the target scope.

## Manual Destructive Smoke Sequence

```bash
PROJECT_DIR=/path/to/project
MANIFEST=/path/to/fresh-dev.manifest.json

./bin/espops doctor --scope dev --project-dir "$PROJECT_DIR"
./bin/espops doctor --scope prod --project-dir "$PROJECT_DIR"
./bin/espops backup --scope dev --project-dir "$PROJECT_DIR"
./bin/espops backup verify --manifest "$MANIFEST"
./bin/espops migrate --from-scope dev --to-scope prod --project-dir "$PROJECT_DIR" --manifest "$MANIFEST"
```

This is a manual destructive sequence, not a product command. Use the fresh manifest produced by the `backup` step. Add a same-scope `restore` smoke only when you intentionally want to test that destructive path separately. The sequence does no setup, pull, cleanup, retry, or fallback.

Developer rules and test paths live in `CONTRIBUTING.md`.
