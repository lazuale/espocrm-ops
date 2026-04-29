# espocrm-ops

Small backup and restore helper for this EspoCRM Docker Compose deployment.

## Commands

```text
espops backup
espops check-backup <backup-dir>
espops restore <backup-dir> --yes
```

`restore` requires an explicit backup directory and the `--yes` flag. It refuses to continue without that flag.

## Configuration

The tool reads only `.env` from the current directory. Required keys:

```text
BACKUP_ROOT
ESPO_STORAGE_DIR
DB_USER
DB_PASSWORD
DB_ROOT_PASSWORD
DB_NAME
```

There are no defaults and no fallback to process environment variables.

## Backup Format

Each backup directory contains:

```text
db.sql.gz
files.tar.gz
SHA256SUMS
manifest.json
```

`manifest.json` is version 1 and records creation time, file names, SHA-256 hashes, and sizes. `check-backup` verifies the manifest, checksum file, compressed database dump, and compressed file archive before reporting success.

## Restore Safety

Restore validates the backup and extracts the file archive into a temporary storage directory before changing the database or live storage. The previous storage directory is renamed to:

```text
<ESPO_STORAGE_DIR>.before-restore-<timestamp>
```

It is kept in place and is not deleted automatically.

## Restore Safety Model

Restore is intentionally strict:

- The backup is fully validated before destructive actions.
- The files archive is extracted to a temporary directory before the database reset.
- The database reset and import happen before the storage swap.
- If the database import fails after reset, storage is not swapped and manual database recovery from the backup is required.
- If the final storage swap cannot place restored storage at `ESPO_STORAGE_DIR`, the tool attempts to roll the previous storage back into place.
- If that rollback also fails, the command reports both paths and the manual recovery action.
- Old storage is preserved after a successful swap.
- There is no fallback to a latest backup or legacy backup format.
- No shell is used.
- The Docker CLI is the only external binary used.

## External Commands

The Go code does not invoke a shell. The Docker CLI is the only external binary used, and it is called directly with argument lists.

`espops` redacts `MYSQL_PWD` in errors it creates for failed Docker commands. The password is still passed to `docker compose exec` as `MYSQL_PWD` for the database process.

## Development

```text
make fmt
make test
make build
make check
```
