# Scheduled Maintenance Stays On The Canonical Go Command

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`,
`AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and
`.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already had a Go-owned `espops maintenance` command for routine
housekeeping and retention cleanup. Operators also need to run maintenance from
cron, systemd timers, and other unattended automation without falling back to
shell orchestration or parsing ad hoc text output.

That unattended path must stay canonical and Go-owned:

- scheduled preview and apply behavior should live on `maintenance`, not on a
  sibling `maintenance-run` or shell wrapper
- unattended apply must refuse destructive cleanup unless the operator
  explicitly opts in
- automation needs a stable JSON summary that clearly distinguishes nothing to
  do, preview candidates, successful removals, blocked runs, and partial
  failure

## Decision

Extend `espops maintenance` with explicit unattended execution semantics.

`maintenance` now owns the canonical scheduled/non-interactive contract:

- `--unattended` marks the run as scheduler-safe and non-interactive
- unattended preview remains safe by default
- unattended apply requires explicit `--allow-unattended-apply`
- the JSON contract exposes explicit `mode`, `unattended`, `outcome`, and
  aggregate checked/candidate/kept/protected/removed/failed item counts
- preview-versus-apply semantics remain on one Go-owned command path, with no
  new shell scheduler controller and no duplicate top-level automation command

## Consequences

- cron/systemd automation can call `espops maintenance` directly and rely on a
  stable machine-readable outcome
- destructive unattended cleanup is refused predictably unless explicitly
  allowed
- operators keep one canonical maintenance noun while gaining clear scheduled
  semantics instead of another cleanup surface
- machine baselines change because the governed maintenance JSON contract gains
  explicit unattended automation fields

## Rules

- Keep unattended and scheduled maintenance semantics on `espops maintenance`.
- Do not add a sibling top-level scheduled-maintenance command while
  `maintenance` remains canonical.
- Do not move unattended preview/apply orchestration into shell wrappers.
- Keep JSON outcomes and exit behavior explicit enough for cron/systemd
  automation without parsing human prose.
