#!/usr/bin/env python3
"""Trace oracle for the in-app-browser-driven DSH all-actions run.

The UI interaction itself is intentionally manual/black-box. This oracle reads
the resulting DSH traces and proves that the bundled Skill and the public kc
verb table were actually exercised by the role agents.
"""

from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path


EXPECTED_VERBS = {
    "allow", "allowed", "append", "archive-catalog", "archive-repo", "audit",
    "catalog-add", "checkout", "commit", "define-workspace", "describe-index",
    "describe-schema", "diff", "gate-add", "gate-ls", "gate-rm", "hook-add",
    "hook-ls", "hook-rm", "index-plan", "index-sync", "ingest", "init",
    "inspect", "list", "log", "merge", "mount", "overlay", "preview",
    "propose", "provenance", "put", "read", "receipt", "record-validation",
    "register", "remove", "repo-add", "resolve", "retire-workspace", "revoke", "search",
    "status", "store-ls", "store-set", "stream", "sync", "validate",
    "vfs-list", "vfs-read", "vfs-write", "whoami",
}

ROLE_EXPECTED = {
    "owner": {
        "allow", "allowed", "catalog-add", "define-workspace", "gate-add",
        "gate-ls", "hook-add", "hook-ls", "init", "mount", "overlay", "put",
        "read", "receipt", "register", "remove", "repo-add", "status",
        "store-ls", "store-set", "whoami",
    },
    "producer": {
        "append", "commit", "ingest", "propose", "put", "receipt", "remove",
        "vfs-write", "whoami",
    },
    "reviewer": {
        "merge", "preview", "read", "record-validation", "validate", "whoami",
    },
    "consumer": {
        "checkout", "describe-schema", "index-plan", "inspect", "list", "read",
        "resolve", "search", "status", "stream", "sync", "vfs-list", "vfs-read",
        "whoami",
    },
    "auditor": {
        "audit", "describe-index", "diff", "index-sync", "log", "provenance",
        "read", "whoami",
    },
    "unauthorized": {"put", "read", "whoami"},
    "lifecycle": {
        "allowed", "archive-catalog", "archive-repo", "gate-ls", "gate-rm",
        "hook-ls", "hook-rm", "index-plan", "retire-workspace", "revoke",
        "status", "whoami",
    },
}

FS_TOOLS = {"read", "write", "edit", "list", "glob", "grep"}

ROLE_MARKERS = {
    "owner": ["You are the Catalog Owner in a completely new empty environment"],
    "producer": [
        "You are the fixed Producer principal",
        "You are still the fixed Producer principal",
    ],
    "reviewer": ["You are the fixed Reviewer/Gatekeeper principal"],
    "consumer": ["You are the fixed Consumer principal"],
    "auditor": ["You are the fixed Auditor principal"],
    "unauthorized": ["You are fixed Unauthorized Actor mallory"],
    "lifecycle": ["You are the fixed Lifecycle Admin principal"],
}


def decode(trace: Path) -> list[dict]:
    result = subprocess.run(
        ["zstd", "-dc", str(trace)], capture_output=True, text=True, check=True
    )
    events = []
    for line in result.stdout.splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    return events


def find_traces(dsh_home: Path, role: str) -> list[Path]:
    candidates = []
    markers = ROLE_MARKERS[role]
    for path in (dsh_home / "sessions").rglob("session.jsonl.zstd"):
        events = decode(path)
        rendered = json.dumps(events, ensure_ascii=False)
        if any(marker in rendered for marker in markers):
            candidates.append(path)
    if not candidates:
        raise RuntimeError(f"no DSH trace found for role {role}")
    return sorted(candidates, key=lambda path: path.stat().st_mtime)


def inspect_traces(traces: list[Path]) -> dict:
    verbs: list[str] = []
    tools: list[str] = []
    skill_loaded = False
    for trace in traces:
        for event in decode(trace):
            if event.get("type") != "tool/call":
                continue
            data = event.get("data", {})
            name = str(data.get("name", ""))
            tools.append(name)
            raw = data.get("arguments", "{}")
            try:
                args = json.loads(raw) if isinstance(raw, str) else raw
            except (json.JSONDecodeError, TypeError):
                args = {}
            if name == "kc" and isinstance(args, dict) and isinstance(args.get("verb"), str):
                verbs.append(args["verb"])
            if name == "skill" and isinstance(args, dict) and args.get("name") == "knowledge-catalog":
                skill_loaded = True
    return {
        "traces": [str(trace) for trace in traces],
        "knowledgeCatalogSkillLoaded": skill_loaded,
        "kcVerbs": verbs,
        "tools": tools,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dsh-home", default=str(Path.home() / ".dsh"))
    parser.add_argument("--out")
    args = parser.parse_args()
    dsh_home = Path(args.dsh_home)
    roles = {}
    all_verbs: set[str] = set()
    all_tools: set[str] = set()
    failures: list[str] = []
    for role, expected in ROLE_EXPECTED.items():
        try:
            traces = find_traces(dsh_home, role)
            evidence = inspect_traces(traces)
        except (RuntimeError, subprocess.CalledProcessError) as error:
            failures.append(str(error))
            continue
        roles[role] = evidence
        role_verbs = set(evidence["kcVerbs"])
        all_verbs.update(role_verbs)
        all_tools.update(str(name).lower() for name in evidence["tools"])
        if not evidence["knowledgeCatalogSkillLoaded"]:
            failures.append(f"{role}: knowledge-catalog Skill was not loaded")
        missing_role = expected - role_verbs
        if missing_role:
            failures.append(f"{role}: missing verbs {sorted(missing_role)}")

    missing_verbs = EXPECTED_VERBS - all_verbs
    missing_fs = FS_TOOLS - all_tools
    if missing_verbs:
        failures.append(f"all roles: missing public verbs {sorted(missing_verbs)}")
    if missing_fs:
        failures.append(f"all roles: missing filesystem tools {sorted(missing_fs)}")
    report = {
        "expectedVerbCount": len(EXPECTED_VERBS),
        "observedVerbCount": len(all_verbs & EXPECTED_VERBS),
        "observedVerbs": sorted(all_verbs),
        "missingVerbs": sorted(missing_verbs),
        "filesystemTools": sorted(all_tools & FS_TOOLS),
        "missingFilesystemTools": sorted(missing_fs),
        "roles": roles,
        "pass": not failures,
        "failures": failures,
    }
    rendered = json.dumps(report, indent=2) + "\n"
    if args.out:
        Path(args.out).write_text(rendered)
    print(rendered, end="")
    return 0 if not failures else 1


if __name__ == "__main__":
    raise SystemExit(main())
