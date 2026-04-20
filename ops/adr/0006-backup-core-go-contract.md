# Backup Uses the Canonical Go Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The repository is intentionally narrowed to backup and recovery. Backup
creation and verification therefore need one explicit Go-owned command family
instead of hidden execution nouns or shell-owned sequencing.

The remaining contract requirements are simple:

- `backup` must own deterministic backup creation
- `backup verify` must own deterministic verification
- shell wrappers must stay thin passthroughs instead of rebuilding backup
  orchestration
- Go JSON and exit-code behavior must remain canonical for the retained backup
  flow

## Decision

Keep backup creation and verification in the canonical public Go command family:

- `espops backup` is the authoritative owner of backup creation
- `espops backup verify` is the authoritative owner of verification
- the shared Go backup usecase owns backup runtime behavior, including:

- application-service stop/start decisions for consistent snapshots
- database dump execution
- files archive creation, including Docker-helper fallback
- checksum sidecars
- manifest emission
- backup retention cleanup

`scripts/backup.sh` remains only a thin shell wrapper around the retained Go
commands. It may translate the minimal shell UX into `espops backup` or
`espops backup verify`, but it must not reintroduce controller logic.

## Consequences

- Backup creation and verification semantics are no longer split across shell
  and hidden Go entrypoints.
- The retained public product surface is simpler: one `backup` command family
  instead of a hidden executor plus adjacent backup nouns.
- Shell remains only the outer UX boundary where the user still wants a script.

## Rules

- Do not reintroduce backup implementation, selection, or verification logic
  into `scripts/backup.sh`.
- Keep backup verification on `backup verify` instead of creating sibling
  inventory or audit commands.
- Extend the shared Go backup usecase when backup behavior changes.
