# Canonical Health Summary Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already had several Go-owned operational inspection and recovery
surfaces:

- `doctor` for readiness validation
- `overview` for operator dashboard context
- `status-report` for detailed contour/runtime state
- `backup-health` for backup freshness and policy posture
- Go-owned journal history, operation report, export, and maintenance flows

That still left one operator and automation gap: there was no single canonical
Go surface that assembled the highest-value operational signals into one
authoritative contour verdict.

Operators and cron/CI consumers could inspect multiple Go commands, but they
still had to infer the final contour posture themselves:

- whether readiness was merely warned or actually blocking
- whether runtime collection succeeded cleanly
- whether backup posture was healthy enough to trust
- whether the latest operation outcome should degrade confidence
- whether maintenance lock state was affecting safe automation

Without one canonical summary boundary, a future shell or ad hoc client could
become the real controller for alerts and verdict semantics.

## Decision

Add a canonical public Go command, `health-summary`, backed by a Go-owned
`healthsummary` usecase.

`health-summary` is the single authoritative contour verdict and alert path:

- it aggregates existing Go-owned health signals instead of creating a new
  shell or duplicate controller stack
- it consumes canonical status data from the Go-owned status-report path
- it consumes canonical backup posture from the Go-owned backup-health path
- it produces one explicit overall verdict:
  `healthy`, `degraded`, `blocked`, or `failed`
- it reports explicit alert items with severity, cause attribution, and next
  action
- it reports explicit included, omitted, and failed section status for the
  component summaries it assembles
- it keeps one concise text output for operators and one stable JSON contract
  for automation
- it treats `blocked` as a validation failure with a structured result payload
  and treats incomplete summary collection as `failed`

The command intentionally preserves one clear authoritative path. It does not
introduce a parallel `health` alias, a shell-owned controller, or external
monitoring integration logic in this pass.

## Consequences

- operators and automation now have one canonical Go-owned contour verdict
  surface instead of stitching together `doctor`, `status-report`,
  `backup-health`, and journal state themselves
- the governed CLI contract expands with a new `health-summary` command and a
  canonical JSON fixture baseline
- future health-related automation must extend the Go-owned summary path rather
  than rebuilding controller logic in shell

## Rules

- Use `health-summary` as the canonical contour verdict and alert path.
- Keep the health-summary implementation Go-owned end to end.
- Aggregate existing Go-owned surfaces instead of creating another discovery or
  shell orchestration stack.
- Preserve explicit overall verdict semantics:
  `healthy`, `degraded`, `blocked`, and `failed`.
- Treat `blocked` as a structured non-zero validation outcome and treat
  incomplete required summary collection as `failed`.
- Do not add duplicate health-summary automation entrypoints or shell-owned
  alert assembly logic.
