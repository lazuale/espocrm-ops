#!/usr/bin/env python3
import argparse
import hashlib
import json
import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = ROOT / "AI" / "spec" / "CONTRACT_SURFACE.spec"
BASELINE = ROOT / "AI" / "compiled" / "SHELL_DEBT_BASELINE.json"


def load_spec() -> dict:
    return json.loads(SPEC.read_text(encoding="utf-8"))


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def function_names(text: str) -> list[str]:
    names = set()
    patterns = [
        re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)\(\)\s*\{", re.MULTILINE),
        re.compile(r"^function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{", re.MULTILINE),
    ]
    for pattern in patterns:
        for match in pattern.finditer(text):
            names.add(match.group(1))
    return sorted(names)


def collect_state() -> dict:
    frozen = load_spec()["frozen_shell_debt"]
    files = {}
    for rel in sorted(frozen):
        path = ROOT / rel
        text = path.read_text(encoding="utf-8")
        files[rel] = {
            "sha256": sha256(path),
            "line_count": len(text.splitlines()),
            "function_names": function_names(text),
        }
    return {"version": 1, "files": files}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--write-baseline", action="store_true")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    state = collect_state()
    rendered = json.dumps(state, indent=2, sort_keys=True) + "\n"

    if args.write_baseline:
        BASELINE.write_text(rendered, encoding="utf-8")
        print(f"wrote {BASELINE.relative_to(ROOT)}")
        return 0

    if not BASELINE.exists():
        print(f"missing baseline: {BASELINE.relative_to(ROOT)}", file=sys.stderr)
        return 1

    if BASELINE.read_text(encoding="utf-8") != rendered:
        print("shell debt baseline drift detected; run make ai-refresh", file=sys.stderr)
        return 1

    print("shell debt baseline matched")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
