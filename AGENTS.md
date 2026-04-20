# AGENTS

## Mission
- Keep the retained core smaller, stricter, and more reliable.
- Reliability first. Compactness next. Clarity after that.
- The retained product is exactly `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.

## Authority
- Repository authority for the retained product is `AGENTS.md` -> Go code under `cmd/espops/` and `internal/` -> `Makefile` -> `.github/workflows/ai-governance.yml`.
- `AI/*` is transitional governance machinery. It is not part of the default CI path and does not define the runtime product contract.
- `README.md` and `CONTRIBUTING.md` are practical docs. If they drift from the retained Go product, fix them.
- If an archived doc conflicts with `AGENTS.md` or the retained Go product, ignore the archived doc.

## Defaults
- Block unless clearly safe.
- Fail closed.
- Delete drift instead of wrapping drift.
- Shell is thin execution only.
- No success before explicit post-check or health-check.

## Rules
- `internal/` owns retained-core behavior.
- `cmd/espops/` owns CLI entrypoints and command surface only.
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
- Run `make ci` before claiming repository health after product, workflow, or contributor-path changes.
- Use shell or AI checks only when intentionally working on those transitional paths.
