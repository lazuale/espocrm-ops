# Canonical Contour Overview Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository had already moved the major operational execution paths and the
highest-value operator diagnostics into Go, but one important operator summary
surface still remained shell-controlled:

- `scripts/contour-overview.sh` still selected overview sections itself
- the shell path still called doctor, status, backup audit, and backup catalog
  scripts in sequence
- the shell path still assembled the summary text and shell-owned JSON envelope
- operators still depended on shell-collected section output instead of a
  canonical Go summary surface

That left the current contour summary outside the Go CLI contract and preserved
another parallel operator overview stack.

## Decision

Introduce a canonical public Go command, `overview`, backed by a Go-owned
overview summary usecase.

`overview` is now the authoritative owner of:

- contour section selection for the current-state overview
- doctor readiness summarization
- runtime status summarization from Go-owned env, Docker, and lock primitives
- backup summary assembly from Go-owned audit and catalog usecases
- recent operation summary assembly from the Go-owned journal history model
- explicit included-versus-omitted-versus-failed section reporting
- warning aggregation, failure attribution, and canonical text and JSON output
- canonical overview exit-code behavior

`scripts/contour-overview.sh` remains only as a thin compatibility wrapper. It
may preserve legacy shell entrypoint ergonomics such as contour parsing and
optional file-output forwarding, but it must delegate immediately to
`espops overview ...` and must not own section sequencing, summary assembly, or
JSON contract logic.

## Consequences

- the real contour overview path is now primarily Go-owned instead of
  shell-orchestrated
- the governed CLI contract changes in this pass because `overview` joins the
  public Go command surface and gains its own canonical JSON fixture
- `scripts/contour-overview.sh --json` changes classification from
  shell-generated non-canonical JSON to passthrough compatibility JSON because
  the shell wrapper no longer builds its own overview envelope
- operators can inspect readiness, runtime state, backup state, and recent
  operations from one canonical Go summary surface instead of a shell-heavy
  assembly flow

## Rules

- Do not reintroduce shell-owned contour overview sequencing or summary
  assembly.
- Keep `scripts/contour-overview.sh` as a thin compatibility wrapper only.
- Treat `espops overview` as the canonical machine contract for contour
  overview, including JSON and exit-code behavior.
- Extend the Go overview usecase when new summary sections or contour-state
  signals become useful instead of rebuilding controller logic in shell.
