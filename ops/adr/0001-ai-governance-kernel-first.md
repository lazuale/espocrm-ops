# AI-First Governance Becomes Repository Authority

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-17

## Context

The repository moved from prose-led governance to machine-enforced AI governance while Go became the contract core and shell remained transitional.

## Decision

The repository authority set became:
- `AGENTS.md`
- `AI/spec/*`
- generated enforcement under `AI/compiled/*`
- `Makefile`
- `.github/workflows/ai-governance.yml`

Everything else became implementation detail or historical memory. Go kept the canonical JSON and exit-code contract. Shell JSON stayed passthrough or explicitly non-canonical.

## Consequences

- Bootstrap now starts from the AI corpus instead of legacy prose.
- CI and generators enforce governance drift.
- Contract changes require refreshed baselines and ADR review.

## Rules

- no new shell-owned destructive plan, selection, policy, or stable report logic;
- no prose-derived or shell-text-derived machine contract;
- no hidden fallback or silent noop infrastructure;
- no synthetic wrappers or generic packages without ownership necessity.
