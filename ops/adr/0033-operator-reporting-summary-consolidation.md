# Operator Reporting Shares Section Summary Internals

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The Go-owned operator layer already had stable command surfaces for overview,
status reporting, maintenance, support bundles, operation export/history, and
the major restore/rollback/migrate flows. Those commands were working, but the
internal reporting path had started to repeat the same assembly logic in
several places:

- section included/omitted/failed list aggregation was duplicated across
  overview, status-report, maintenance, and support-bundle
- warning deduplication was repeated across multiple operator usecases
- CLI result details rebuilt the same section summary shapes independently
- `internal/contract/result/details.go` repeated the same item and section
  fields across operator-facing result types
- operator golden normalization helpers repeated the same recursive JSON
  replacement walk

That duplication made future operator work more likely to drift across
multiple files for one reporting change.

## Decision

Consolidate the operator reporting internals around shared section-summary
primitives while keeping the external command contracts stable.

This pass standardizes the internal shape by:

- introducing shared section summary and section item primitives in
  `internal/contract/result`
- moving operator-oriented result detail shapes out of the larger
  `details.go` catch-all into a focused operator details file
- adding a small shared section collector under `internal/usecase` for
  included/omitted/failed and warning aggregation
- reusing that collector across overview, status-report, maintenance, and
  support-bundle, while also reusing the same warning dedupe path in restore,
  rollback, and migrate flows
- consolidating the operator CLI summary rendering block and operator JSON test
  normalization recursion

The public JSON fields, text output semantics, and exit behavior stay
behaviorally stable in this refactor pass.

## Consequences

- future operator commands can extend shared section summaries without copying
  list-count assembly into each surface
- operator warning handling is less likely to drift across restore, rollback,
  migrate, maintenance, and dashboard/reporting paths
- `internal/contract/result/details.go` is less of a single dumping ground for
  unrelated operator result shapes
- the refactor improves internal extensibility without introducing a new
  product surface or changing the governed machine contract

## Rules

- Reuse the shared operator section-summary primitives for new
  included/omitted/failed operator reporting surfaces.
- Keep operator warning deduplication on the shared usecase reporting path
  instead of reintroducing per-command copies.
- Do not split stable external command contracts from their Go-owned operator
  implementation while this shared reporting shape remains canonical.
- Treat this consolidation as an internal refactor: external JSON and exit
  semantics must stay stable unless a separate contract change is explicitly
  intended.
