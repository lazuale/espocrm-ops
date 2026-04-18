# Bootstrap Surface and CI Hygiene Cleanup

Historical ADR. Non-authoritative. Repository authority lives in `AGENTS.md`, `AI/spec/*`, generated enforcement under `AI/compiled/*`, `Makefile`, and `.github/workflows/ai-governance.yml`.

## Status

Accepted

## Date

2026-04-18

## Context

The repository was development-ready in policy terms but still carried unnecessary root-level operational noise and a few avoidable CI hygiene gaps.

Committed env templates lived at the repository root even though they were operational examples rather than authority surfaces. Temporary smoke-test and restore-drill env files also defaulted into the repository root, which made preserved artifacts and failed cleanup paths clutter the bootstrap surface. The merge-gate workflow still used an older `actions/checkout` major and lacked run concurrency controls and a job timeout.

## Decision

The repository root stays centered on authority and execution surfaces. Committed env templates move under `ops/env/`, and temporary smoke-test and restore-drill env files move under `.cache/env/`.

The merge gate stays strict but gets safer operational defaults: `actions/checkout@v6`, disabled credential persistence, workflow concurrency cancellation, a job timeout, and noninteractive `apt-get` installation for `shellcheck`.

Validation and regression guards are updated so these repo-shape decisions remain enforced.

## Consequences

- Root-level clutter is reduced without changing the canonical authority set.
- Smoke-test and restore-drill preserved artifacts no longer compete with repository entrypoints.
- CI avoids duplicate in-flight runs on the same ref and uses a current checkout action surface.
- The env-template move requires all scripts and tests to read examples from `ops/env/`.

## Rules

- keep committed env templates outside the repository root;
- keep temporary generated env files outside the repository root;
- keep CI strict while preferring current, low-friction workflow defaults over stale runner behavior;
- enforce repo-shape cleanup with code and tests, not with prose alone.
