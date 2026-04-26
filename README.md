# espocrm-ops

`espops` is a small internal JSON CLI for one EspoCRM Docker Compose server.

Commands:

- `doctor`
- `backup`
- `backup verify`
- `restore`

## Build

```bash
make build
```

The binary is written to `bin/espops`.

## Project

Run `espops` against a project directory containing:

- `compose.yaml`
- `.env.<scope>`

Example:

```bash
./bin/espops doctor --scope prod --project-dir /path/to/project
```

Env files are parsed as literal `KEY=VALUE` lines only. Quotes, spaces, shell expansion syntax, and duplicate keys fail. Keys not used by `espops` are ignored and left for Docker Compose.

Env keys read by `espops`:

- `BACKUP_ROOT`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_PASSWORD`
- `DB_ROOT_PASSWORD`
- `DB_NAME`

`APP_SERVICES` is a comma-separated list. `DB_SERVICE` and every service in `APP_SERVICES` must expose a Docker Compose healthcheck.

## Backup Set

`backup` writes:

```text
BACKUP_ROOT/<scope>/<timestamp>/
  manifest.json
  db.sql.gz
  files.tar.gz
```

The manifest schema is:

```json
{
  "scope": "prod",
  "created_at": "2026-04-26T13:00:00Z",
  "db": {"file": "db.sql.gz", "sha256": "..."},
  "files": {"file": "files.tar.gz", "sha256": "..."},
  "db_name": "espocrm"
}
```

## Doctor

`doctor` fails unless:

- config parses
- `docker compose config` works
- `BACKUP_ROOT` is writable
- `ESPO_STORAGE_DIR` exists and is a directory
- native `tar` exists
- configured services are healthy
- MariaDB ping works

## Backup

```bash
./bin/espops backup --scope prod --project-dir /path/to/project
./bin/espops backup verify --manifest /path/to/backups/prod/<timestamp>/manifest.json
```

`backup` acquires the project lock, checks prerequisites, creates the backup directory, stops app services, dumps MariaDB to `db.sql.gz`, archives storage to `files.tar.gz`, starts app services, waits for health, writes SHA-256 checksums into `manifest.json`, and verifies the new backup set before reporting success.

## Restore

```bash
./bin/espops backup verify --manifest /path/to/backups/prod/<timestamp>/manifest.json
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/backups/prod/<timestamp>/manifest.json
```

`restore` is destructive and same-scope only. The manifest scope and `db_name` must match the target config. It acquires the project lock, verifies the manifest, creates a target snapshot backup, extracts `files.tar.gz` to staging, stops app services, resets the configured database as MariaDB root, imports `db.sql.gz`, switches storage by same-parent rename, starts app services, waits for health, and runs DB ping.

If restore fails after the snapshot exists, the JSON result includes `snapshot_manifest`.

## JSON Output

Every command writes one JSON object to stdout. Success has `"ok": true`; failure has `"ok": false` and an `error` object.
