# AGENTS

## Product Boundary

- The product commands are exactly `doctor`, `backup`, `backup verify`, and `restore`.
- This is a small internal ops wrapper for one EspoCRM Docker Compose server.
- It is not a platform, security product, backup framework, restore framework, API, or secrets framework.
- `cmd/espops/` owns only the process entrypoint.
- `internal/` owns the direct command behavior.
- Keep one direct Go path. Do not add a second runtime, adapter layer, DI layer, or hidden alternate path.

## Allowed Tools

- Go may launch processes and do primitive filesystem work.
- Go must not implement Docker, MariaDB, tar, gzip, or sha256 behavior.
- Use CLI tools:
  - `docker compose`
  - `mariadb-dump`
  - `mariadb`
  - `tar`
  - `gzip`
  - `sha256sum`
- Keep these process calls in `internal/runtime/docker.go`.

## CLI Contract

- Stdout is plain text.
- Stderr is errors.
- Exit codes are only:
  - `0` success
  - `1` failed command/runtime/filesystem/checksum
  - `2` bad CLI usage
- Do not add JSON output, DTO responses, structured error taxonomies, or API-style envelopes.

## Backup Shape

Backup layout is exactly:

```text
BACKUP_ROOT/<scope>/<timestamp>/
  manifest.json
  db.sql.gz
  files.tar.gz
```

`manifest.json` contains only the checksums needed to verify the two fixed artifacts:

```json
{
  "db": "...",
  "files": "..."
}
```

Do not duplicate scope, timestamp, database name, or fixed artifact filenames in the manifest.

Env files are `KEY=VALUE` lines. For keys read by `espops`, quotes, spaces, and shell expansion fail. Duplicate keys fail.

Env keys read by `espops` are:

- `BACKUP_ROOT`
- `ESPO_STORAGE_DIR`
- `APP_SERVICES`
- `DB_SERVICE`
- `DB_USER`
- `DB_PASSWORD`
- `DB_ROOT_PASSWORD`
- `DB_NAME`

Other env keys are ignored by `espops` and left for Docker Compose.

## Operation Rules

- No retries.
- No fallback.
- No health polling.
- No Docker output parsing.
- No project lock.
- No snapshot restore.
- No staging restore.
- No rollback or recovery logic.
- `backup verify` only checks:
  - `manifest.json`, `db.sql.gz`, and `files.tar.gz` exist
  - `sha256sum` matches the manifest values
  - `gzip -t` accepts both gzip files
- `restore` is a linear script:
  - verify
  - stop app services
  - reset database
  - import database
  - replace storage directory
  - extract files
  - start app services

## Test And Docs

- Keep `README.md` for operators and `CONTRIBUTING.md` for developers.
- Keep tests focused on the minimal contract.
- Do not keep broad repository guards that enforce removed architecture.
- Run `go test ./...` before claiming repository health after product, workflow, or contributor-path changes.
