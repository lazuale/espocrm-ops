# Rollback Auto-Selection Moves to the Go Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The rollback path was still carrying shell-owned backup-set auto-selection even after backup verification became a canonical Go CLI contract. Shell could still choose the latest rollback set on its own, and the optional Go path depended on shell-side JSON extraction logic.

That left a destructive operational path with two selection stacks: shell heuristics and Go verification. It also left rollback selection vulnerable to shell drift instead of forcing it through the same contract surface that already governs `verify-backup`.

## Decision

Rollback auto-selection now delegates to the Go `verify-backup` JSON contract as the only authoritative selection path.

Shell keeps the outer rollback wrapper responsibilities that are still transitional: contour/env handling, operator approvals, plan/report files, snapshot orchestration, and post-restore stack startup. Manual rollback pairs remain explicitly operator-supplied shell inputs, but automatic selection is no longer owned by shell heuristics or shell fallback logic.

## Consequences

- Rollback no longer derives its own latest valid backup set from shell artifact traversal.
- The selected manifest, DB backup, and files backup now come from the canonical Go verification contract.
- Shell is reduced to consuming Go-selected artifact paths and writing wrapper-level rollback reports.
- Remaining shell-owned selection debt still exists in other flows such as `restore-drill` and `migrate-backup`.

## Rules

- rollback auto-selection must flow through `espops --json verify-backup --backup-root`;
- rollback must not keep a shell fallback selector for the latest valid backup set;
- shell may derive reporting metadata from Go-selected artifact paths, but must not own the selection semantics;
- future consolidation should delete similar shell selection fallback in other destructive flows rather than duplicate it.
