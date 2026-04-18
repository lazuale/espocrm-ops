# Checked Cleanup and Write Errors Are Repository Health Work

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

After the Go `1.26.2` and `staticcheck` remediation, the next full-gate blocker was `golangci-lint run --no-config ./...`.

The failing set was narrow but operationally relevant:

- unchecked cleanup and close errors in CLI, platform, restore, and backup paths
- unchecked lock release errors in runtime and test coverage around journal and restore flows
- one remaining ineffective assignment in lock readiness probing
- the remaining `QF1001` boolean simplifications in environment parsing

These were not cosmetic style issues. They were places where cleanup, lock release, or operator-visible output failures could be dropped silently.

## Decision

Treat the current `golangci-lint` failures as required health remediation and clear them in the smallest coherent pass.

This pass preserves behavior while making cleanup and output failures explicit:

- CLI text error and warning rendering now checks write failures
- lock, file, gzip, tar, stage, and response-body cleanup paths now check and propagate errors where the current full-gate failures required it
- repository tests now fail explicitly when deferred cleanup or lock release fails
- the remaining lock-readiness assignment and env-parser boolean forms are simplified to the lint-safe equivalents

## Consequences

- `make check-full` can progress past the `golangci-lint` stage only when cleanup and write reliability is explicit in the currently failing paths.
- Cleanup and release failures are now part of the observable error surface in affected runtime paths instead of being silently dropped.
- Test helpers that create archives or hold locks now validate deferred teardown instead of ignoring it.

## Rules

- Do not treat unchecked cleanup, release, close, or operator-facing write errors as cosmetic lint in this repository.
- When health tooling flags a bounded cleanup or release failure path, prefer explicit propagation or explicit test failure over silent ignore.
- Keep future lint-health passes narrow and tied to the active full-gate failures instead of using them as a pretext for unrelated cleanup.
