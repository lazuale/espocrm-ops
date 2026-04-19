# Canonical Operation Gate Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-20

## Context

The repository already had several Go-owned operational inspection and
execution surfaces:

- `doctor` for readiness validation
- `status-report` and `health-summary` for canonical operational posture
- `backup-health` for backup policy posture
- `update-plan`, `rollback-plan`, and `restore --dry-run` for action previews
- Go-owned execution for `update`, `rollback`, `restore`, `restore-drill`, and
  `migrate-backup`

That still left one operator and automation gap: there was no single
authoritative Go path that answered whether a requested risky action is
currently allowed, merely risky, or blocked.

Operators could inspect health and plan surfaces manually, but automation still
had to reconstruct one decision from multiple commands:

- whether current contour health should block a stateful run or only degrade
  confidence
- whether an action-specific dry-run or preflight is ready
- whether current lock, latest-operation, or backup-selection signals should
  stop the requested action
- whether action flags reduce safeguards enough to require operator review

Without one canonical decision boundary, a future shell wrapper or ad hoc
client could become the real readiness controller for risky actions.

## Decision

Add a canonical public Go command, `operation-gate`, backed by a Go-owned
`operationgate` use case.

`operation-gate` is the single authoritative readiness decision path for risky
operator actions:

- it supports `update`, `rollback`, `restore`, `restore-drill`, and
  `migrate-backup`
- it aggregates existing Go-owned surfaces instead of creating a new shell or
  duplicate policy controller
- it consumes Go-owned `health-summary` for cross-cutting operational posture
- it consumes Go-owned action previews or preflights:
  `update-plan`, `rollback-plan`, `restore --dry-run`, plus Go preflight checks
  for `restore-drill` and `migrate-backup`
- it produces one explicit decision:
  `allowed`, `risky`, or `blocked`
- it reports explicit alert items with severity, cause attribution, and next
  action
- it reports explicit included, omitted, and failed section status for the
  component decisions it assembles
- it keeps one concise text output for operators and one stable JSON contract
  for automation
- it treats `blocked` as a structured non-zero validation outcome and treats
  incomplete required decision collection as `operation_gate_failed`

The command intentionally preserves one clear authoritative path. It does not
introduce shell-owned gating logic, duplicate `can-run` aliases, or external
policy engine integration in this pass.

## Consequences

- operators and automation now have one canonical Go-owned readiness decision
  surface instead of stitching together health, plans, and preflight logic
- future readiness automation must extend the Go-owned operation gate instead
  of rebuilding shell decision logic
- the governed CLI contract expands with a new `operation-gate` command and a
  canonical JSON fixture baseline

## Rules

- Use `operation-gate` as the canonical readiness decision path for risky
  operator actions.
- Keep operation-gate implementation Go-owned end to end.
- Aggregate existing Go-owned health, plan, and preflight surfaces instead of
  adding another shell or duplicate controller stack.
- Preserve explicit decision semantics:
  `allowed`, `risky`, and `blocked`.
- Treat `blocked` as a structured non-zero validation outcome and treat
  incomplete required decision collection as `operation_gate_failed`.
- Do not add duplicate operation readiness entrypoints or shell-owned gating
  logic.
