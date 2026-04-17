#!/usr/bin/env python3
import json
import os
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = ROOT / "AI" / "spec" / "PR_BODY.spec"


def section_bodies(text: str, sections: list[str]) -> dict[str, str]:
    bodies: dict[str, str] = {}
    headings = [f"## {section}" for section in sections]
    positions = []
    for heading in headings:
        index = text.find(heading)
        if index == -1:
            continue
        positions.append((index, heading))
    positions.sort()
    for i, (start, heading) in enumerate(positions):
        body_start = start + len(heading)
        body_end = positions[i + 1][0] if i + 1 < len(positions) else len(text)
        bodies[heading.removeprefix("## ")] = text[body_start:body_end].strip()
    return bodies


def main() -> int:
    spec = json.loads(SPEC.read_text(encoding="utf-8"))
    has_real_body = "PR_BODY_PATH" in os.environ
    if not has_real_body:
        print("pr body check skipped")
        return 0
    target = pathlib.Path(os.environ["PR_BODY_PATH"])
    if not target.exists():
        print(f"missing PR body/template: {target}", file=sys.stderr)
        return 1
    text = target.read_text(encoding="utf-8")
    for section in spec["required_sections"]:
        heading = f"## {section}"
        if heading not in text:
            print(f"missing PR section: {heading}", file=sys.stderr)
            return 1
    if has_real_body:
        bodies = section_bodies(text, spec["required_sections"])
        placeholders = {"n/a", "na", "todo", "tbd", "same as template"}
        for section in spec["required_sections"]:
            body = bodies.get(section, "").strip()
            if not body:
                print(f"empty PR section: ## {section}", file=sys.stderr)
                return 1
            if body.lower() in placeholders:
                print(f"placeholder PR section: ## {section}", file=sys.stderr)
                return 1
        checks = bodies.get("Checks", "")
        if not any(token in checks for token in ("make ", "go test", "go vet", "shellcheck", "bash ")):
            print("PR Checks section must mention concrete commands", file=sys.stderr)
            return 1
        adr_body = bodies.get("ADR", "")
        if "ADR Required:" not in adr_body or "ADR Link:" not in adr_body:
            print("PR ADR section must include ADR Required and ADR Link lines", file=sys.stderr)
            return 1
        required_line = next((line for line in adr_body.splitlines() if line.startswith("ADR Required:")), "")
        link_line = next((line for line in adr_body.splitlines() if line.startswith("ADR Link:")), "")
        required_value = required_line.removeprefix("ADR Required:").strip().lower()
        link_value = link_line.removeprefix("ADR Link:").strip().lower()
        if required_value not in {"yes", "no"}:
            print("ADR Required must be yes or no", file=sys.stderr)
            return 1
        if required_value == "yes" and link_value in {"", "n/a", "na"}:
            print("ADR Link must be filled when ADR Required is yes", file=sys.stderr)
            return 1
    print("pr body check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
