# Canonical Operation History Listing Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0016, ADR 0017, ADR 0018, and ADR 0019, operators could:

- execute canonical Go-owned `update` and `rollback` runs
- inspect dry-run plans before execution
- inspect canonical operation reports after execution
- recover failed or blocked runs through Go-owned retry and resume semantics

One operator workflow gap still remained:

- the journal had canonical Go-owned data, but the only practical inspection entry points were `last-operation` and `show-operation`
- operators still lacked a fast Go-owned way to list recent runs, filter to the interesting subset, and choose the operation id worth drilling into next
- leaving history listing as raw journal-entry output would keep operator triage tied to low-level fields instead of the canonical report model already owned in Go

That gap made routine operations slower than they needed to be and created pressure to reintroduce shell-side filtering or ad hoc log reading.

## Decision

Extend the existing Go-owned `history` surface into a canonical operation-list view.

The canonical history flow now works as follows:

- `history` remains the single list surface and projects raw journal entries through the existing Go-owned report/explain model
- each listed item is a compact operation summary with canonical status, scope, recovery-run metadata, target summary where applicable, warning count, failure attribution, and a short human-readable summary
- list filtering stays in Go and supports the highest-value operator filters on command, status, scope, recovery-run state, target prefix, timestamps, and count
- recent-first ordering stays canonical because the journal reader remains the source of order authority
- `show-operation` and `last-operation` remain the drill-down surfaces for full explanation rather than duplicating list logic elsewhere

## Consequences

- operators can triage recent operations quickly without reading raw journal JSON or shell logs first
- the canonical JSON contract changes in this pass because `history` items are now operation summaries instead of raw journal entries
- list text and JSON are both produced from the same Go-owned summary projection, so filtering and explanation do not drift into duplicate paths

## Rules

- Do not add a second shell-owned history or listing path.
- Keep history summaries derived from the Go-owned journal/report model rather than re-parsing logs.
- Extend the Go history summary projection when new canonical operation data becomes useful for operator triage.
