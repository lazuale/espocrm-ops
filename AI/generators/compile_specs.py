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
    docs = specs["DOCS_SYNC"]
    package_policy = specs["PACKAGE_POLICY"]
    contract_surface = specs["CONTRACT_SURFACE"]
    policy = {
        "version": 1,
        "readme_required_snippets": docs["readme_required_snippets"],
        "contributing_required_snippets": docs["contributing_required_snippets"],
        "contributing_forbidden_snippets": docs["contributing_forbidden_snippets"],
        "archived_docs": docs["archived_docs"],
        "archived_knowledge_docs": docs["archived_knowledge_docs"],
        "archive_banner": docs["archive_banner"],
        "knowledge_banner": docs["knowledge_banner"],
        "canonical_contract": contract_surface["canonical"],
        "non_canonical_shell_json": contract_surface["non_canonical_shell_json"],
        "passthrough_shell_json": contract_surface["passthrough_shell_json"],
        "banned_directory_names": package_policy["banned_directory_names"],
        "banned_package_names": package_policy["banned_package_names"],
        "banned_file_stems": package_policy["banned_file_stems"],
        "allowed_internal_roots": package_policy["allowed_internal_roots"],
        "allowed_cmd_roots": package_policy["allowed_cmd_roots"],
        "frozen_shell_debt": contract_surface["frozen_shell_debt"],
        "non_canonical_json_scripts": [
            item.removesuffix(" --json") for item in contract_surface["non_canonical_shell_json"]
        ],
        "passthrough_json_scripts": [
            item.removesuffix(" --json") for item in contract_surface["passthrough_shell_json"]
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
