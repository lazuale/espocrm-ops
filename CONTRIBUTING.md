# Contributing

This repository is intentionally small. Keep it stupid.

## Product Shape

The product commands are exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`

## Code Boundaries

- `cmd/espops/` is only the process entrypoint.
- `internal/cli/` owns argument parsing and plain text output.
- `internal/config/` owns literal env parsing.
- `internal/ops/` owns the direct command order.
- `internal/runtime/docker.go` owns calls to `docker compose`, `mariadb-dump`, `mariadb`, `tar`, `gzip`, and `sha256sum`.

Do not add runtime interfaces, adapters, DI, JSON output, structured error taxonomies, health polling, rollback, staging, snapshots, or retry loops.

## Backup Shape

Backup layout:

```text
BACKUP_ROOT/<scope>/<timestamp>/
  manifest.json
  db.sql.gz
  files.tar.gz
```

Manifest:

```json
{
  "db": "...",
  "files": "..."
}
```

Do not duplicate scope, timestamp, database name, or fixed file names in the manifest.

## Restore Order

`restore` must stay linear:

1. Verify manifest/checksums/gzip readability.
2. Stop app services.
3. Reset the DB as MariaDB root.
4. Import the DB dump.
5. Replace the storage directory.
6. Extract files.
7. Start app services.

No rollback. No staging. No snapshot.

## Verification

Before handing off repository health, run:

```bash
go test ./...
```

Use `make build` for a local binary.
