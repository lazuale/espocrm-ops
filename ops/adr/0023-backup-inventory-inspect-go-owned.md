# Canonical Backup Inventory And Inspect Move Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0003, ADR 0005, ADR 0006, ADR 0010, ADR 0016, and ADR 0017, the repository already had important Go-owned backup-adjacent capabilities:

- canonical backup execution through the shared Go backup usecase
- canonical latest-valid selection for rollback through Go verification
- Go-owned audit and catalog reporting surfaces
- Go-owned rollback and update reports that already described backup artifacts in the operation journal

One operator gap still remained:

- the existing backup catalog listed grouped files, but it did not expose one canonical backup-set identity for follow-up inspection
- origin/source attribution still depended on operators inferring intent from paths and filenames instead of the Go-owned journal/report model
- inspection of one selected backup set still had no dedicated canonical Go surface
- partial or invalid manifests did not become an explicit structured backup-state report

That left backup discovery more filesystem-shaped than operator-shaped even though the repository already had the Go metadata needed to do better.

## Decision

Keep `backup-catalog` as the single canonical Go-owned backup inventory command and add `show-backup` as the canonical Go-owned inspect command for one selected backup set.

The backup inventory/inspect path now works as follows:

- `backup-catalog` remains the one inventory surface instead of introducing a parallel list command
- each backup set now has a canonical Go-owned `id` derived from the grouped backup identity
- backup grouping, db/files pairing, manifest validation state, readiness classification, and human/json reporting stay in Go
- manifest metadata is surfaced directly in Go inventory items instead of leaving operators to infer scope and timestamps from raw paths
- backup origin classification is derived from the Go-owned journal/report model by matching created backup artifacts to the journaled operation that produced them
- `show-backup` resolves one backup by canonical `id` and returns the same structured Go-owned summary for focused inspection

## Consequences

- operators can discover and inspect backup sets without starting from raw directory listings or ad hoc path interpretation
- the canonical JSON surface changes in this pass because `backup-catalog` items are enriched and `show-backup` is added with governed fixtures
- source attribution is available when the journal still contains the producing operation; otherwise the origin remains explicitly `unknown`
- invalid or partial manifests no longer disappear behind filesystem presence alone; they are reported as structured incomplete or corrupted inventory state

## Rules

- Do not add a second Go or shell backup inventory path beside `backup-catalog`.
- Do not move backup identity, pairing, or source classification back into shell wrappers.
- Keep backup origin attribution anchored to the Go journal/report model instead of filename heuristics whenever the journal makes that attribution determinable.
