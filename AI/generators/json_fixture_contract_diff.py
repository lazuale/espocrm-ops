#!/usr/bin/env python3
import argparse
import hashlib
import json
import pathlib
import shutil
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE_DIR = ROOT / "internal" / "cli" / "testdata"
BASELINE_DIR = ROOT / "AI" / "compiled" / "JSON_CONTRACT_BASELINE"
MANIFEST = BASELINE_DIR / "manifest.json"


def source_files() -> list[pathlib.Path]:
    return sorted(path for path in SOURCE_DIR.glob("*.json"))


def file_hash(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_json(path: pathlib.Path) -> None:
    try:
        json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"{path}: invalid json: {exc}", file=sys.stderr)
        raise SystemExit(1)


def write_baseline() -> int:
    BASELINE_DIR.mkdir(parents=True, exist_ok=True)
    manifest = {}
    expected = {"manifest.json"}
    for path in source_files():
        validate_json(path)
        target = BASELINE_DIR / path.name
        shutil.copyfile(path, target)
        manifest[path.name] = file_hash(path)
        expected.add(path.name)
    for path in BASELINE_DIR.iterdir():
        if path.name not in expected and path.is_file():
            path.unlink()
    MANIFEST.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"wrote {BASELINE_DIR.relative_to(ROOT)}")
    return 0


def check_baseline() -> int:
    if not MANIFEST.exists():
        print(f"missing baseline manifest: {MANIFEST.relative_to(ROOT)}", file=sys.stderr)
        return 1
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    expected = {path.name for path in source_files()} | {"manifest.json"}
    actual = {path.name for path in BASELINE_DIR.iterdir() if path.is_file()}
    if actual != expected:
        print("json baseline directory contains unexpected files; run make ai-refresh", file=sys.stderr)
        return 1
    for path in source_files():
        validate_json(path)
        target = BASELINE_DIR / path.name
        if not target.exists():
            print(f"missing baseline fixture: {target.relative_to(ROOT)}", file=sys.stderr)
            return 1
        if file_hash(path) != manifest.get(path.name):
            print(f"json fixture drift: {path.relative_to(ROOT)}", file=sys.stderr)
            return 1
    print("json fixture baseline matched")
    return 0


def parse_files(paths: list[str]) -> int:
    for raw in paths:
        validate_json((ROOT / raw) if not pathlib.Path(raw).is_absolute() else pathlib.Path(raw))
    print("json parse check passed")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--write-baseline", action="store_true")
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--parse-files", nargs="*")
    args = parser.parse_args()

    if args.write_baseline:
        return write_baseline()
    if args.check:
        return check_baseline()
    if args.parse_files is not None:
        return parse_files(args.parse_files)
    parser.print_help()
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
