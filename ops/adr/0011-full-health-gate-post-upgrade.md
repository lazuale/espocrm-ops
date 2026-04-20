# Full Post-Upgrade Repository Health Requires the Canonical Full Gate

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

This ADR was later narrowed by ADR 0015 for `govulncheck` handling. Use `AGENTS.md`, `Makefile`, and `.github/workflows/ai-governance.yml` for the current blocking gate.

## Status

Accepted

## Date

2026-04-18

## Context

After the Go/toolchain/dependency upgrade audit, the repository's canonical fast gate was no longer sufficient to support repo-health claims:

- `make ci` only covered the fast governance/build path
- CI did not run race detection, vulnerability scanning, static analysis, linting, coverage reporting, integration checks, or the shell regression suite
- local extended health checks were materially stronger than CI
- future agents were still instructed to use `make ci` as if it were enough to claim that the repository was healthy

That mismatch made health claims misleading after the upgrade.

## Decision

Introduce and enforce a two-tier health standard:

- `make check-fast` is the canonical fast gate and matches the existing quick governance/build cycle
- `make ci` remains an alias for the fast gate so existing fast-entrypoint usage keeps working
- `make check-full` is the authoritative full repository health gate for post-upgrade health claims

At acceptance time, the intended `make check-full` order was:

1. `make ai-refresh`
2. `make ai-check`
3. the fast gate
4. `go test -race ./...`
5. `govulncheck ./...`
6. `staticcheck ./...`
7. `golangci-lint run --no-config ./...`
8. `go test ./... -coverprofile=coverage.out`
9. `go tool cover -func=coverage.out`
10. `make integration`
11. `make regression`

GitHub Actions must install the required Go health tools and run the full gate instead of the fast gate.

## Consequences

- The repository can no longer be declared healthy from `make ci` alone after Go, toolchain, dependency, workflow, or enforcement-surface changes.
- CI now reflects the stronger post-upgrade health standard and will surface existing repo issues instead of hiding them behind the fast gate.
- Health-tool versions are pinned in repo-managed enforcement so local and CI usage share the same baseline.

## Rules

- Do not downgrade CI or agent instructions back to the fast gate for post-upgrade health claims.
- Keep `AGENTS.md`, `Makefile`, and `.github/workflows/ai-governance.yml` aligned whenever the full health standard changes.
- Do not treat a failing `make check-full` result as justification to weaken the gate.
