# AGENTS

## Mission
- Keep the retained core smaller, stricter, and more reliable.
- Reliability first. Compactness next. Clarity after that.
- The retained product is exactly `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.

## Authority
- Repository authority is `AGENTS.md` -> `ARCHITECTURE.md` and `MICRO_MONOLITHS.md` -> Go code under `cmd/espops/` and `internal/` -> `Makefile` -> `.github/workflows/ci.yml`.
- `README.md` and `CONTRIBUTING.md` are practical docs. If they drift from the retained Go product, fix them.
- If an archived doc conflicts with `AGENTS.md` or the retained Go product, ignore the archived doc.

## Defaults
- Block unless clearly safe.
- Fail closed.
- Delete drift instead of wrapping drift.
- No second runtime.
- No success before explicit post-check or health-check.

## Rules
- `internal/` owns retained-core behavior.
- `cmd/espops/` owns the program entrypoint and command surface only.
- No implicit micro-monolith split.
- No implicit micro-monolith merge.
- No cross-monolith caller edge outside `MICRO_MONOLITHS.md`.
- No hidden fallback.
- No auto-repair.
- No auto-normalization.
- No silent recovery.
- No implicit path switching.
- No ambiguous success.
- No shell-owned product, contract, validation, or recovery semantics.
- No new product surface.

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

## Done
- Keep authority surfaces in sync.
- Keep `ARCHITECTURE.md`, `MICRO_MONOLITHS.md`, `CONTRIBUTING.md`, and compliance docs in sync.
- Run `make ci` before claiming repository health after product, workflow, or contributor-path changes.
