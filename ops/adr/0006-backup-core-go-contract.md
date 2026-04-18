# Backup Core Uses a Hidden Go Contract

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After ADR 0005, the `update` boundary for the pre-update recovery point moved behind `update-backup`, but the actual backup implementation still lived in `scripts/backup.sh`.

That left the highest-value remaining core gap in place:

- standalone backup execution semantics were still shell-owned
- `update-backup` still depended on shell-owned backup implementation
- backup artifact creation, checksum sidecars, manifests, service stop/start handling, and retention cleanup were not yet owned by the canonical Go execution stack

## Decision

Introduce a hidden Go command, `backup-exec`, and a shared Go backup execution usecase as the authoritative owner of backup runtime behavior.

The shared Go backup path now owns:

- application-service stop/start decisions for consistent snapshots
- database dump execution
- files archive creation, including Docker-helper fallback
- checksum sidecars
- text and JSON manifest emission
- backup retention cleanup

`scripts/backup.sh` is reduced to env loading, locking, context printing, and a single `run_espops backup-exec ...` delegation.

`update-backup` now uses the same Go backup execution usecase directly instead of bouncing through `scripts/backup.sh`.

## Consequences

- Backup implementation semantics are no longer shell-owned at the core execution layer.
- Standalone backup and `update-backup` now share one authoritative Go implementation.
- Shell remains only the outer boundary for env loading, locking, and operator-facing entrypoints.
- The machine contract surface changes in this pass because `backup-exec` is added and `update-backup` artifacts/details now report the Go-created recovery-point artifacts instead of shell-wrapper metadata.

## Rules

- Do not reintroduce backup implementation logic into `scripts/backup.sh`.
- Future backup consolidation should extend the shared Go backup usecase rather than adding parallel shell execution paths.
- `update-backup` must continue to use the shared Go backup execution path instead of invoking `backup.sh`.
