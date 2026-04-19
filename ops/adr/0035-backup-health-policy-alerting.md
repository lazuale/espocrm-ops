# Backup Health Adds Freshness and Policy Alerting

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already had Go-owned backup execution, verification, inventory,
inspection, audit, migration, restore, rollback, and operator reporting.

That still left one operator gap: there was no single canonical Go surface
that turned backup facts into an explicit operational posture verdict for
humans and automation. Operators could inspect backup sets and run audits, but
they still had to infer whether backup posture was healthy enough to trust,
merely degraded, or actively blocked.

The highest-value signals were already available in Go:

- latest observed backup-set readiness from `backup-catalog`
- latest restore-ready backup selection from the same canonical inventory path
- manifest and checksum posture from the catalog/audit inspection stack
- origin and journal context from the Go-owned operation journal model

What was missing was one concise policy result that assembled those facts into
explicit verdicts, breaches, warnings, and operator guidance.

## Decision

Add a canonical public Go command, `backup-health`, backed by a Go-owned
backup health usecase.

`backup-health` is the single authoritative freshness and policy alert surface
for backup posture:

- it evaluates the newest observed backup set and the newest restore-ready
  backup set from the Go-owned backup catalog path
- it reports one explicit verdict: `healthy`, `degraded`, or `blocked`
- it reports explicit warning and breach alerts with concrete next actions
- it keeps one stable JSON result for automation and one concise text view for
  operators
- it treats `blocked` posture as a validation failure with a structured result
  payload instead of an ambiguous success

The command defaults checksum verification on so a fully healthy verdict is
grounded in verified backup posture rather than unverified sidecar presence.

## Consequences

- operators and automation now have one canonical Go-owned backup posture
  verdict instead of reconstructing policy state from catalog/audit details
- future overview or health-summary features can consume the same backup
  health usecase instead of inventing another alerting path
- the machine contract expands with a new governed `backup-health` JSON result
  surface and a new public CLI command

## Rules

- Use `backup-health` as the canonical backup freshness and policy alert
  surface instead of adding parallel shell or Go health commands.
- Keep backup posture verdicts explicit: `healthy`, `degraded`, or `blocked`.
- Treat `blocked` backup posture as a non-zero validation outcome while still
  returning structured JSON details for automation.
- Keep lower-level backup facts owned by the existing Go backup inventory,
  inspect, and verification paths; `backup-health` should assemble those facts
  rather than duplicating a second backup discovery stack.
