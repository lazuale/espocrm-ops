# espocrm-ops

`espops` is a small internal CLI wrapper for one EspoCRM Docker Compose server.

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

Env files are parsed as `KEY=VALUE` lines. For keys read by `espops`, quotes, spaces, and shell expansion syntax fail. Duplicate keys fail. Other keys are left for Docker Compose.

Env keys read by `espops`:

- `BACKUP_ROOT`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_PASSWORD`
- `DB_ROOT_PASSWORD`
- `DB_NAME`

`APP_SERVICES` is a comma-separated list.

## Backup Set

`backup` writes:

```text
BACKUP_ROOT/<scope>/<timestamp>/
  manifest.json
  db.sql.gz
  files.tar.gz
```

`manifest.json` contains only checksums:

```json
{
  "db": "...",
  "files": "..."
}
```

## Doctor

`doctor` fails unless:

- config parses
- `docker`, `tar`, `gzip`, and `sha256sum` exist
- `docker compose config` works
- `BACKUP_ROOT` exists and is writable
- `ESPO_STORAGE_DIR` exists and is a directory
- MariaDB accepts `SELECT 1`

## Backup

```bash
./bin/espops backup --scope prod --project-dir /path/to/project
./bin/espops backup verify --manifest /path/to/backups/prod/<timestamp>/manifest.json
```

`backup` runs `docker compose stop`, dumps MariaDB through `mariadb-dump | gzip`, archives storage with `tar`, starts app services with `docker compose up -d`, and writes `sha256sum` values to `manifest.json`.

`backup verify` only checks that the fixed files exist, `sha256sum` matches, and `gzip -t` accepts both `db.sql.gz` and `files.tar.gz`. It does not inspect tar contents.

## Restore

```bash
./bin/espops backup verify --manifest /path/to/backups/prod/<timestamp>/manifest.json
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/backups/prod/<timestamp>/manifest.json
```

`restore` is destructive and linear: verify, stop app services, reset the database as MariaDB root, import `db.sql.gz`, replace `ESPO_STORAGE_DIR`, extract `files.tar.gz`, start app services.

## Output

Stdout is plain text. Errors go to stderr. Exit codes are `0` for success, `1` for failure, and `2` for bad CLI usage.
