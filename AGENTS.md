# AGENTS

## Product Boundary

- The product commands are exactly `doctor`, `backup`, `backup verify`, and `restore`.
- This is a small internal ops tool for one EspoCRM server.
- It is not a platform, security product, backup framework, or secrets framework.
- `cmd/espops/` owns only the process entrypoint.
- `internal/` owns product behavior.
- Keep one direct Go code path; do not add a second runtime or hidden alternate path.
- Keep Docker Compose execution, MariaDB execution, native tar execution, Go gzip DB streams, and process env forwarding in `internal/runtime/docker.go`.

## Backup Shape

- Backup layout is exactly:

  ```text
  BACKUP_ROOT/<scope>/<timestamp>/
    manifest.json
    db.sql.gz
    files.tar.gz
  ```

- Manifest contains only:
  - `scope`
  - `created_at`
  - `db.file` and `db.sha256`
  - `files.file` and `files.sha256`
  - `db_name`
- Env files are literal `KEY=VALUE` only; no quotes, spaces, shell expansion, or duplicate keys.
- Env keys read by `espops` are:
  - `BACKUP_ROOT`
  - `ESPO_STORAGE_DIR`
  - `APP_SERVICES`
  - `DB_SERVICE`
  - `DB_USER`
  - `DB_PASSWORD`
  - `DB_ROOT_PASSWORD`
  - `DB_NAME`
- Other env keys are ignored by `espops` and left for Docker Compose.
- `DB_SERVICE` and every service in `APP_SERVICES` are explicit runtime inputs and must be healthy before success is reported.

## Operation Safety

- Fail closed when correctness is ambiguous.
- No auto-repair, auto-normalization, hidden fallback, cloud/offsite integration, or secrets framework.
- Mutating operations acquire one simple project lock.
- Do not add multi-scope locks.
- `restore` is same-scope only; there is no cross-scope restore command.
- `restore` must verify the manifest and checksums, create a target snapshot backup, extract files to staging, stop app services, reset the DB as MariaDB root, import the DB, switch storage by same-parent rename, start app services, check service health, and run DB ping.
- Hard fail when:
  - backup cannot be created
  - backup does not verify
  - restore could mix DB/files because manifest metadata or checksums do not match
  - manifest/checksum does not match
  - service health fails
  - DB ping fails
  - `BACKUP_ROOT` is not writable
  - `ESPO_STORAGE_DIR` is missing or not a directory
  - native `tar` is missing

## Test And Docs

- Keep `README.md` for operators and `CONTRIBUTING.md` for developers.
- Keep tests focused on the minimal contract.
- Do not keep broad repository guards that enforce removed architecture.
- Run `go test ./...` before claiming repository health after product, workflow, or contributor-path changes.
