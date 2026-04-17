#!/usr/bin/env python3
import json
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = ROOT / "AI" / "spec" / "ADR_SEMANTIC.spec"
ADR_DIR = ROOT / "ops" / "adr"


def main() -> int:
    spec = json.loads(SPEC.read_text(encoding="utf-8"))
    for path in sorted(ADR_DIR.glob("*.md")):
        text = path.read_text(encoding="utf-8")
        for section in spec["required_sections"]:
            heading = f"## {section}"
            if heading not in text:
                print(f"{path.relative_to(ROOT)} missing {heading}", file=sys.stderr)
                return 1
    print("semantic adr guard passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

