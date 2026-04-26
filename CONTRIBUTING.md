# Contributing

This repository is intentionally small. Treat old code as donor material only.

## Product Shape

The product commands are exactly:

- `doctor`
- `backup`
- `backup verify`
- `restore`

Do not add `migrate`, retention, sidecar checksum files, warnings, digest pinning policy, runtime contract v2, `storage_contract`, `BACKUP_NAME_PREFIX`, `BACKUP_RETENTION_DAYS`, `ESPO_CONTOUR`, cloud/offsite storage, or a secrets framework.

## Code Boundaries

- `cmd/espops/` is only the process entrypoint.
- `internal/cli/` owns command parsing and JSON output.
- `internal/config/` owns literal env parsing and config validation.
- `internal/manifest/` owns manifest read/write/validation.
- `internal/ops/` owns command orchestration.
- `internal/runtime/docker.go` owns Docker Compose, MariaDB, native tar/gzip execution, and process env forwarding.

Avoid abstractions until two real call sites need them.

## Runtime Contract

Env files are literal `KEY=VALUE` only. The product env keys are:

- `BACKUP_ROOT`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_PASSWORD`
- `DB_ROOT_PASSWORD`
- `DB_NAME`

Backup layout:

```text
BACKUP_ROOT/<scope>/<timestamp>/
  manifest.json
  db.sql.gz
  files.tar.gz
```

Manifest version `1`:

```json
{
  "version": 1,
  "scope": "...",
  "created_at": "...",
  "db": {"file": "db.sql.gz", "sha256": "..."},
  "files": {"file": "files.tar.gz", "sha256": "..."},
  "db_name": "...",
  "db_service": "...",
  "app_services": ["..."]
}
```

## Restore Order

`restore` must:

1. Acquire the simple project lock.
2. Verify the source manifest and checksums.
3. Create a target snapshot backup.
4. Extract files to staging.
5. Stop app services.
6. Reset the DB as MariaDB root.
7. Import the DB dump.
8. Switch storage by same-parent rename.
9. Start app services.
10. Check service health.
11. Run DB ping.

Do not report success before health and DB ping pass.

## Verification

Before handing off repository health, run:

```bash
go test ./...
```

Use `make build` for a local binary.
