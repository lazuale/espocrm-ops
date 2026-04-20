# Recovery Flows Require Coherent Input And Runtime Health

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-20

## Context

The retained product surface is now limited to backup and recovery core:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

That hard cut shifts the next risk from breadth to trustworthiness. Two
reliability gaps remained in the retained recovery paths:

- manifest-backed verification accepted some inconsistent backup-set metadata,
  which made it possible for a manifest to point at a mismatched DB/files pair
  and still proceed through recovery-adjacent paths;
- `restore` and `migrate` could report success immediately after startup
  orchestration without proving that the final running contour was actually
  healthy.

`migrate` also tolerated an invalid matching manifest under `BACKUP_ROOT` by
warning and falling back to direct archive use, which violated the no-hidden-
fallback rule for retained recovery flows.

## Decision

Strengthen the retained backup and recovery contract without expanding product
surface:

- canonical manifest-backed verification now requires a coherent backup-set
  identity across manifest name, DB backup name, and files backup name;
- `migrate` no longer warns and falls back when a matching manifest for the
  selected backup set is invalid; it blocks instead;
- `restore` success now requires post-restore runtime health validation for the
  services that should be running after the operation;
- `migrate` success now requires target-start runtime health validation for the
  started target services.

The public commands stay the same. The hardening is entirely inside the retained
Go-owned recovery flows.

## Consequences

- `backup verify` rejects more inconsistent manifest-backed backup sets before
  they can be trusted by recovery flows.
- `restore` and `migrate` no longer have ambiguous success when runtime startup
  reached `docker compose up` but final service health did not.
- Matching invalid manifests under `BACKUP_ROOT` become blocking defects instead
  of warnings for `migrate`.
- Governed JSON fixtures and compiled contract baselines must be refreshed so
  the stricter step details remain visible.

## Rules

- Do not let manifest-backed recovery proceed from a DB/files pair that does
  not resolve to one coherent backup set.
- Do not hide invalid matching manifests behind warning-and-fallback behavior in
  `migrate`.
- Do not report successful `restore` or `migrate` completion before required
  runtime health validation passes for the services that should be up.
