#!/usr/bin/env python3
"""Fail if the Agent signal pack drifts from recording rules, alerts, or the System Status dashboard."""
from __future__ import annotations

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES = ROOT / "docs" / "observability"
OVERVIEW = ROOT / "scripts" / "deploy" / "observability" / "grafana" / "kc-overview.json"


def names(text: str, kind: str) -> set[str]:
    return set(re.findall(rf"^\s+- {kind}: (\S+)", text, re.M))


def unquote(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        return value[1:-1]
    return value


def alert_next_queries(text: str) -> dict[str, str]:
    blocks = re.split(r"(?m)^\s+- alert: ", text)
    found: dict[str, str] = {}
    for block in blocks[1:]:
        name, _, rest = block.partition("\n")
        match = re.search(r"nextQuery: (.*)", rest)
        if match:
            found[name.strip()] = unquote(match.group(1))
    return found


def main() -> int:
    signals = json.loads((RULES / "agent-signals.json").read_text())
    recording = (RULES / "prometheus-recording-rules.yaml").read_text()
    alerts = (RULES / "prometheus-alert-rules.yaml").read_text()
    overview_raw = OVERVIEW.read_text()
    overview = json.loads(overview_raw)
    overview_exprs = "\n".join(
        str(target.get("expr", ""))
        for panel in overview.get("panels", [])
        for target in panel.get("targets", [])
    )
    errors: list[str] = []

    recorded = names(recording, "record")
    alerted = names(alerts, "alert")
    for item in signals["monitor"]:
        name = item.get("recording")
        if name and name not in recorded:
            errors.append(f"monitor {item['id']} recording {name} is missing")
    for item in signals["troubleshoot"]:
        for query in item.get("queries", []):
            if query.startswith("kc:") and " " not in query and query not in recorded:
                errors.append(f"troubleshoot {item['symptom']} recording {query} is missing")

    packed = {item["name"]: item for item in signals["alerts"]}
    if set(packed) != alerted:
        errors.append(f"alert set mismatch extra={sorted(set(packed) - alerted)} missing={sorted(alerted - set(packed))}")
    annotated = alert_next_queries(alerts)
    for name, item in packed.items():
        got = annotated.get(name)
        if got != item["nextQuery"]:
            errors.append(f"alert {name} nextQuery want {item['nextQuery']!r} got {got!r}")

    selector = signals["selectors"]["canonicalReadOperation"]
    if f'kc_operation="{selector}"' not in recording:
        errors.append(f"READ SLI must select kc_operation={selector!r}")
    if re.search(r'kc_operation="read"', recording):
        errors.append('READ SLI still selects kc_operation="read"; live command is knowledge-read')
    if re.search(r'kc_operation="read"', alerts):
        errors.append('READ alerts still select kc_operation="read"; live command is knowledge-read')

    if "clamp_min" in overview_raw:
        errors.append("System Status dashboard must not clamp missing traffic into 0%")
    for needle in (
        "kc_writer_commands_total",
        'kc_operation="knowledge-read"',
        "kc_search_requests_total",
        "kc:sli:writer_availability:ratio",
        "kc:sli:read_availability:ratio",
        "kc:sli:search_availability:ratio",
    ):
        if needle not in overview_exprs:
            errors.append(f"System Status dashboard missing {needle}")

    if errors:
        print("observability signal pack drift:", file=sys.stderr)
        for error in errors:
            print(f"  {error}", file=sys.stderr)
        return 1
    print("observability signal pack ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
