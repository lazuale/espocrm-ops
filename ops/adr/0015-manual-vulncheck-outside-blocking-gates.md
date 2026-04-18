# Manual Vulnerability Scan Outside Blocking Gates

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The post-upgrade full health gate had been strengthened to include `govulncheck ./...` as a blocking step in both `make check-full` and the GitHub Actions merge gate.

In practice, that step introduced a failure mode that was not repository-side:

- `govulncheck` depends on live vulnerability-database fetches
- repeated local and CI runs were vulnerable to external timeout and availability failures
- the rest of the repository health sequence could still pass while `govulncheck` failed for network reasons alone

That made the blocking gate less trustworthy as a measure of repository health because an external service outage could stop `make check-full` and CI before any real repo-side blocker was reached.

## Decision

Keep `govulncheck` available as a manual operator command, but remove it from the blocking `make check-full` sequence and from CI's blocking gate path.

This decision keeps the core health standard strong while separating repo-owned failures from external vulnerability-feed availability:

- `make vulncheck` remains the manual entrypoint for vulnerability scanning
- `make check-full` no longer runs `govulncheck`
- GitHub Actions continues to run `make check-full`, so CI no longer blocks on `govulncheck`
- CI installs only the tools required by the blocking gate path

## Consequences

- `make check-full` now measures repository-side health without being interrupted by external vuln-feed timeouts.
- Vulnerability scanning remains available, but it is now an explicit manual action instead of a gating prerequisite.
- The repository can still record and act on vulnerability findings without conflating them with blocking CI availability problems.

## Rules

- Keep `make vulncheck` manual-only unless a future explicit decision restores it to a blocking gate.
- Do not describe `govulncheck` as part of `make check-full` or the CI merge gate after this decision.
- Keep `AGENTS.md`, `Makefile`, `.github/workflows/ai-governance.yml`, and any directly entangled regression expectations aligned when the health gate definition changes.
