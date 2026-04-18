# Inherited Shell Context For Nested Backup Calls

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After the Go `1.26.2`, `staticcheck`, and `golangci-lint` remediation, the next full-health investigation retried `govulncheck` and then continued the remaining local checks to find the next repository-side blocker.

`govulncheck` was still externally blocked by vuln-database network timeouts, so the remaining full-health sequence was continued manually. The next repository-side blocker was `make regression`.

The regression suite stalled in a real stateful path:

- `restore-files.sh` acquired the shared operation lock and the maintenance lock
- the pre-restore emergency snapshot then invoked `backup.sh`
- `backup.sh` still treated itself as a top-level entrypoint and re-entered Go `run-operation`
- Go `run-operation` tried to acquire the shared operation lock again and waited forever on the lock already held by the parent restore flow

The same nested-backup pattern also existed in other shell-owned operations that already hold inherited runtime context, including restore-db, rollback, and smoke-test flows.

## Decision

When a shell-owned parent flow has already resolved the environment and already owns the inherited operation and maintenance context, nested calls to `backup.sh` must run in inherited shell-exec mode instead of re-entering Go `run-operation`.

This pass keeps the standalone `backup.sh` entrypoint behavior unchanged while fixing the nested lock-recursion case:

- nested backup calls from restore-files, restore-db, rollback, and smoke-test now export `ESPO_SHELL_EXEC_CONTEXT=1` for the child `backup.sh`
- `backup.sh` therefore goes straight to the canonical Go `backup-exec` path instead of trying to reacquire the shared operation lock through `run-operation`
- a focused shell regression now verifies that inherited shell context reaches `backup-exec` directly and does not invoke `run-operation`

## Consequences

- `make regression` can exercise snapshot-bearing restore and rollback flows without hanging on self-recursive operation-lock acquisition.
- The nested backup path still uses the Go-owned backup execution contract; only the redundant outer Go preflight wrapper is skipped when the parent already owns that context.
- Standalone `backup.sh` continues to route through Go `run-operation` as before.

## Rules

- Do not re-enter Go `run-operation` from a shell child when the parent shell flow already owns the shared operation lock and maintenance lock for the same operation context.
- Future nested backup invocations from locked shell flows must reuse inherited shell-exec context instead of introducing a second operation-lock acquisition path.
