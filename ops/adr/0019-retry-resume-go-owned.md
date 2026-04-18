# Canonical Retry And Resume Semantics Move Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0016, ADR 0017, and ADR 0018, operators could:

- run real Go-owned `update` and `rollback` executions
- inspect Go-owned dry-run plans before execution
- inspect Go-owned operation reports after execution

One important operational gap still remained:

- failed or blocked runs still forced operators to infer by hand whether they should rerun everything, continue from a known checkpoint, or stop because continuation was unsafe
- the journal already contained canonical step outcomes, target selection, warnings, and failure attribution, but no Go-owned recovery decision model used that data
- pushing retry or resume reasoning back into shell wrappers or raw log reading would recreate the same duplicated control drift the repository is explicitly trying to delete

That gap violated the repository direction that canonical recovery behavior, machine contract, warnings, and operator explanation belong to Go.

## Decision

Extend the Go-owned `update`, `rollback`, and journal/report surfaces with canonical retry and resume semantics.

The canonical recovery path now works as follows:

- `show-operation` and `last-operation` project each real `update` or `rollback` journal entry into a Go-owned recovery evaluation that declares whether the source run is retryable, resumable, or refused, plus the recommended mode, resume checkpoint, explanation, and action
- `update` and `rollback` accept explicit recovery flags that recover a prior operation by id while keeping the public command identity canonical
- recovery decisions are derived from journaled step outcomes and journaled target/request data instead of shell heuristics or prose parsing
- retry from start is allowed only when the journal proves that rerunning the full flow is safe enough
- partial resume is allowed only from explicit checkpoints the Go usecases can explain and own directly
- unsafe or ambiguous continuation paths are refused instead of guessed silently
- recovery executions and recovery dry-runs persist their own Go-owned recovery metadata so later reports explain both what happened and how the run was launched

`update` and `rollback` remain the canonical operation owners. Recovery is an explicit mode on those command surfaces, not a parallel shell-owned controller.

## Consequences

- Operators can ask the Go journal/report stack whether a failed or blocked operation should be retried, resumed, or refused before taking action.
- The machine contract surface changes in this pass because `update`, `rollback`, `show-operation`, and related JSON fixtures now expose recovery metadata.
- Rollback recovery reuses the journaled selected target as authority when available so retries and resumes do not silently drift to a different backup set.
- Unsafe rollback continuation after files-restore failure is refused explicitly rather than framed as a success or guessed as resumable.

## Rules

- Do not add shell-owned retry, resume, checkpoint, or recovery decision logic for `update` or `rollback`.
- Keep retry and resume decisions derived from journaled Go result data and canonical step outcomes.
- Extend the Go recovery evaluator when new update or rollback phases introduce a new safe checkpoint or a new refusal case.
