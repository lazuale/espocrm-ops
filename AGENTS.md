# AGENTS

## Product Boundary

- The product commands are exactly `doctor`, `backup`, `backup verify`, `restore`, `migrate`, and `smoke`.
- `cmd/espops/` owns only the process entrypoint.
- `internal/` owns product behavior.
- Keep one direct Go code path; do not add a second runtime or hidden alternate path.
- Keep shell execution, Docker Compose execution, MariaDB execution, and process env forwarding in `internal/runtime/docker.go`.

## Runtime Contract

- Keep `compose.yaml`, env examples, `README.md`, and Go config validation on one literal contract.
- `DB_SERVICE` and `APP_SERVICES` are explicit runtime inputs; do not infer or default them.
- `DB_SERVICE` and every service in `APP_SERVICES` must expose a Docker Compose healthcheck.
- Success requires explicit health or post-check evidence; MariaDB reachability alone is not success.
- The shipped Compose/env contract uses inline `DB_PASSWORD` and `DB_ROOT_PASSWORD`; do not advertise file-based password env keys.
- For `prod`, `.env.prod` must stay a regular non-symlink file with permissions no broader than `0600` or `0640`.
- Do not claim mutable image tags are production-safe unless deployed image refs are digest-pinned.
- `restore`, `migrate`, and `smoke` require explicit `DB_ROOT_PASSWORD`, `ESPO_RUNTIME_UID`, and `ESPO_RUNTIME_GID`; do not fall back to `DB_USER` or infer ownership.

## Operation Safety

- Fail closed when correctness is ambiguous.
- No auto-repair, auto-normalization, silent recovery, or implicit path switching.
- No mutating operation without the explicit per-scope operation lock.
- `restore`, `migrate`, and `smoke` must not mutate from manifest version `1` or from a manifest without explicit runtime metadata.
- `restore` is same-scope only; `migrate` is the cross-scope restore path.
- Destructive flows must validate input, reset the target database as MariaDB root, stage storage changes, apply explicit runtime ownership, run post-checks, and report success only after contract services are healthy.
- Do not add product commands, decorative env keys, or shell-owned product semantics.

## Test And Docs

- Do not claim reliability improvement without scenario proof.
- Do not claim integration coverage without real Docker integration evidence.
- Keep `README.md` for operators and `CONTRIBUTING.md` for developers.
- Run `make ci` before claiming repository health after product, workflow, or contributor-path changes.
