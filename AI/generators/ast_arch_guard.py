#!/usr/bin/env python3
import json
import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = ROOT / "AI" / "spec" / "ARCH.spec"


def go_files(prefix: str):
    directory = ROOT / prefix
    if not directory.exists():
        return []
    return [
        path
        for path in directory.rglob("*.go")
        if not path.name.endswith("_test.go")
    ]


def read(path: pathlib.Path) -> str:
    return path.read_text(encoding="utf-8")


def imports_of(text: str) -> list[str]:
    imports = []
    for match in re.finditer(r'"github\.com/lazuale/espocrm-ops/([^"]+)"', text):
        imports.append(match.group(1))
    return imports


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


def load_spec() -> dict:
    return json.loads(SPEC.read_text(encoding="utf-8"))


def check_imports(prefix: str, forbidden_prefixes: list[str]) -> None:
    for path in go_files(prefix):
        text = read(path)
        imports = imports_of(text)
        for imported in imports:
            for forbidden in forbidden_prefixes:
                if imported.startswith(forbidden):
                    fail(f"{path.relative_to(ROOT)} imports forbidden path {imported}")


def check_runtime_guard(rule: dict) -> None:
    flags = 0
    if "multiline" in rule.get("flags", []):
        flags |= re.MULTILINE
    pattern = re.compile(rule["pattern"], flags)
    for prefix in rule["paths"]:
        for path in go_files(prefix):
            if pattern.search(read(path)):
                fail(f"{path.relative_to(ROOT)} {rule['message']}")


def main() -> int:
    spec = load_spec()
    for prefix, rule in spec["layers"].items():
        forbidden = rule.get("must_not_import", [])
        if forbidden:
            check_imports(prefix, forbidden)
    for rule in spec["runtime_guards"]:
        check_runtime_guard(rule)
    print("architecture guard passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
