# Restore, Update, and Rollback Share Execution-Step Rendering Internals

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The Go-owned execution flows for restore, restore-drill, update, update-plan,
rollback, and rollback-plan already had stable command behavior. The remaining
duplication cluster was in the CLI reporting path for execution steps:

- each command family rebuilt near-identical `result` item slices from
  usecase steps
- text rendering repeated the same per-step status, summary, details, and
  action formatting logic
- warning block rendering was duplicated in multiple execution and plan
  renderers
- future execution-flow changes would have required touching several command
  files to keep step rendering consistent

That duplication increased drift risk without adding any product value.

## Decision

Consolidate execution-step reporting internals around one shared CLI path while
keeping the external command contracts stable.

This pass introduces a focused shared execution-step helper that:

- maps restore, restore-drill, update, update-plan, rollback, and
  rollback-plan usecase steps into their existing `result` item shapes
- renders shared warning blocks for plan and execution surfaces that already
  expose warnings
- renders shared step lists with configurable section titles, status text
  formatting, and tolerant handling for legacy restore item decoding

The public JSON fields, text output semantics, and exit behavior remain
behaviorally stable in this consolidation pass.

## Consequences

- execution-flow commands now extend one shared step rendering path instead of
  repeating the same loop and formatting logic
- step-to-result-item translation is less likely to drift across restore,
  update, and rollback families
- future execution-flow additions can reuse the shared rendering options
  without adding another generic layer or broader redesign

## Rules

- Reuse the shared execution-step rendering helper for new restore, update, or
  rollback family step lists instead of copying per-command loops.
- Keep step item mapping aligned to the existing `result` contract types unless
  an explicit contract change is intended.
- Treat this consolidation as an internal refactor: external JSON, text, and
  exit semantics must stay stable unless a separate corrective change is
  required.
