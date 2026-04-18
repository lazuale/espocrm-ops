# Canonical Operation Prune Retention Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0018, ADR 0019, and ADR 0020, operators could:

- inspect canonical operation reports with `show-operation` and `last-operation`
- list recent operation summaries with `history`
- reason about retry and resume decisions from the journaled Go-owned report model

One operator workflow gap still remained:

- the journal had no canonical Go-owned retention surface for pruning operation history after review
- leaving prune and retention decisions outside Go would create pressure for shell-owned cleanup logic and duplicate history-selection behavior
- a minimal delete-count output would not be enough for safe operator use because prune decisions need an explicit preview of what stays, what is removed, and what is protected

That gap left retained operation history harder to manage than it needed to be and risked reintroducing non-canonical cleanup paths.

## Decision

Extend the existing Go-owned journal surface with a canonical `journal-prune` retention command.

The canonical prune flow now works as follows:

- `journal-prune` remains the single prune command surface for journaled operations
- prune policy stays Go-owned and supports the highest-value controls on `keep-days`, `keep-last`, and `dry-run`
- prune output projects journal entries through the existing Go-owned operation summary/report model so each decision can say whether an operation is kept, removed, or protected
- the latest operation is protected explicitly and reported as such instead of being dropped silently by age or count retention
- JSON and human-readable text both come from the same Go-owned retention decision model rather than parallel shell logic

## Consequences

- operators can preview and apply journal retention safely without reading raw files or maintaining shell-side prune heuristics
- the canonical JSON contract changes in this pass because `journal-prune` details and items now expose retention decisions, protected counts, and the latest protected operation id, and a governed prune fixture is added
- empty journal day directories can still be removed after file pruning, but directory cleanup remains secondary to the canonical per-operation decision report

## Rules

- Do not add a second shell-owned prune or retention path for operation history.
- Keep prune decisions derived from the canonical Go journal/report model instead of shell file selection or prose rules.
- Make retention safety explicit in the reported decision set whenever the latest operation is protected from removal.
