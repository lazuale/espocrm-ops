# AGENTS

## Mission
- Keep the product smaller, stricter, and more reliable.
- Reliability first. Compactness next. Clarity after that.
- The product is exactly `doctor`, `backup`, `backup verify`, `restore`, `migrate`, and `smoke`.

## Authority
- Repository authority is `AGENTS.md` -> Go code under `cmd/espops/` and `internal/` -> `Makefile` -> `.github/workflows/ci.yml`.
- `README.md` and `CONTRIBUTING.md` are practical docs. If they drift from the Go product, fix them.
- If a non-authority doc conflicts with `AGENTS.md` or the Go product, ignore the non-authority doc.

## Defaults
- Block unless clearly safe.
- Fail closed.
- Delete drift instead of wrapping drift.
- No second runtime.
- No success before explicit post-check or health-check.

## Rules
- `internal/` owns product behavior.
- `cmd/espops/` owns the program entrypoint and command surface only.
- Keep one direct code path.
- Keep package edges obvious and local.
- No hidden alternate path.
- No auto-repair.
- No auto-normalization.
- No silent recovery.
- No implicit path switching.
- No ambiguous success.
- No shell-owned product, contract, validation, or recovery semantics.
- No guard theatre.
- No fake machine-enforcement.
- No new product surface.
- `DB_SERVICE` and `APP_SERVICES` are explicit runtime contract inputs, not inferred defaults.
- `restore`, `migrate`, and `smoke` require an explicit MariaDB root secret for database reset; no fallback to `DB_USER`.
- `restore`, `migrate`, and `smoke` require explicit `ESPO_RUNTIME_UID` and `ESPO_RUNTIME_GID`; do not guess runtime ownership from the image or current user.

## Core Flow
- Resolve input.
- Validate input.
- Verify coherence.
- Execute side effects.
- Run explicit post-check or health-check.
- Return explicit result.
- If correctness is ambiguous, block or fail.

## Testing
- Scenario proof first.
- Do not claim a reliability improvement without end-to-end evidence.
- Do not claim integration coverage without real Docker integration evidence.

## Done
- Keep authority surfaces in sync.
- Keep `README.md` and `CONTRIBUTING.md` in sync with the shipped tool.
- Run `make ci` before claiming repository health after product, workflow, or contributor-path changes.
