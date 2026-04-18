# espocrm-ops

This file is a dispatch note only. It is not an authority surface.

Start with [AGENTS.md](AGENTS.md), then read the relevant `AI/spec/*` files for the area you are changing.

Canonical root path: `AGENTS.md` -> `AI/spec/*` -> required `AI/compiled/*` -> `Makefile` -> `.github/workflows/ai-governance.yml`.

Run `make ai-refresh`, `make ai-check`, and `make ci`.

Do not edit `AI/compiled/*` manually. Regenerate it with `make ai-refresh`.
