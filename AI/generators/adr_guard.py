#!/usr/bin/env python3
import json
import os
import pathlib
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = ROOT / "AI" / "spec" / "ADR_REQUIRED.spec"


def git_output(cmd: list[str]) -> str | None:
    try:
        return subprocess.check_output(cmd, cwd=ROOT, text=True)
    except subprocess.CalledProcessError:
        return None


def changed_files() -> list[str]:
    spec = json.loads(SPEC.read_text(encoding="utf-8"))
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
    files = {line.strip() for line in output.splitlines() if line.strip()}
    files.update(line.strip() for line in untracked.splitlines() if line.strip())
    files = sorted(
        path
        for path in files
        if not path.endswith("_test.go")
        and not path.startswith("AI/compiled/")
    )
    if not files:
        return []
    needs_adr = any(
        any(path.startswith(trigger) or path == trigger for trigger in spec["path_triggers"])
        for path in files
    )
    if not needs_adr:
        return []
    adr_prefix = spec["adr_directory"].rstrip("/") + "/"
    if any(path.startswith(adr_prefix) for path in files):
        return []
    return files


def main() -> int:
    offenders = changed_files()
    if offenders:
        print("ADR required for changed files but no ADR update found:", file=sys.stderr)
        for path in offenders:
            print(f"  - {path}", file=sys.stderr)
        return 1
    print("adr guard passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
