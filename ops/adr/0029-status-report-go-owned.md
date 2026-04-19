# Canonical Status Report Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository had already moved the high-value operational execution paths,
overview summary, and support diagnostics into Go, but the canonical detailed
status report still remained shell-controlled:

- `scripts/status-report.sh` still resolved env context, storage paths, service
  state, lock state, and latest artifacts itself
- the shell path still assembled the operator-facing text report and the
  shell-owned JSON envelope
- operators still depended on shell sequencing for the authoritative detailed
  contour/runtime report

That left another core operator inspection surface outside the Go CLI contract
and preserved a shell-heavy controller for current contour state.

## Decision

Introduce a canonical public Go command, `status-report`, backed by a Go-owned
status-report usecase.

`status-report` is now the authoritative owner of:

- detailed status section selection for the current contour
- resolved env and storage context reporting
- doctor readiness summarization for the selected contour
- runtime service and lock-state collection from Go-owned env, Docker, and lock
  primitives
- latest operation summary collection from the Go-owned journal history model
- latest backup, report, and support artifact summary collection from Go-owned
  backup and filesystem inspection logic
- explicit included-versus-omitted-versus-failed section reporting
- warning aggregation, failure attribution, and canonical text and JSON output
- canonical status-report exit-code behavior

`scripts/status-report.sh` remains only as a thin compatibility wrapper. It may
preserve legacy shell entrypoint ergonomics such as contour parsing and optional
file-output forwarding, but it must delegate immediately to
`espops status-report ...` and must not own status collection sequencing,
summary assembly, or JSON contract logic.

## Consequences

- the real detailed status-report path is now primarily Go-owned instead of
  shell-orchestrated
- the governed CLI contract changes in this pass because `status-report` joins
  the public Go command surface and gains its own canonical JSON fixture
- `scripts/status-report.sh --json` changes classification from shell-generated
  non-canonical JSON to passthrough compatibility JSON because the shell
  wrapper no longer builds its own status envelope
- operators can inspect resolved contour context, runtime state, latest
  operation state, and latest artifact state from one canonical Go report
  surface instead of a shell-heavy assembly flow

## Rules

- Do not reintroduce shell-owned status-report sequencing or summary assembly.
- Keep `scripts/status-report.sh` as a thin compatibility wrapper only.
- Treat `espops status-report` as the canonical machine contract for detailed
  contour status, including JSON and exit-code behavior.
- Extend the Go status-report usecase when new contour-state signals become
  useful instead of rebuilding controller logic in shell.
