# Product Hard-Cut To Backup And Recovery Core

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-20

## Context

The repository had grown beyond the retained backup and recovery core. That
breadth created drift, duplicate command selection paths, extra help and
registration noise, and a larger governed contract surface than the retained
product actually needs.

## Decision

Hard-cut the product to backup and recovery core.

The retained operator-facing command set is:

- `doctor`
- `backup`
- `backup verify`
- `restore`
- `migrate`

Everything else is removed from root registration, help output, shell
dispatchers, tests, goldens, specs, and ADR memory unless validation proves it
is directly required for the retained flows.

## Consequences

- Compatibility break is intentional in this pass.
- The governed surface shrinks to readiness, backup creation/verification,
  restore, and migrate.
- Removed commands do not keep aliases, deprecation shims, or compatibility
  wrappers.
- Historical ADR memory is pruned so the active tree no longer carries removed
  product nouns as if they were current.

## Rules

- If a surface is not directly required for `doctor`, `backup`, `backup
  verify`, `restore`, or `migrate`, remove it.
- Do not reintroduce removed commands indirectly through wrappers, aliases, or
  hidden entrypoints.
- Keep canonical JSON and exit-code behavior only for the retained commands.
