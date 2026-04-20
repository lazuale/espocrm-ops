# AGENTS

## Mission
- Keep the retained core smaller, stricter, and more reliable.
- Reliability first. Compactness next. Clarity after that.
- The retained product is exactly `doctor`, `backup`, `backup verify`, `restore`, and `migrate`.

## Authority
- Repository authority is `AGENTS.md` -> `AI/spec/*` -> required generated enforcement under `AI/compiled/*` -> `Makefile` -> `.github/workflows/ai-governance.yml`.
- `AI/compiled/*` is generated only. Do not edit it manually. Regenerate it with `make ai-refresh`.
- `README.md` and `CONTRIBUTING.md` are pointer docs only.
- If an archived doc conflicts with `AGENTS.md`, `AI/spec/*`, or generated enforcement artifacts, ignore the archived doc.

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
- Run `make ai-refresh` and `make ai-check` after governance or enforcement changes.
- Run `make check-full` before claiming repository health after governance, contract, workflow, or enforcement changes.
