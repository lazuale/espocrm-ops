# Canonical Support-Bundle Generation Moves Into Go

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-19

## Context

The repository already moved the main operational execution paths into Go, but
`scripts/support-bundle.sh` still owned the real support-bundle controller flow.

That shell path still selected bundle content, collected runtime artifacts,
invoked doctor and backup reporting, copied manifests, built the archive, and
applied retention cleanup.

This left the canonical operator support-bundle behavior outside the Go CLI
contract and outside the Go-owned JSON and exit-code surface.

## Decision

Introduce a canonical public Go command, `support-bundle`, backed by a Go-owned
support-bundle generation usecase.

`support-bundle` is now the authoritative owner of:

- contour preflight, env resolution, runtime directory preparation, and shared
  operation locking
- bundle content selection and sequencing
- redacted env capture and attempted Docker and Compose capture
- inclusion of doctor, recent operation history, backup catalog, and latest
  manifest artifacts
- explicit included-versus-omitted section reporting
- warning emission, failure attribution, archive creation, retention cleanup,
  and canonical JSON/text output

`scripts/support-bundle.sh` remains only as a thin compatibility wrapper. It
may keep the legacy shell entrypoint shape, but it must delegate immediately to
`espops support-bundle ...` and must not own substantive bundle sequencing,
collection, or reporting logic.

## Consequences

- The real support-bundle path is now primarily Go-owned instead of
  shell-orchestrated.
- The contract surface changes in this pass because the public
  `support-bundle` command joins the governed CLI surface and gains its own JSON
  fixture and baseline entry.
- The shell boundary shrinks to argument forwarding, contour parsing, and
  env-file passthrough for compatibility callers.
- Support-bundle success can still include explicit omissions and warnings for
  runtime data that could not be collected, but those omissions are now exposed
  through the Go contract instead of ad hoc shell behavior.

## Rules

- Do not reintroduce shell-owned support-bundle sequencing, archive assembly, or
  report selection.
- Keep `scripts/support-bundle.sh` as a thin compatibility wrapper only.
- Treat `espops support-bundle` as the canonical machine contract for support
  bundle generation, including JSON and exit-code behavior.
- Extend the Go `support-bundle` flow when new bundle sections or troubleshooting
  artifacts are needed instead of rebuilding controller logic in shell.
