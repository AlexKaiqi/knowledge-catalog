#!/usr/bin/env python3
"""Paid DSH Agent acceptance for the shell-only KC surface and task mount."""

from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
import tempfile
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-roles")
MODEL_PATCH = Path(os.environ.get("DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")))
DSH_EXECUTABLE = os.environ.get("DSH_EXECUTABLE", "dsh")
KC_EXECUTABLE = os.environ.get("KC_EXECUTABLE", "kc")
KCFS_EXECUTABLE = os.environ.get("KCFS_EXECUTABLE", "kcfs")
KC_HOME = Path(os.environ.get("KC_HOME", tempfile.mkdtemp(prefix="kc-agent-role-home-"))).resolve()
ARTIFACTS = Path(os.environ.get("KC_ROLE_ARTIFACTS", tempfile.mkdtemp(prefix="kc-agent-role-evidence-")))

CATALOG = "kr://acme/catalog"
REPO = "kr://acme/public/core"
WORKSPACE = "agent"
OBJECT = "policy/P-103"
RETIRED_MODEL_TOOLS = {
    "kc", "resource", "knowledge_context", "knowledge_list", "knowledge_search",
    "knowledge_read", "knowledge_schema", "knowledge_relations", "knowledge_provenance",
}

CONSUMER_TASK = f"""Load and follow the bundled knowledge-catalog skill. The task
mount, identity, Catalog, Workspace and pin are already injected. Through the shell,
run `kc knowledge read --object {OBJECT}` without passing --home, --as, --catalog,
--workspace or --pin. Confirm v=2 and status=governed. Read-only files may be inspected
with ordinary shell commands, but do not call any KC/VFS model tool. Reply exactly
CONSUMER=v2."""

UNAUTHORIZED_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, attempt exactly one grouped command `kc writer put` for {OBJECT} in {REPO}
with command-id mallory-denied-1 and value {{"v":999}}. It must return FORBIDDEN.
Do not retry or alter identity. Reply exactly UNAUTHORIZED=FORBIDDEN."""


def kc_json(*args: str, principal: str | None = None) -> dict | list:
    command = [KC_EXECUTABLE, "--home", str(KC_HOME)]
    if principal:
        command += ["--as", principal]
    command += list(args)
    proc = subprocess.run(command, capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"{' '.join(command)} failed: {proc.stdout or proc.stderr}")
    return json.loads(proc.stdout)


def grant(principal: str, action: str, *, repository: bool = False) -> None:
    scope = ["--repo", REPO] if repository else ["--catalog", CATALOG, "--workspace", WORKSPACE]
    kc_json("admin", "grant", "add", "--principal", principal, "--action", action, *scope)


def bootstrap() -> None:
    kc_json("local", "init", "--catalog", CATALOG)
    kc_json("local", "repository", "attach", "--repo", REPO)
    kc_json(
        "writer", "put", "--command-id", "owner-seed-1", "--repo", REPO,
        "--object", OBJECT, "--value", '{"v":2,"status":"governed"}',
        "--origin-kind", "SOURCE", "--source-ref", "agent://fixture/bootstrap", "--actor-ref", "fixture",
    )
    kc_json(
        "catalog", "workspace", "define", "--workspace", WORKSPACE, "--revision", "1",
        "--source", f"{REPO}=refs/heads/main@knowledge",
    )
    for principal in ("consumer", "mallory"):
        grant(principal, "workspace.resolve")
        grant(principal, "file.read", repository=True)
    grant("consumer", "knowledge.read", repository=True)


def verify_trace(name: str, workdir: Path) -> None:
    dsh_home = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh")))
    traces = list((dsh_home / "sessions").glob(f"*{workdir.name}*/session-*/session.jsonl.zstd"))
    if not traces:
        raise RuntimeError(f"{name} produced no DSH trace")
    trace = max(traces, key=lambda item: item.stat().st_mtime)
    decoded = subprocess.run(["zstd", "-dc", str(trace)], capture_output=True, text=True, check=True).stdout
    skill_loaded = False
    shell_calls = 0
    retired: list[str] = []
    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "tool/call":
            continue
        data = event.get("data", {})
        tool = str(data.get("name", ""))
        if tool in RETIRED_MODEL_TOOLS:
            retired.append(tool)
        if tool in {"bash", "shell"}:
            shell_calls += 1
        if tool == "skill":
            try:
                skill_loaded = json.loads(data.get("arguments", "{}")).get("name") == "knowledge-catalog" or skill_loaded
            except (json.JSONDecodeError, TypeError):
                pass
    evidence = {"trace": str(trace), "skillLoaded": skill_loaded, "shellCalls": shell_calls, "retiredModelTools": retired}
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / f"{name}.trace.json").write_text(json.dumps(evidence, indent=2) + "\n")
    if not skill_loaded or shell_calls == 0 or retired:
        raise RuntimeError(f"invalid {name} Agent surface evidence: {evidence}")


def run_role(name: str, principal: str, task: str, marker: str) -> None:
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-role-{name}-")).resolve()
    env = os.environ.copy()
    env.update({
        "DSH_PERMISSION_MODE": "danger-full-access", "KC_HOME": str(KC_HOME),
        "KC_WORKSPACE": WORKSPACE, "KC_AS": principal, "KCFS_BIN": KCFS_EXECUTABLE,
    })
    proc = subprocess.run(
        [DSH_EXECUTABLE, "--profile", PROFILE, "--patch", str(MODEL_PATCH), task],
        cwd=workdir, env=env, capture_output=True, text=True, timeout=480,
    )
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / f"{name}.stdout.txt").write_text(proc.stdout)
    (ARTIFACTS / f"{name}.stderr.txt").write_text(proc.stderr)
    if proc.returncode != 0 or marker not in proc.stdout:
        raise RuntimeError(f"{name} failed; expected {marker}; stderr:\n{proc.stderr[-4000:]}")
    verify_trace(name, workdir)


def current_value() -> dict:
    result = kc_json("knowledge", "read", "--repo", REPO, "--object", OBJECT, "--ref", "refs/heads/main")
    return result["value"]  # type: ignore[index]


def main() -> int:
    if platform.system() != "Linux" or not Path("/dev/fuse").exists():
        print("SKIP: paid DSH mount acceptance requires Linux FUSE")
        return 0
    if not MODEL_PATCH.is_file():
        print(f"missing model patch: {MODEL_PATCH}", file=sys.stderr)
        return 1
    try:
        bootstrap()
        before = current_value()
        run_role("consumer", "consumer", CONSUMER_TASK, "CONSUMER=v2")
        run_role("unauthorized", "mallory", UNAUTHORIZED_TASK, "UNAUTHORIZED=FORBIDDEN")
        after = current_value()
        if before != after or after != {"v": 2, "status": "governed"}:
            raise RuntimeError("unauthorized Agent changed authoritative state")
        (ARTIFACTS / "oracle.json").write_text(json.dumps({"before": before, "after": after}, indent=2) + "\n")
        print("PASS: DSH shell-only fixed-pin mount and denied write")
        return 0
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
