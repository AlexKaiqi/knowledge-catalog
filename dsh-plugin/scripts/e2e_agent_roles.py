#!/usr/bin/env python3
"""Paid DSH Agent acceptance for the six core shell-only KC roles."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-roles")
MODEL_PATCH = Path(os.environ.get("DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")))
DSH_EXECUTABLE = os.environ.get("DSH_EXECUTABLE", "dsh")
KC_EXECUTABLE = os.environ.get("KC_EXECUTABLE", "kc")
KC_HOME = Path(os.environ.get("KC_HOME", tempfile.mkdtemp(prefix="kc-agent-role-home-"))).resolve()
KC_SERVER_URL = os.environ.get("KC_SERVER_URL", "")
KC_ADMIN_PRINCIPAL = os.environ.get("KC_AS", "service:agent-e2e")
ARTIFACTS = Path(os.environ.get("KC_ROLE_ARTIFACTS", tempfile.mkdtemp(prefix="kc-agent-role-evidence-")))
SKILL_ONLY_PATCH = PLUGIN / "scripts" / "questions-skill-only.patch.yml"
SCENARIOS = PLUGIN / "scripts" / "agent-scenarios.json"

CATALOG = "kr://acme/catalog"
REPO = "kr://acme/public/core"
WORKSPACE = "agent"
OBJECT = "policy/P-103"
PROVIDER_OBJECT = "policy/agent-published"
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

PROVIDER_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, publish exactly one source knowledge object with grouped CLI command
`kc writer put --command-id provider-agent-1 --repo {REPO} --object {PROVIDER_OBJECT}
--value '{{"v":3,"status":"agent-published"}}' --origin-kind SOURCE
--source-ref agent://provider/fixture`. Do not edit Repository files or call git.
After the JSON receipt succeeds, reply exactly PROVIDER=APPLIED."""

GOVERNOR_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, run exactly `kc catalog workspace check --catalog {CATALOG} --workspace
{WORKSPACE}`. Confirm the returned outcome is PASSED and reply exactly
GOVERNOR=PASSED."""

AUDITOR_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, inspect {OBJECT} with `kc knowledge provenance --object {OBJECT}` and
`kc knowledge log --object {OBJECT}`, then inspect the Catalog registry with
`kc catalog audit --catalog {CATALOG} --limit 20`. Do not write anything. After
all three grouped commands return JSON, reply exactly AUDITOR=OBSERVED."""

RECOVERY_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, first run `kc knowledge read --workspace wrong --object {OBJECT}` and
confirm it fails with PRECONDITION_FAILED because the task already owns a fixed
Workspace coordinate. Then retry as `kc knowledge read --object {OBJECT}` without
passing task coordinates, confirm v=2, and reply exactly RECOVERY=v2."""

UNAUTHORIZED_TASK = f"""Load and follow the bundled knowledge-catalog skill. Through
the shell, attempt exactly one grouped command `kc writer put` for {OBJECT} in {REPO}
with command-id mallory-denied-1 and value {{"v":999}}. It must return FORBIDDEN.
Do not retry or alter identity. Reply exactly UNAUTHORIZED=FORBIDDEN."""

ROLES = (
    ("provider", "provider", PROVIDER_TASK, "PROVIDER=APPLIED"),
    ("governor", "governor", GOVERNOR_TASK, "GOVERNOR=PASSED"),
    ("consumer", "consumer", CONSUMER_TASK, "CONSUMER=v2"),
    ("auditor", "auditor", AUDITOR_TASK, "AUDITOR=OBSERVED"),
    ("recovery", "recovery", RECOVERY_TASK, "RECOVERY=v2"),
    ("unauthorized", "mallory", UNAUTHORIZED_TASK, "UNAUTHORIZED=FORBIDDEN"),
)


def scenario_names(key: str) -> tuple[str, ...]:
    contract = json.loads(SCENARIOS.read_text())
    if contract.get("version") != 1 or not isinstance(contract.get(key), list):
        raise RuntimeError(f"invalid Agent scenario contract: {SCENARIOS}")
    return tuple(str(name) for name in contract[key])


def kc_json(*args: str, principal: str | None = None) -> dict | list:
    if not KC_SERVER_URL:
        raise RuntimeError("KC_SERVER_URL is required; Agent acceptance only uses KC Server")
    command = [
        KC_EXECUTABLE, "--server", KC_SERVER_URL,
        "--as", principal or KC_ADMIN_PRINCIPAL,
    ]
    command += list(args)
    proc = subprocess.run(command, capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"{' '.join(command)} failed: {proc.stdout or proc.stderr}")
    return json.loads(proc.stdout)


def grant(principal: str, action: str, *, repository: bool = False, workspace: bool = True) -> None:
    scope = ["--repo", REPO] if repository else ["--catalog", CATALOG]
    if not repository and workspace:
        scope += ["--workspace", WORKSPACE]
    kc_json("admin", "grant", "add", "--principal", principal, "--action", action, *scope)


def bootstrap() -> None:
    kc_json(
        "writer", "put", "--command-id", "owner-seed-1", "--repo", REPO,
        "--object", OBJECT, "--value", '{"v":2,"status":"governed"}',
        "--origin-kind", "SOURCE", "--source-ref", "agent://fixture/bootstrap", "--actor-ref", "fixture",
    )
    kc_json(
        "catalog", "workspace", "define", "--workspace", WORKSPACE, "--revision", "1",
        "--source", f"{REPO}=refs/heads/main@knowledge",
    )
    for principal in ("consumer", "auditor", "recovery"):
        grant(principal, "workspace.consume")
        grant(principal, "knowledge.read", repository=True)
    grant("provider", "writer.commit", repository=True)
    grant("governor", "workspace.resolve")
    grant("governor", "knowledge.read", repository=True)
    grant("auditor", "knowledge.provenance", repository=True)
    grant("auditor", "knowledge.history.read", repository=True)
    grant("auditor", "catalog.audit.read", workspace=False)


def task_context(name: str, principal: str, workdir: Path, pin: dict | list) -> None:
    directory = KC_HOME / "tasks" / name
    directory.mkdir(parents=True, exist_ok=True)
    context = {
        "version": 1,
        "sessionId": name,
        "principal": principal,
        "catalog": CATALOG,
        "workspace": WORKSPACE,
        "pin": pin,
        "root": str(workdir),
        "readOnly": True,
        "mounts": [],
    }
    (directory / "context.json").write_text(json.dumps(context, indent=2) + "\n")


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


def run_role(name: str, principal: str, task: str, marker: str, pin: dict | list) -> None:
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-role-{name}-")).resolve()
    task_context(name, principal, workdir, pin)
    env = os.environ.copy()
    env.update({
        "DSH_PERMISSION_MODE": "danger-full-access", "KC_HOME": str(KC_HOME),
        "KC_SERVER_URL": KC_SERVER_URL, "KC_WORKSPACE": WORKSPACE, "KC_AS": principal,
    })
    proc = subprocess.run(
        [
            DSH_EXECUTABLE, "--profile", PROFILE,
            "--patch", str(MODEL_PATCH), "--patch", str(SKILL_ONLY_PATCH), task,
        ],
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


def value_of(object_id: str) -> dict:
    result = kc_json("knowledge", "read", "--repo", REPO, "--object", object_id, "--ref", "refs/heads/main")
    return result["value"]  # type: ignore[index]


def main() -> int:
    if not MODEL_PATCH.is_file():
        print(f"missing model patch: {MODEL_PATCH}", file=sys.stderr)
        return 1
    if not SKILL_ONLY_PATCH.is_file():
        print(f"missing Skill-only patch: {SKILL_ONLY_PATCH}", file=sys.stderr)
        return 1
    if not SCENARIOS.is_file():
        print(f"missing Agent scenario contract: {SCENARIOS}", file=sys.stderr)
        return 1
    try:
        expected = scenario_names("coreRoles")
        actual = tuple(role[0] for role in ROLES)
        if actual != expected or len(set(actual)) != len(actual):
            raise RuntimeError(f"core Agent roles drifted: expected {expected}, got {actual}")
        selected_names = {
            name.strip() for name in os.environ.get("KC_ROLE_FILTER", "").split(",") if name.strip()
        }
        unknown = selected_names - set(actual)
        if unknown:
            raise RuntimeError(f"unknown KC_ROLE_FILTER names: {sorted(unknown)}")
        roles = tuple(role for role in ROLES if not selected_names or role[0] in selected_names)
        if not roles:
            raise RuntimeError("KC_ROLE_FILTER selected no roles")
        bootstrap()
        before = current_value()
        pin = kc_json("catalog", "workspace", "resolve", "--workspace", WORKSPACE)
        for name, principal, task, marker in roles:
            run_role(name, principal, task, marker, pin)
        after = current_value()
        if before != after or after != {"v": 2, "status": "governed"}:
            raise RuntimeError("unauthorized Agent changed authoritative state")
        provider = value_of(PROVIDER_OBJECT) if "provider" in {role[0] for role in roles} else None
        if provider is not None and provider != {"v": 3, "status": "agent-published"}:
            raise RuntimeError(f"provider Agent published unexpected value: {provider}")
        (ARTIFACTS / "oracle.json").write_text(json.dumps({
            "seedBefore": before,
            "seedAfter": after,
            "providerPublished": provider,
            "roles": [role[0] for role in roles],
        }, indent=2) + "\n")
        (ARTIFACTS / "summary.json").write_text(json.dumps({
            "status": "PASS", "roles": [role[0] for role in roles],
        }, indent=2) + "\n")
        if len(roles) == len(ROLES):
            print("PASS: six DSH shell-only Agent roles")
        else:
            print(f"PASS: filtered DSH shell-only Agent roles ({', '.join(role[0] for role in roles)})")
        return 0
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
