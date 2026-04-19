# Maintenance Becomes The Canonical Housekeeping Run

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already had Go-owned operator surfaces for dashboard summary,
detailed status, operation history, backup inspection, support bundles, and
the major operational execution flows. Operators still lacked one canonical
Go-owned maintenance entrypoint for safe routine cleanup:

- journal pruning remained a focused single-purpose flow instead of a broader
  housekeeping run
- report, support-bundle, and restore-drill retention cleanup had no single
  top-level operator command that previewed and applied cleanup together
- operators needed one clear command for routine retention enforcement without
  reopening shell-owned orchestration or adding another ambiguous cleanup noun

## Decision

Introduce `espops maintenance` as the canonical Go-owned housekeeping and
maintenance run.

`maintenance` is now the authoritative owner for:

- safe preview-versus-apply housekeeping mode selection
- contour env resolution and maintenance preflight for routine cleanup
- journal retention execution through the Go-owned journal prune primitive
- report retention cleanup
- support-bundle retention cleanup
- restore-drill env, temporary storage, and backup-root cleanup
- explicit keep-versus-protect-versus-remove reporting
- explicit included-versus-omitted-versus-failed section reporting
- canonical text, JSON, error, and exit-code behavior for routine operator
  housekeeping

Do not add a second sibling top-level cleanup noun such as `cleanup` or
`housekeeping` while `maintenance` remains the canonical maintenance path.

## Consequences

- operators gain one clear Go-owned housekeeping entrypoint for safe routine
  cleanup and retention enforcement
- the governed CLI contract changes because `maintenance` joins the public Go
  surface with a stable JSON result and human-readable preview/apply output
- `journal-prune` remains a focused lower-level tool, while `maintenance`
  becomes the higher-level routine cleanup pass that coordinates the key
  cleanup domains
- no shell wrapper is required for the canonical maintenance flow in this pass

## Rules

- Treat `espops maintenance` as the single canonical Go-owned housekeeping and
  maintenance run.
- Do not introduce a duplicate top-level `cleanup` or `housekeeping` command
  while `maintenance` is the canonical path.
- Keep housekeeping orchestration, preview/apply logic, JSON output, and
  exit-code behavior Go-owned.
- Reuse the Go journal, report, support, and restore-drill primitives instead
  of rebuilding cleanup controllers in shell.
