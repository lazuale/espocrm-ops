#!/usr/bin/env python3
import argparse
import json
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC_DIR = ROOT / "AI" / "spec"
COMPILED_DIR = ROOT / "AI" / "compiled"


def load_specs() -> dict:
    specs = {}
    for path in sorted(SPEC_DIR.glob("*.spec")):
        specs[path.stem] = json.loads(path.read_text(encoding="utf-8"))
    return specs


def ensure_parent(path: pathlib.Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)


def write_or_check(path: pathlib.Path, content: str, check: bool) -> int:
    ensure_parent(path)
    if check:
        if not path.exists():
            print(f"missing generated file: {path.relative_to(ROOT)}", file=sys.stderr)
            return 1
        if path.read_text(encoding="utf-8") != content:
            print(f"outdated generated file: {path.relative_to(ROOT)}", file=sys.stderr)
            return 1
        return 0
    path.write_text(content, encoding="utf-8")
    return 0


def make_policy(specs: dict) -> str:
    sync = specs["SYNC"]
    architecture = specs["ARCH"]
    surface = specs["SURFACE"]
    policy = {
        "version": 1,
        "readme_required_snippets": sync["readme_required_snippets"],
        "contributing_required_snippets": sync["contributing_required_snippets"],
        "contributing_forbidden_snippets": sync["contributing_forbidden_snippets"],
        "agents_required_snippets": sync["agents_required_snippets"],
        "changed_file_rules": sync["changed_file_rules"],
        "canonical_contract": surface["canonical"],
        "json_fixture_glob": surface["json_fixture_glob"],
        "shell_parse_smoke": surface["shell_parse_smoke"],
        "non_canonical_shell_json": surface["non_canonical_shell_json"],
        "passthrough_shell_json": surface["passthrough_shell_json"],
        "banned_directory_names": architecture["banned_directory_names"],
        "banned_package_names": architecture["banned_package_names"],
        "banned_file_stems": architecture["banned_file_stems"],
        "allowed_internal_roots": architecture["allowed_internal_roots"],
        "allowed_cmd_roots": architecture["allowed_cmd_roots"],
        "frozen_shell_debt": surface["frozen_shell_debt"],
        "non_canonical_json_scripts": [
            item.removesuffix(" --json") for item in surface["non_canonical_shell_json"]
        ],
        "passthrough_json_scripts": [
            item.removesuffix(" --json") for item in surface["passthrough_shell_json"]
        ],
    }
    return json.dumps(policy, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    specs = load_specs()
    failures = 0

    failures |= write_or_check(COMPILED_DIR / "POLICY.json", make_policy(specs), args.check)

    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
