#!/usr/bin/env python3
"""Build the final read-only oracle from a completed resource-role evidence dir."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

REPO = "kr://demo/payments/operations"
OBJECT = "Service:payment-api"


def load_lines(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def kc(root: Path, *args: str) -> object:
    output = subprocess.run(
        [str(root / "kc"), "--home", str(root / "kc-home"), *args],
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    return json.loads(output)


def main() -> int:
    if len(sys.argv) != 2:
        raise SystemExit("usage: finalize_resource_evidence.py EVIDENCE_DIR")
    root = Path(sys.argv[1]).resolve()
    runs = load_lines(root / "host-home" / "runs" / "payment-ops.jsonl")
    traces = load_lines(root / "host-home" / "access.jsonl")
    successful = [run for run in runs if run.get("outcome") == "SUCCEEDED"]
    if len(successful) != 2:
        raise SystemExit(f"expected exactly two knowledge-changing scheduled runs: {successful}")
    if [run["summary"] for run in successful] != [
        {"added": 1, "updated": 0, "removed": 0, "unchanged": 0, "ignored": 0},
        {"added": 0, "updated": 1, "removed": 0, "unchanged": 0, "ignored": 0},
    ]:
        raise SystemExit(f"unexpected collection summaries: {successful}")
    if any(run.get("trigger", {}).get("kind") != "schedule" for run in successful):
        raise SystemExit(f"knowledge change was not automatic: {successful}")
    initial_commit = successful[0]["targetCommit"]
    updated_commit = successful[1]["targetCommit"]
    initial = kc(root, "read", "--repo", REPO, "--commit", initial_commit, "--object", OBJECT)
    updated = kc(root, "read", "--repo", REPO, "--commit", updated_commit, "--object", OBJECT)
    encoded_initial = json.dumps(initial, sort_keys=True)
    encoded_updated = json.dumps(updated, sort_keys=True)
    if not all(value in encoded_initial for value in ("payments-platform", "healthy", "ops-r1")):
        raise SystemExit(f"bad initial knowledge: {initial}")
    if not all(value in encoded_updated for value in ("payments-sre", "degraded", "ops-r2")):
        raise SystemExit(f"bad updated knowledge: {updated}")
    if len(traces) != 4:
        raise SystemExit(f"expected four consumer resource accesses: {traces}")
    sessions = {trace.get("identity", {}).get("session") for trace in traces}
    if len(sessions) != 2 or None in sessions:
        raise SystemExit(f"resource accesses did not retain two DSH sessions: {traces}")
    for trace in traces:
        if trace.get("identity", {}).get("principal") != "consumer":
            raise SystemExit(f"resource trace lost consumer identity: {trace}")
        if not trace.get("descriptor", {}).get("commit") or not trace.get("generation"):
            raise SystemExit(f"resource trace lost pinned coordinates: {trace}")
    role_traces = {}
    for path in sorted(root.glob("*.trace.json")):
        role_traces[path.stem] = json.loads(path.read_text())
    expected_roles = {
        "owner.trace", "developer.trace", "operator.trace", "consumer-initial.trace",
        "source-owner.trace", "consumer-updated.trace", "auditor.trace",
    }
    if set(role_traces) != expected_roles:
        raise SystemExit(f"missing DSH role traces: {set(role_traces)}")
    oracle = {
        "pass": True,
        "scenario": "payment-api-operations",
        "roles": sorted(role_traces),
        "initialKnowledge": initial,
        "updatedKnowledge": updated,
        "knowledgeChangingRuns": successful,
        "resourceAccessTraces": traces,
        "provenance": kc(root, "provenance", "--repo", REPO, "--commit", updated_commit, "--object", OBJECT),
        "log": kc(root, "log", "--repo", REPO, "--ref", "refs/heads/main", "--object", OBJECT),
        "dshRoleTraces": role_traces,
    }
    (root / "oracle.json").write_text(json.dumps(oracle, indent=2) + "\n")
    print(f"PASS: {root / 'oracle.json'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
