# Contributing

`README.md` is the operator contract. This file is for changing the product without widening that contract by accident.

## Product Shape

The shipped command surface is fixed:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

Production behavior lives in:

- `cmd/espops/`: process entrypoint only
- `internal/cli/`: command wiring, argument validation, JSON envelopes, exit mapping
- `internal/config/`: env-file parsing and config loading
- `internal/ops/`: operation workflows, locks, post-checks
- `internal/runtime/`: Docker Compose and MariaDB process execution
- `internal/manifest/`: manifest validation and artifact path resolution

Keep shell execution and `os.Environ()` confined to `internal/runtime/docker.go`.

Do not add a second runtime, hidden alternate path, auto-repair, guessed service names, or product commands beyond the shipped set.

## Runtime Invariants

Keep `compose.yaml`, `env/.env.*.example`, `README.md`, `internal/config/`, and `repository_test.go` on one contract.

Required invariants:

- `DB_SERVICE` and `APP_SERVICES` stay explicit; no inferred defaults.
- `DB_SERVICE` and every `APP_SERVICES` entry must have a Compose healthcheck.
- Env files stay literal `KEY=VALUE` only; no quotes, spaces, or shell expansion syntax.
- Success requires service health plus explicit operation post-checks; MariaDB reachability alone is not enough.
- Mutating operations must acquire the per-scope cross-process operation lock before mutation.
- New backups write manifest version `2` with runtime metadata; `restore` and `migrate` must block manifest version `1` before mutation.
- `restore` and `migrate` require explicit `DB_ROOT_PASSWORD`; never fall back to `DB_USER` for database reset.
- `restore` and `migrate` require explicit `ESPO_RUNTIME_UID` and `ESPO_RUNTIME_GID`; never infer ownership from the image, container, or operator account.
- Shipped Compose/env contract uses inline `DB_PASSWORD` and `DB_ROOT_PASSWORD`; do not advertise file-based password env keys.
- `prod` env loading stays fail-closed: `.env.prod` must be a regular non-symlink file with mode exactly `0600`.
- Do not call mutable image tags production-safe unless deployed image refs are digest-pinned.
- For `migrate` from `dev` to `prod`, both scopes must use the same digest-pinned `ESPOCRM_IMAGE` and `MARIADB_IMAGE` refs.
- Do not add decorative env keys to examples.

## Local Tools

Recommended:

- Go `1.26.x`
- Docker with the Compose plugin
- `staticcheck`
- `golangci-lint`

Install Go-side health tools:

```bash
make install-health-tools
```

## Test Paths

Fast local layer:

```bash
make test
make test-race
make test-readonly
make ci-fast
```

Docker integration layer:

```bash
make pull-images
make integration
make ci-integration
```

Full health claim:

```bash
make ci
```

`make test` may use fake docker scripts where CLI wiring or failure shaping is the unit under test. Do not claim integration coverage from those tests.

`make integration` is the real Docker layer. It requires a live Docker daemon, the Compose plugin, and required images available locally. It must not silently pass with zero real integration tests.

`make ci-fast` is the pull-request path and must not pull Docker images. `make ci` runs both fast checks and Docker integration.

## Change Flow

1. Make the smallest product change that preserves the command surface and package ownership.
2. Run the smallest relevant tests while iterating.
3. Add scenario proof for reliability-sensitive changes:
   - health/post-check changes need `internal/runtime/` and `internal/ops/` coverage
   - lock changes need lock tests plus flow tests proving lock acquisition before mutation
   - manifest/runtime-contract changes need `internal/manifest/` and `internal/ops/` coverage proving destructive commands block before mutation
4. Run `make ci-fast` before claiming pull-request health.
5. Run `make ci` before claiming full repository health after product, workflow, or contributor-path changes.
6. Update `README.md`, `CONTRIBUTING.md`, and `AGENTS.md` when command behavior or the runtime contract changes.

Repo-wide guards live in `repository_test.go`.
