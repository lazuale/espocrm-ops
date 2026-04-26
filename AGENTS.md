# AGENTS

## Product Boundary

- The product commands are exactly `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.
- `cmd/espops/` owns only the process entrypoint.
- `internal/` owns product behavior.
- Keep one direct Go code path; do not add a second runtime or hidden alternate path.
- Keep shell execution, Docker Compose execution, MariaDB execution, and process env forwarding in `internal/runtime/docker.go`.

## Runtime Contract

- Keep `compose.yaml`, env examples, `README.md`, and Go config validation on one literal contract.
- Env files are literal `KEY=VALUE` only; no quotes, spaces, or shell expansion syntax.
- `DB_SERVICE` and `APP_SERVICES` are explicit runtime inputs; do not infer or default them.
- `DB_SERVICE` and every service in `APP_SERVICES` must expose a Docker Compose healthcheck.
- Success requires explicit health or post-check evidence; MariaDB reachability alone is not success.
- The shipped Compose/env contract uses inline `DB_PASSWORD` and `DB_ROOT_PASSWORD`; do not advertise file-based password env keys.
- For `prod`, `.env.prod` regular-file and `0600` hygiene is a warning, not a blocker; unreadable or unparsable env files still fail.
- Do not claim mutable image tags are as reproducible as digest-pinned refs; missing digest pinning is a warning for this internal deployment.
- For `migrate` from `dev` to `prod`, both scopes must use the same `ESPOCRM_IMAGE` and `MARIADB_IMAGE` refs; digest pinning is recommended but not a hard fail.
- `restore` and `migrate` require explicit `DB_ROOT_PASSWORD`, `ESPO_RUNTIME_UID`, and `ESPO_RUNTIME_GID`; do not fall back to `DB_USER` or infer ownership.

## Operation Safety

- Fail closed when correctness is ambiguous.
- No auto-repair, auto-normalization, or silent recovery.
- No mutating operation without the explicit per-scope operation lock.
- `restore` and `migrate` must not mutate from manifest version `1` or from a manifest without explicit runtime metadata.
- `restore` is same-scope only; `migrate` is the cross-scope restore path.
- Destructive flows must validate input, reset the target database as MariaDB root, stage storage changes, apply explicit runtime ownership, run post-checks, and report success only after contract services are healthy.
- Do not add product commands, decorative env keys, or shell-owned product semantics.

## Test And Docs

- Do not claim reliability improvement without scenario proof.
- Do not claim integration coverage without real Docker integration evidence.
- Keep `README.md` for operators and `CONTRIBUTING.md` for developers.
- Run `make ci` before claiming repository health after product, workflow, or contributor-path changes.
