# AGENTS

`espocrm-ops` is AI-first and governance-first.

Bootstrap order:
1. `AGENTS.md`
2. `AI/spec/*`
3. required generated enforcement artifacts under `AI/compiled/`
   `POLICY.json`, `CONTRACT_BASELINE.json`, `JSON_CONTRACT_BASELINE/`, `SHELL_DEBT_BASELINE.json`
4. `Makefile`
5. `.github/workflows/ai-governance.yml`

Repository truth:
- Go is the strategic core.
- Shell is transitional legacy.
- Canonical machine contract belongs to Go CLI JSON and exit-code surfaces.
- Shell JSON is either thin passthrough to Go or explicitly non-canonical shell data.
- Delete drift instead of wrapping drift.
- Hidden fallback, silent noop, wrapper creep, helper explosion, and prose-derived contract are defects.

Authority:
- Active authority lives only in `AGENTS.md`, `AI/spec/*`, the required generated enforcement artifacts under `AI/compiled/`, `Makefile`, and `.github/workflows/ai-governance.yml`.
- `AI/compiled/*` is generated enforcement state. Do not edit it manually. Regenerate it with `make ai-refresh`.
- Archived human docs are non-authoritative memory only. They do not override the AI corpus.
- If an archived doc conflicts with `AGENTS.md`, `AI/spec/*`, or generated enforcement artifacts, ignore the archived doc.

Builder rules:
- Do not add new shell-owned destructive plan, selection, policy, or stable report logic.
- Do not add new shell `--json` surfaces without classifying them as explicit passthrough wrappers or explicit non-canonical shell data.
- Do not add new generic packages or layers named `common`, `utils`, `helpers`, `services`, `builders`, `factories`, `managers`, `shared`, `facade`, `wrapper`, or `core`.
- Do not parse JSON or machine errors with `awk`, `grep`, or prose matching as stable contract logic.
- Do not introduce hidden fallback behavior.
- Do not leave the AI specs, compiled policy/baselines, workflow, and Makefile out of sync.
- Do not treat archived docs as current truth, even when they mention older workflows, generated files, or historical rules.

Standard cycle:
- Edit code or `AI/spec/*`.
- Run `make ai-refresh`.
- Run `make ai-check`.
- Run `make ci`.
- Update `ops/adr/*` only when the AI rules require an ADR for an architecture or contract change.
