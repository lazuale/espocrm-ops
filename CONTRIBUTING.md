# Contributing

This file is onboarding only. Repository authority lives in `AGENTS.md` and `AI/spec/*`.

It is a reference pointer, not an authority surface.

Start with [AGENTS.md](AGENTS.md).

Canonical root path: `AGENTS.md` -> `AI/spec/*` -> required `AI/compiled/*` -> `Makefile` -> `.github/workflows/ai-governance.yml`.

Validate changes with `make ai-refresh`, `make ai-check`, and `make ci`.

Do not introduce a second governance surface.
