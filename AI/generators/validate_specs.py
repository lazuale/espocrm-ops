#!/usr/bin/env python3
import json
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC_DIR = ROOT / "AI" / "spec"
GENERATOR_DIR = ROOT / "AI" / "generators"
COMPILED_DIR = ROOT / "AI" / "compiled"
POLICY_DIR = ROOT / "policy"
REQUIRED_SPECS = {
    "ARCH.spec",
    "SURFACE.spec",
    "SYNC.spec",
}
ALLOWED_GENERATORS = {
    "ast_arch_guard.py",
    "contract_diff.py",
    "json_fixture_contract_diff.py",
    "runner.py",
    "validate_specs.py",
}
ALLOWED_COMPILED_ENTRIES = {
    "CONTRACT_BASELINE.json",
    "JSON_CONTRACT_BASELINE",
}
def main() -> int:
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
