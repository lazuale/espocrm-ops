# Canonical Operation Report Explain Surface Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0016 and ADR 0017, the real `update` and `rollback` execution paths were Go-owned, and their live JSON/text outputs already contained step-by-step status, warnings, and failure attribution.

Operators still lacked one canonical way to explain a completed operation after the fact:

- the journal stored timing, details, artifacts, warnings, and canonical error codes, but it did not persist the original result message or per-step result items
- `show-operation` and `last-operation` could identify an operation, but they could not explain the real `update` or `rollback` outcome without sending operators back to raw logs
- adding a second shell-owned or prose-derived report path would have recreated the same drift the repository is explicitly trying to delete

That gap violated the repository direction that operator-facing result reporting, machine-readable explanation, warnings, and failure attribution belong to the Go command/usecase/contract stack.

## Decision

Extend the Go-owned journal and journal-reading commands so completed `update` and `rollback` operations can be explained canonically after execution.

The canonical explain/report path now works as follows:

- journaled Go commands persist the original result message and structured step items beside existing timing, details, artifacts, warnings, and error metadata
- `show-operation` and `last-operation` project journal entries into a Go-owned operation report that exposes scope, selected rollback target data, normalized step outcomes, warnings, and failure attribution in both JSON and human-readable text
- downstream execution steps that were previously stored as `not_run` in execution results are normalized to `blocked` in the explain/report view so the post-run report aligns with operator-facing troubleshooting language
- the report continues to derive its machine-readable detail from the same Go-owned result payload instead of inventing a second reporting schema in shell

`history` remains the journal listing surface, while `show-operation` and `last-operation` are the canonical read paths for explaining one operation outcome.

## Consequences

- Operators can inspect what happened during a real `update` or `rollback` run without starting from raw logs.
- The machine contract surface changes in this pass because the journal entry payload grows and `show-operation` now has a governed JSON fixture for the operation report view.
- The report stack stays Go-owned and journal-derived instead of creating a parallel explanation path in shell or archived prose.

## Rules

- Do not add shell-owned operation explanation or stable report logic for `update` or `rollback`.
- Keep post-run explanation derived from the canonical Go result payload written to the journal.
- Extend the Go report projection when new update or rollback phases, warnings, or failure cases need to be operator-visible after execution.
