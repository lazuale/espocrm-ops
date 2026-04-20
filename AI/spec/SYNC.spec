{
  "name": "sync",
  "version": 1,
  "readme_required_snippets": [
    "[AGENTS.md](AGENTS.md)",
    "`AI/spec/*`",
    "`make ai-refresh`",
    "`make ai-check`",
    "`make ci`",
    "Do not edit `AI/compiled/*` manually. Regenerate it with `make ai-refresh`."
  ],
  "contributing_required_snippets": [
    "[AGENTS.md](AGENTS.md)",
    "`AI/spec/*`",
    "This file is onboarding only. Repository authority lives in `AGENTS.md` and `AI/spec/*`.",
    "`make ai-refresh`",
    "`make ai-check`",
    "`make ci`"
  ],
  "contributing_forbidden_snippets": [
    "## Required Documents Before Architectural Changes",
    "If the PR changes an architectural boundary, see the detailed regulation",
    "Archived human docs"
  ],
  "agents_required_snippets": [
    "Repository authority is `AGENTS.md` -> `AI/spec/*` -> required generated enforcement under `AI/compiled/*` -> `Makefile` -> `.github/workflows/ai-governance.yml`.",
    "`AI/compiled/*` is generated only. Do not edit it manually. Regenerate it with `make ai-refresh`.",
    "Shell is thin execution only.",
    "If an archived doc conflicts with `AGENTS.md`, `AI/spec/*`, or generated enforcement artifacts, ignore the archived doc."
  ],
  "changed_file_rules": [
    {
      "trigger_prefixes": [
        "AGENTS.md",
        "AI/spec/",
        "AI/generators/",
        "Makefile",
        ".github/workflows/ai-governance.yml"
      ],
      "requires_any_prefix": [
        "AI/compiled/"
      ]
    },
    {
      "trigger_prefixes": [
        "internal/contract/",
        "internal/cli/"
      ],
      "requires_any_prefix": [
        "AI/compiled/CONTRACT_BASELINE.json",
        "AI/compiled/JSON_CONTRACT_BASELINE/"
      ]
    },
    {
      "trigger_prefixes": [
        "scripts/doctor.sh",
        "scripts/backup.sh",
        "scripts/restore.sh",
        "scripts/migrate.sh",
        "scripts/espo.sh",
        "scripts/regression-test.sh",
        "scripts/lib/common.sh"
      ],
      "requires_any_prefix": [
        "AI/compiled/"
      ]
    }
  ],
  "allowed_root_markdown": [
    "AGENTS.md",
    "README.md",
    "CONTRIBUTING.md"
  ],
  "required_ops_env_examples": [
    "ops/env/.env.dev.example",
    "ops/env/.env.prod.example"
  ],
  "allowed_github_top_level": [
    "workflows"
  ],
  "allowed_workflows": [
    "ai-governance.yml"
  ]
}
