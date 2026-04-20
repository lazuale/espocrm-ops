#!/usr/bin/env python3
import argparse
import hashlib
import json
import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
BASELINE = ROOT / "AI" / "compiled" / "CONTRACT_BASELINE.json"
SPEC = ROOT / "AI" / "spec" / "SURFACE.spec"


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_spec() -> dict:
    return json.loads(SPEC.read_text(encoding="utf-8"))


def collect_go_files(spec: dict) -> list[pathlib.Path]:
    paths = spec.get("baseline_paths") or spec.get("canonical_paths") or []
    collected: list[pathlib.Path] = []
    for rel in paths:
        path = ROOT / rel
        if not path.exists():
            continue
        if path.is_file():
            if path.suffix == ".go" and not path.name.endswith("_test.go"):
                collected.append(path)
            continue
        collected.extend(
            child
            for child in path.rglob("*.go")
            if not child.name.endswith("_test.go")
        )
    return sorted({path for path in collected})


def collect_command_specs(cli_files: list[pathlib.Path]) -> list[dict]:
    command_specs = []
    patterns = {
        "use": re.compile(r'Use:\s+"([^"]+)"'),
        "name": re.compile(r'Name:\s+"([^"]+)"'),
        "error_code": re.compile(r'ErrorCode:\s+"([^"]+)"'),
        "exit_code": re.compile(r"ExitCode:\s*([A-Za-z0-9_\.]+)"),
    }
    for path in cli_files:
        text = path.read_text(encoding="utf-8")
        if "CommandSpec{" not in text and 'Use:' not in text:
            continue
        entry = {"file": str(path.relative_to(ROOT))}
        for key, pattern in patterns.items():
            match = pattern.search(text)
            if match:
                entry[key] = match.group(1)
        if len(entry) > 1:
            command_specs.append(entry)
    return command_specs


def collect_state() -> dict:
    spec = load_spec()
    go_files = collect_go_files(spec)
    uses = []
    cli_dir = ROOT / "internal" / "cli"
    cli_files = [path for path in go_files if path.is_relative_to(cli_dir)]
    for path in cli_files:
        for match in re.finditer(r'Use:\s+"([^"]+)"', path.read_text(encoding="utf-8")):
            uses.append(match.group(1))
    return {
        "version": 1,
        "files": {
            str(path.relative_to(ROOT)): sha256(path)
            for path in go_files
        },
        "cobra_use_strings": sorted(set(uses)),
        "command_specs": collect_command_specs(cli_files),
    }


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
        print("contract baseline drift detected; run make ai-refresh", file=sys.stderr)
        return 1

    print("contract baseline matched")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
