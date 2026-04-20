#!/usr/bin/env python3
import argparse
import json
import os
import pathlib
import re
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
ARCH_SPEC = ROOT / "AI" / "spec" / "ARCH.spec"
SURFACE_SPEC = ROOT / "AI" / "spec" / "SURFACE.spec"
SYNC_SPEC = ROOT / "AI" / "spec" / "SYNC.spec"


def load_json(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git_output(cmd: list[str]) -> str | None:
    try:
        return subprocess.check_output(cmd, cwd=ROOT, text=True)
    except subprocess.CalledProcessError:
        return None


def changed_files() -> list[str]:
    base_sha = os.environ.get("BASE_SHA", "").strip()
    output = None
    if base_sha:
        output = git_output(["git", "diff", "--name-only", "--relative", f"{base_sha}...HEAD"])
    if output is None:
        output = git_output(["git", "diff", "--name-only", "--relative", "HEAD"])
    if output is None:
        output = git_output(["git", "diff-tree", "--no-commit-id", "--name-only", "--root", "-r", "HEAD"])
    if output is None:
        output = subprocess.check_output(["git", "ls-files"], cwd=ROOT, text=True)
    untracked = subprocess.check_output(
        ["git", "ls-files", "--others", "--exclude-standard"],
        cwd=ROOT,
        text=True,
    )
    changed = {line.strip() for line in output.splitlines() if line.strip()}
    changed.update(line.strip() for line in untracked.splitlines() if line.strip())
    return sorted(changed)


def docs_sync() -> int:
    sync = load_json(SYNC_SPEC)
    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    for snippet in sync["readme_required_snippets"]:
        if snippet not in readme:
            print(f"README.md missing required snippet: {snippet}", file=sys.stderr)
            return 1
    contributing_path = ROOT / "CONTRIBUTING.md"
    if not contributing_path.exists():
        print("missing CONTRIBUTING.md", file=sys.stderr)
        return 1
    contributing = contributing_path.read_text(encoding="utf-8")
    for snippet in sync["contributing_required_snippets"]:
        if snippet not in contributing:
            print(f"CONTRIBUTING.md missing required snippet: {snippet}", file=sys.stderr)
            return 1
    for snippet in sync["contributing_forbidden_snippets"]:
        if snippet in contributing:
            print(f"CONTRIBUTING.md contains forbidden snippet: {snippet}", file=sys.stderr)
            return 1
    agents = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    for snippet in sync["agents_required_snippets"]:
        if snippet not in agents:
            print(f"AGENTS.md missing required snippet: {snippet}", file=sys.stderr)
            return 1
    print("docs sync passed")
    return 0


def test_sync() -> int:
    rules = load_json(SYNC_SPEC)["changed_file_rules"]
    files = changed_files()
    if not files:
        print("test sync passed")
        return 0
    file_set = set(files)
    for rule in rules:
        triggered = any(
            path == prefix or path.startswith(prefix)
            for path in file_set
            for prefix in rule["trigger_prefixes"]
        )
        if not triggered:
            continue
        satisfied = any(
            any(path == prefix or path.startswith(prefix) for path in file_set)
            for prefix in rule["requires_any_prefix"]
        )
        if not satisfied:
            print("test/docs companion rule violated", file=sys.stderr)
            print(f"triggered by: {rule['trigger_prefixes']}", file=sys.stderr)
            print(f"requires one of: {rule['requires_any_prefix']}", file=sys.stderr)
            return 1
    print("test sync passed")
    return 0


def shell_guard() -> int:
    surface = load_json(SURFACE_SPEC)
    non_canonical = {
        item.removesuffix(" --json")
        for item in surface["non_canonical_shell_json"]
    }
    passthrough = {
        item.removesuffix(" --json")
        for item in surface["passthrough_shell_json"]
    }
    classified_json = non_canonical | passthrough

    for path in sorted((ROOT / "scripts").glob("*.sh")):
        rel = str(path.relative_to(ROOT))
        text = path.read_text(encoding="utf-8")
        exposes_json = "[--json]" in text or bool(re.search(r"^\s*--json\)", text, re.MULTILINE))
        if exposes_json and rel not in classified_json:
            print(f"{rel} exposes --json but is not classified as shell passthrough or non-canonical", file=sys.stderr)
            return 1
        if rel in non_canonical:
            if (
                '"canonical": false' not in text
                or '"contract_level": "non_canonical_shell"' not in text
                or '"machine_contract": false' not in text
            ):
                print(f"{rel} must explicitly mark json output as non-canonical shell data", file=sys.stderr)
                return 1
        if rel in passthrough:
            forbidden = ['json_escape(', '"canonical": false', '"contract_level": "non_canonical_shell"', '"machine_contract": false']
            for token in forbidden:
                if token in text:
                    print(f"{rel} is classified as passthrough json but contains shell-owned json token {token}", file=sys.stderr)
                    return 1
    print("shell guard passed")
    return 0


def package_guard() -> int:
    architecture = load_json(ARCH_SPEC)
    banned_dirs = set(architecture["banned_directory_names"])
    banned_pkgs = set(architecture["banned_package_names"])
    banned_file_stems = set(architecture["banned_file_stems"])
    allowed_internal_roots = set(architecture["allowed_internal_roots"])
    allowed_cmd_roots = set(architecture["allowed_cmd_roots"])
    internal_root = ROOT / "internal"
    cmd_root = ROOT / "cmd"
    if internal_root.exists():
        for path in sorted(internal_root.iterdir()):
            if path.is_dir() and path.name not in allowed_internal_roots:
                print(f"unexpected internal root: {path.relative_to(ROOT)}", file=sys.stderr)
                return 1
    if cmd_root.exists():
        for path in sorted(cmd_root.iterdir()):
            if path.is_dir() and path.name not in allowed_cmd_roots:
                print(f"unexpected cmd entrypoint: {path.relative_to(ROOT)}", file=sys.stderr)
                return 1
    for path in ROOT.rglob("*"):
        if not path.is_dir():
            continue
        if path.name in banned_dirs and ".git" not in path.parts:
            print(f"banned directory name: {path.relative_to(ROOT)}", file=sys.stderr)
            return 1
    for path in ROOT.rglob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        rel = path.relative_to(ROOT)
        if rel.parts[0] not in {"cmd", "internal"}:
            continue
        stem = path.stem.lower()
        if any(token in stem for token in banned_file_stems):
            print(f"banned go file stem {path.stem} in {rel}", file=sys.stderr)
            return 1
        for line in path.read_text(encoding="utf-8").splitlines():
            if line.startswith("package "):
                pkg = line.split()[1]
                if pkg in banned_pkgs:
                    print(f"banned package name {pkg} in {path.relative_to(ROOT)}", file=sys.stderr)
                    return 1
                break
    print("package guard passed")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["docs-sync", "test-sync", "shell-guard", "package-guard"])
    args = parser.parse_args()
    if args.mode == "docs-sync":
        return docs_sync()
    if args.mode == "test-sync":
        return test_sync()
    if args.mode == "shell-guard":
        return shell_guard()
    if args.mode == "package-guard":
        return package_guard()
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
