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
make pull-images
make vet
make mod-verify
make staticcheck
make lint
```

## Images And Supply-Chain Contract

Shipped runtime image keys are:

- `ESPOCRM_IMAGE`
- `MARIADB_IMAGE`

Shipped env examples stay tag-based:

- `ESPOCRM_IMAGE=espocrm/espocrm:9.3.4-apache`
- `MARIADB_IMAGE=mariadb:11.4`

That is readable and easy to replace, but it is not digest-pinned. The repository does not claim that the shipped examples are production-safe from a supply-chain perspective.

For real production, replace both image refs with digest-pinned values before first deployment, for example `repo@sha256:...`. `espops` does not auto-detect digests and does not enforce digest pinning for you.

To pre-pull the exact runtime refs you intend to trust:

```bash
docker pull "<ESPOCRM_IMAGE from your env file>"
docker pull "<MARIADB_IMAGE from your env file>"
```

For Docker integration health, use:

```bash
make pull-images
make integration
```

`make pull-images` pre-pulls the minimal integration fixture images (`mariadb:11.4` and `alpine:3.20`) and fails closed with a clearer diagnosis when Docker Hub, auth, rate-limit, or missing-image problems block the pull.

`make integration` proves Docker daemon access, Compose plugin access, required integration images available locally, and then the real Docker integration tests. A Docker Hub or network timeout does not by itself prove that the Go code is broken; it proves only that the registry path was not available.

## Runtime Contract

`espops` expects a project directory that contains:

- `compose.yaml`
- `.env.dev` and/or `.env.prod`

`compose.yaml` consumes these env keys directly:

- `COMPOSE_PROJECT_NAME`
- `ESPOCRM_IMAGE`
- `MARIADB_IMAGE`
- `DB_STORAGE_DIR`
- `ESPO_STORAGE_DIR`
- `DB_MEM_LIMIT`
- `DB_CPUS`
- `DB_PIDS_LIMIT`
- `ESPO_MEM_LIMIT`
- `ESPO_CPUS`
- `ESPO_PIDS_LIMIT`
- `DAEMON_MEM_LIMIT`
- `DAEMON_CPUS`
- `DAEMON_PIDS_LIMIT`
- `WS_MEM_LIMIT`
- `WS_CPUS`
- `WS_PIDS_LIMIT`
- `DOCKER_LOG_MAX_SIZE`
- `DOCKER_LOG_MAX_FILE`
- `APP_BIND_ADDRESS`
- `APP_PORT`
- `WS_BIND_ADDRESS`
- `WS_PORT`
- `SITE_URL`
- `WS_PUBLIC_URL`
- `DB_ROOT_PASSWORD`
- `DB_NAME`
- `DB_USER`
- `DB_PASSWORD`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `ESPO_DEFAULT_LANGUAGE`
- `ESPO_TIME_ZONE`
- `ESPO_LOGGER_LEVEL`

`espops` validates these keys for every backup-capable scope:

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

Additional required env keys for `restore`, `migrate`, and `smoke`:

- `DB_ROOT_PASSWORD`
- `ESPO_RUNTIME_UID`
- `ESPO_RUNTIME_GID`

Optional `espops`-only keys:

- `COMPOSE_FILE`
- `ESPO_CONTOUR`

Example env files live under `env/`.

Operator prerequisites:

- Shipped examples and shipped Compose runtime use inline `DB_PASSWORD` and `DB_ROOT_PASSWORD`. File-based password env keys are not part of the runtime contract.
- `BACKUP_ROOT` must already exist before running `doctor`, `backup`, `restore`, or `migrate`
- `BACKUP_ROOT` must be writable by the operator account; `doctor` checks it but does not create or repair it
- `BACKUP_NAME_PREFIX` is required for every backup-capable scope and is used directly for artifact names: `<BACKUP_NAME_PREFIX>_<YYYY-MM-DD_HH-MM-SS>.sql.gz`, `.tar.gz`, and `.manifest.json`
- `MIN_FREE_DISK_MB` is required for every backup-capable scope, must be an integer greater than zero, and is checked before `backup` stops app services or creates backup artifacts
- `BACKUP_RETENTION_DAYS` is required for every backup-capable scope, must be an integer greater than or equal to zero, and `0` disables retention cleanup explicitly
- `ESPO_STORAGE_DIR` must already exist, must be the real storage directory for the selected scope, and must be clearable by the operator account before `restore` or `migrate`
- Current MariaDB runtime baseline is `mariadb:11.4`; both shipped env examples set `MARIADB_IMAGE=mariadb:11.4`, and the Docker integration fixture uses the same major/minor target
- `APP_BIND_ADDRESS` and `WS_BIND_ADDRESS` are required. Shipped examples use `127.0.0.1` to avoid accidental publish-all. To expose on LAN or public interfaces, set an explicit host IP or `0.0.0.0` on purpose and update `SITE_URL` and `WS_PUBLIC_URL` to match
- Current Compose runtime includes the websocket container. `APP_SERVICES` must explicitly list `espocrm,espocrm-daemon,espocrm-websocket`; there is no fallback that adds websocket automatically
- Shared MariaDB baseline in `deploy/mariadb/z-custom.cnf` sets `innodb_buffer_pool_size=512M`. Shipped examples keep `DB_MEM_LIMIT` at or above that baseline (`768m` in dev and `1g` in prod); do not set `DB_MEM_LIMIT` below the buffer pool size
- `smoke` does no setup, no pull, no cleanup, no retry, and no fallback; if a step fails, `smoke` fails
- Native tooling only: `docker compose`, `mariadb-dump`, `mariadb`, and Go stdlib archive/checksum handling
- One command path only, with no service-name guessing or implicit service defaults
- Mutating operations use per-scope cross-process file locks under `PROJECT_DIR/.espops/locks`; the tool may create that directory as an explicit runtime side effect
  The literal lock home is `.espops/locks` inside the selected project directory.
- `DB_SERVICE` must name the exact Compose database service
- `APP_SERVICES` must list the exact Compose application services as a comma-separated contract
- `DB_SERVICE` and every service in `APP_SERVICES` must expose a Docker Compose healthcheck; a merely running service without health status does not satisfy the success contract
- Retention cleanup runs only after the freshly created backup set passes self-verify, deletes only complete same-prefix sets from the current `BACKUP_ROOT` layout, and refuses incomplete or suspicious sets instead of deleting them automatically
- `backup` writes manifest version `2`. Version `2` manifest records artifact checksums plus a runtime block with `ESPOCRM_IMAGE`, `MARIADB_IMAGE`, `DB_NAME`, `DB_SERVICE`, `APP_SERVICES`, `BACKUP_NAME_PREFIX`, and the fixed storage contract `espocrm-full-storage-v1`
- `backup verify` can still diagnose a valid manifest version `1` backup set, but `restore`, `migrate`, and `smoke` require manifest version `2` with explicit runtime metadata
- `restore` fails closed unless `manifest.scope` matches `--scope`; use `migrate` for intentional cross-scope restore
- `restore` fails closed unless the manifest runtime block matches the target runtime contract. Same-scope `restore` also requires `runtime.db_name` to match the target `DB_NAME`
- `migrate` fails closed unless the manifest version `2` runtime block matches the shared target stack contract for images, service names, and storage contract. Source and target `DB_NAME` may differ across scopes, so `runtime.db_name` is recorded in the manifest but is not used to block cross-scope `migrate`
- `restore`, `migrate`, and `smoke` reset the target database as MariaDB root before importing the dump
- `restore` and `migrate` restore files through staged extraction: the archive is validated, extracted into staging, the staged tree is validated, and target storage is cleared only after staging succeeds
- `espops` never guesses `ESPO_RUNTIME_UID` or `ESPO_RUNTIME_GID` from the image or the container runtime
- `restore`, `migrate`, and `smoke` apply the restored storage tree to the explicit runtime uid/gid and fail closed if the operator cannot apply that ownership
- `doctor` succeeds only when Compose config parses, backup root and storage dir checks pass, contract services are listed, contract services are `running` and `healthy`, and MariaDB `SELECT 1` succeeds
- `restore`, `migrate`, and `smoke` succeed only after the restored storage post-check passes and the contract services are `running` and `healthy` before MariaDB `SELECT 1`; this validates Compose service health, not a full browser or user-login flow
- `doctor` is read-only and does not take an operation lock
- `backup` locks its scope before runtime validation, disk checks, service stop, and artifact creation
- `restore` locks its target scope before manifest verify, snapshot backup, service stop, database reset, and storage mutation
- `migrate` locks only its target scope because only the target mutates; the source scope is read-only manifest input
- `smoke` locks both scopes for the whole flow, in deterministic scope-key order, so it cannot overlap with manual mutating commands

## Minimal Safe Workflow

Prepare the target scope first:

1. Ensure `compose.yaml` and `.env.<scope>` exist.
2. Ensure `BACKUP_ROOT` already exists and is writable.
3. Ensure `ESPO_STORAGE_DIR` already exists, points at the correct scope storage, and is clearable by the operator account for `restore` and `migrate`.
4. Pre-pull the exact runtime images you intend to trust.
5. Run `espops doctor`.
   Doctor does not stop or repair anything. It proves config parsing, local backup/storage prerequisites, contract service presence, Docker Compose service health, and DB reachability.
6. Expect `espops` to create `PROJECT_DIR/.espops/locks` on the first mutating command. That directory is the explicit operation-lock home.

Then use the commands in this order:

1. `espops backup`
   Backup checks `MIN_FREE_DISK_MB`, stops app services only after that guard passes, writes prefix-based artifacts under `BACKUP_ROOT`, writes manifest version `2` with the explicit runtime block, self-verifies the new set, then applies strict same-prefix retention if `BACKUP_RETENTION_DAYS` is greater than zero.
2. `espops backup verify`
   Verify the manifest you plan to trust. Version `1` remains a verify-only diagnostic path; destructive commands require manifest version `2`.
3. `espops restore`
   Restore is destructive for the target scope. It verifies the source manifest first, requires manifest version `2`, requires a same-scope manifest, blocks if the manifest runtime block does not match the target runtime contract, creates a target snapshot before mutation, resets the target database, imports into the clean database, then restores files through staged extraction, clears target storage only after staging succeeds, applies the explicit runtime uid/gid before the final file post-check, starts the contract services, waits for Docker Compose health to go green, and only then accepts MariaDB `SELECT 1` as the final post-check.
4. `espops migrate`
   Migrate is thin composition over verified restore flow, not a separate engine. It requires manifest version `2`, inherits the same final health contract, checks the recorded image and service contract before mutation, and is the only supported cross-scope restore path.

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

`doctor` is a strict readiness check, not a loose preflight. A green result means the configured Compose services exist, are `running`, report `healthy`, and MariaDB answers a ping.

`doctor` does not acquire the mutating operation lock. It may observe a changing system if you run it concurrently with `backup`, `restore`, `migrate`, or `smoke`.

Create a backup:

```bash
./bin/espops backup --scope dev --project-dir /path/to/project
./bin/espops backup --scope prod --project-dir /path/to/project
```

Verify a backup set:

```bash
./bin/espops backup verify --manifest /path/to/manifest.json
```

`backup verify` proves manifest/artifact/checksum integrity. A version `1` result is still diagnostic only; it is not a restore-ready result.

Restore from a verified manifest:

```bash
./bin/espops restore --scope dev --project-dir /path/to/project --manifest /path/to/manifest.json
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
```

Success here means staged files passed validation, Docker Compose contract services came back `running` and `healthy`, and MariaDB answered the final ping. It does not claim a full browser/login flow.

`restore` requires manifest version `2` with explicit runtime metadata and blocks before snapshot or mutation if the recorded runtime contract does not match the target scope.

If another mutating `espops` process already owns the same scope lock, `restore` fails fast before snapshot, database reset, or storage mutation.

Migrate from one scope into another:

```bash
./bin/espops migrate --from-scope dev --to-scope prod --project-dir /path/to/project --manifest /path/to/manifest.json
```

`migrate` uses the same post-check contract as `restore`: storage checks plus Compose service health plus DB ping.

`migrate` locks only the target scope, because the source manifest is read-only input and the target scope is the one being reset and rewritten.

`migrate` also requires manifest version `2`. It checks the recorded image and service contract before touching the target scope and blocks version `1` manifests.

Run the fixed smoke path:

```bash
./bin/espops smoke --from-scope dev --to-scope prod --project-dir /path/to/project
```

`smoke` acquires both scope locks for the full flow before it starts its doctor/backup/restore/migrate chain.

## Repository Layout

- `cmd/espops/`: program entrypoint only
- `internal/cli/`: root command surface, envelopes, and exit mapping
- `internal/config/`: env-file loading and config validation
- `internal/ops/`: operation workflows and post-checks
- `internal/runtime/`: Docker Compose and MariaDB command execution
- `internal/manifest/`: backup manifest contract and artifact resolution
- `deploy/`: runtime tuning files used by `compose.yaml`
- `env/`: example env files
