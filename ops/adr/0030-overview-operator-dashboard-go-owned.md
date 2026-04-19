# Overview Becomes The Canonical Operator Dashboard

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already had strong Go-owned operator surfaces for readiness,
detailed status, operation history, backup inventory, and support diagnostics.
Even with those surfaces in place, operators still lacked one clearly
designated top-level dashboard entrypoint that answered the most important
question first: what is the current contour state right now?

The existing Go-owned `overview` command was already the closest summary
surface, but it did not yet present the full high-value dashboard shape:

- resolved contour identity and env context were still only present in the more
  detailed `status-report`
- the overview summary highlighted recent operations instead of leading with
  the latest operation and the best drill-down path
- the command needed to become the explicit single dashboard path so the repo
  did not grow another competing top-level `summary` or `dashboard` command

## Decision

Extend the existing Go-owned `overview` command so it becomes the canonical
top-level operator dashboard and summary path.

`espops overview` is now the authoritative dashboard owner for:

- resolved contour identity and env context summary
- doctor readiness summary
- runtime status summary
- latest operation summary
- backup summary
- explicit included-versus-omitted-versus-failed section reporting
- operator drill-down guidance toward `doctor`, `status-report`,
  `show-operation`, `history`, `backup-catalog`, and `show-backup`
- canonical text, JSON, error, and exit-code behavior for the high-level
  operator dashboard

Do not add a second top-level dashboard noun such as `summary` or `dashboard`
while `overview` remains the canonical operator summary path.

## Consequences

- operators gain one clear Go-owned dashboard entrypoint without introducing a
  duplicate top-level summary command
- the governed `overview` JSON contract changes because the dashboard now
  includes context and latest-operation sections plus drill-down guidance
- `status-report` remains the deeper detailed report, while `overview` becomes
  the concise operator dashboard that points into the detailed surfaces
- the compatibility wrapper `scripts/contour-overview.sh` continues to delegate
  immediately to `espops overview` and must not retake dashboard orchestration

## Rules

- Treat `espops overview` as the single canonical top-level operator dashboard
  and summary path.
- Do not add another sibling dashboard command unless repository authority is
  updated intentionally.
- Keep dashboard orchestration, section selection, JSON output, and exit-code
  behavior Go-owned.
- Use `status-report`, history/report surfaces, and backup inventory primitives
  as the drill-down path instead of rebuilding parallel summary controllers.
