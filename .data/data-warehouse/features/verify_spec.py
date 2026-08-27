from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent
COMMAND_PREFIXES = ("kc ", "printf ", '"$CONNECTOR_PREVIEW" ', "jq ", "docker ")


def next_step(lines: list[str], start: int) -> str:
    in_doc_string = False
    for line in lines[start:]:
        stripped = line.strip()
        if stripped.startswith('"""'):
            in_doc_string = not in_doc_string
            continue
        if in_doc_string or not stripped or stripped.startswith(("#", "|")):
            continue
        if re.match(r"^(Given|When|Then|And|But)\b", stripped):
            return stripped
    return ""


def verify(path: Path) -> list[str]:
    errors: list[str] = []
    lines = path.read_text(encoding="utf-8").splitlines()
    for index, line in enumerate(lines):
        stripped = line.strip()
        if not stripped.startswith("When "):
            continue
        following = next_step(lines, index + 1)
        if not following.startswith("Then "):
            errors.append(f"{path}:{index + 1}: When is not immediately followed by Then")
        match = re.fullmatch(r"When I run `(.+)`", stripped)
        if match and not match.group(1).startswith(COMMAND_PREFIXES):
            errors.append(f"{path}:{index + 1}: command is not a recognized real executable")
    return errors


def main() -> int:
    paths = sorted(ROOT.glob("*.feature"))
    errors = [error for path in paths for error in verify(path)]
    if not paths:
        errors.append("no Gherkin feature files found")
    cli_ids: set[str] = set()
    companion_ids: set[str] = set()
    for path in paths:
        body = path.read_text(encoding="utf-8")
        cli_ids.update(re.findall(r"@((?:DW-CLI)-\d+)", body))
        companion_ids.update(re.findall(r"@companion-((?:DW-CLI)-\d+)", body))
    if not cli_ids:
        errors.append("no normative DW-CLI cases found")
    unknown_companions = companion_ids - cli_ids
    if unknown_companions:
        errors.append(f"Agent companion references unknown CLI cases: {sorted(unknown_companions)}")
    agent_path = ROOT / "agent.feature"
    if not agent_path.is_file() or "@agent" not in agent_path.read_text(encoding="utf-8"):
        errors.append("Agent companion must be isolated behind @agent")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(
        "feature contract: PASS "
        f"({len(cli_ids)} normative CLI cases; Agent companions for {len(companion_ids)} cases)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
