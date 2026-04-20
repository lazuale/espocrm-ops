#!/usr/bin/env python3
import json
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC_DIR = ROOT / "AI" / "spec"
GENERATOR_DIR = ROOT / "AI" / "generators"
COMPILED_DIR = ROOT / "AI" / "compiled"
POLICY_DIR = ROOT / "policy"
WORKFLOWS_DIR = ROOT / ".github" / "workflows"
GITHUB_DIR = ROOT / ".github"
REQUIRED_SPECS = {
    "ARCH.spec",
    "SURFACE.spec",
    "SYNC.spec",
}
ALLOWED_GENERATORS = {
    "ast_arch_guard.py",
    "compile_specs.py",
    "contract_diff.py",
    "json_fixture_contract_diff.py",
    "runner.py",
    "shell_debt_diff.py",
    "validate_specs.py",
}
ALLOWED_COMPILED_ENTRIES = {
    "CONTRACT_BASELINE.json",
    "JSON_CONTRACT_BASELINE",
    "POLICY.json",
    "SHELL_DEBT_BASELINE.json",
}
def main() -> int:
    parsed_specs = {}
    spec_entries = {path.name for path in SPEC_DIR.iterdir()}
    unexpected_spec_entries = sorted(spec_entries - REQUIRED_SPECS)
    if unexpected_spec_entries:
        print("unexpected spec entries:", ", ".join(unexpected_spec_entries), file=sys.stderr)
        return 1
    spec_names = {path.name for path in SPEC_DIR.glob("*.spec")}
    missing = sorted(REQUIRED_SPECS - spec_names)
    if missing:
        print("missing spec files:", ", ".join(missing), file=sys.stderr)
        return 1

    for path in sorted(SPEC_DIR.glob("*.spec")):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            print(f"{path}: invalid json: {exc}", file=sys.stderr)
            return 1
        if not isinstance(data, dict):
            print(f"{path}: spec must be a json object", file=sys.stderr)
            return 1
        if data.get("name") in (None, ""):
            print(f"{path}: missing 'name'", file=sys.stderr)
            return 1
        if not isinstance(data.get("version"), int):
            print(f"{path}: missing integer 'version'", file=sys.stderr)
            return 1
        parsed_specs[path.name] = data

    sync = parsed_specs["SYNC.spec"]

    generator_entries = {path.name for path in GENERATOR_DIR.iterdir()}
    unexpected_generators = sorted(generator_entries - ALLOWED_GENERATORS)
    if unexpected_generators:
        print("unexpected generator entries:", ", ".join(unexpected_generators), file=sys.stderr)
        return 1

    compiled_entries = {path.name for path in COMPILED_DIR.iterdir()}
    unexpected_compiled = sorted(compiled_entries - ALLOWED_COMPILED_ENTRIES)
    if unexpected_compiled:
        print("unexpected compiled entries:", ", ".join(unexpected_compiled), file=sys.stderr)
        return 1

    if not GITHUB_DIR.exists():
        print("missing .github directory", file=sys.stderr)
        return 1
    github_entries = {path.name for path in GITHUB_DIR.iterdir()}
    unexpected_github_entries = sorted(github_entries - set(sync["allowed_github_top_level"]))
    if unexpected_github_entries:
        print("unexpected .github entries:", ", ".join(unexpected_github_entries), file=sys.stderr)
        return 1
    missing_github_entries = sorted(set(sync["allowed_github_top_level"]) - github_entries)
    if missing_github_entries:
        print("missing .github entries:", ", ".join(missing_github_entries), file=sys.stderr)
        return 1

    if not WORKFLOWS_DIR.exists():
        print("missing .github/workflows directory", file=sys.stderr)
        return 1
    workflow_entries = {path.name for path in WORKFLOWS_DIR.iterdir()}
    unexpected_workflows = sorted(workflow_entries - set(sync["allowed_workflows"]))
    if unexpected_workflows:
        print("unexpected workflow entries:", ", ".join(unexpected_workflows), file=sys.stderr)
        return 1
    missing_workflows = sorted(set(sync["allowed_workflows"]) - workflow_entries)
    if missing_workflows:
        print("missing workflow files:", ", ".join(missing_workflows), file=sys.stderr)
        return 1
    root_markdown = {path.name for path in ROOT.glob("*.md")}
    unexpected_root_markdown = sorted(root_markdown - set(sync["allowed_root_markdown"]))
    if unexpected_root_markdown:
        print(
            "unexpected root markdown:",
            ", ".join(unexpected_root_markdown),
            file=sys.stderr,
        )
        return 1

    root_env_examples = sorted(path.name for path in ROOT.glob(".env.*.example"))
    if root_env_examples:
        print(
            "unexpected root env examples:",
            ", ".join(root_env_examples),
            file=sys.stderr,
        )
        return 1

    missing_ops_env_examples = sorted(
        rel for rel in sync["required_ops_env_examples"] if not (ROOT / rel).exists()
    )
    if missing_ops_env_examples:
        print(
            "missing ops env examples:",
            ", ".join(missing_ops_env_examples),
            file=sys.stderr,
        )
        return 1

    ops_dir = ROOT / "ops"
    if ops_dir.exists():
        unexpected_ops_markdown = sorted(
            str(path.relative_to(ROOT))
            for path in ops_dir.rglob("*.md")
        )
        if unexpected_ops_markdown:
            print(
                "unexpected ops markdown:",
                ", ".join(unexpected_ops_markdown),
                file=sys.stderr,
            )
            return 1

    if POLICY_DIR.exists():
        print("unexpected legacy governance directory: policy", file=sys.stderr)
        return 1

    agents_dir = ROOT / ".agents"
    if agents_dir.exists():
        print("unexpected legacy governance directory: .agents", file=sys.stderr)
        return 1

    print("specs valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
