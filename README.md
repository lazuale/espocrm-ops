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

## External Commands

The Go code does not invoke a shell. The Docker CLI is the only external binary used, and it is called directly with argument lists.

## Development

```text
make fmt
make test
make build
make check
```
