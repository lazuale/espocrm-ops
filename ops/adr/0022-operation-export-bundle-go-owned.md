# Canonical Operation Export Bundle Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

After ADR 0018, ADR 0019, ADR 0020, and ADR 0021, operators could:

- inspect canonical operation reports with `show-operation` and `last-operation`
- list recent operation summaries with `history`
- prune retained journal history with `journal-prune`
- reason about retry and resume decisions from the journaled Go-owned report model

One operator workflow gap still remained:

- operators had no canonical Go-owned way to export one selected operation into a durable incident-review bundle
- relying on ad hoc `show-operation --json` capture or shell-side packaging would duplicate the existing report model and invite a second export path
- raw report output alone did not say clearly which higher-value sections were present versus absent in the selected journal entry

That gap made incident review and structured handoff harder than it needed to be and created pressure for non-canonical export wrappers.

## Decision

Add a canonical Go-owned `export-operation` command that writes one structured JSON incident bundle for a selected journaled operation.

The canonical export flow now works as follows:

- `export-operation` is the single export surface for operation bundles
- export selection reuses the existing Go-owned `show-operation` lookup path instead of introducing a parallel reader stack
- the written bundle contains canonical bundle metadata, a compact operation summary, the full Go-owned operation report, journal-read stats, and explicit included versus omitted section lists
- the stdout result remains a normal Go CLI result envelope in both text and JSON, while the bundle file is the durable export artifact

## Consequences

- operators can archive or share one operation bundle without collecting arbitrary log archives for the first version
- the canonical machine surface changes in this pass because `export-operation` is added and governed JSON fixtures now cover both the command result and the written bundle payload
- missing optional journal/report data no longer blocks export; the bundle declares omitted sections explicitly instead of guessing or filling them from shell logic

## Rules

- Do not add a second shell-owned operation export or incident-bundle path.
- Keep operation bundle contents derived from the canonical Go journal/report model instead of log scraping or prose reconstruction.
- Keep export scope focused on structured operation information until a future feature proves that larger log packaging is canonically required.
