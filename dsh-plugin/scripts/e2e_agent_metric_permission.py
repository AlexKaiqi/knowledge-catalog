#!/usr/bin/env python3
"""KC-AGENT-01: Agent tasks are the feature's Agent-as briefs, not a second script."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
ROOT = PLUGIN.parent
FEATURE_DIR = ROOT / ".data" / "scenes"


def scene_file(state: str, name: str) -> Path:
    matches = []
    for path in FEATURE_DIR.rglob(name):
        parent = path.parent
        if parent.name == state or (parent.name.startswith("_") and parent.parent.name == state):
            matches.append(path)
    if len(matches) != 1:
        raise RuntimeError(f"{state}/{name} matches={len(matches)}")
    return matches[0]


AGENT_TASK_FILES = [
    scene_file("knowledge-search-granted", "probe-declared-access.feature"),
    scene_file("knowledge-read-granted", "probe-canonical-visible.feature"),
    scene_file("principals-granted", "probe-grant-isolation.feature"),
]
SCENARIOS = PLUGIN / "scripts" / "agent-scenarios.json"
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-roles")
MODEL_PATCH = Path(os.environ.get("DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")))
DSH_EXECUTABLE = os.environ.get("DSH_EXECUTABLE", "dsh")
KC_EXECUTABLE = os.environ.get("KC_EXECUTABLE", "kc")
KC_HOME = Path(os.environ.get("KC_HOME", tempfile.mkdtemp(prefix="kc-agent-metric-home-"))).resolve()
KC_SERVER_URL = os.environ.get("KC_SERVER_URL", "")
KC_ADMIN_PRINCIPAL = os.environ.get("KC_AS", "service:agent-e2e")
ARTIFACTS = Path(os.environ.get("KC_METRIC_ARTIFACTS", tempfile.mkdtemp(prefix="kc-agent-metric-evidence-")))
SKILL_ONLY_PATCH = PLUGIN / "scripts" / "questions-skill-only.patch.yml"

CATALOG = "kr://scene/catalog"
REPO = "kr://scene/knowledge"
WORKSPACE = "scene-set"
OBJECT = "metric/gmv"
AGENT_HEADER = re.compile(r"^Agent as (\S+) \(([^)]+)\)$")
RETIRED_MODEL_TOOLS = {
    "kc", "resource", "knowledge_context", "knowledge_list", "knowledge_search",
    "knowledge_read", "knowledge_schema", "knowledge_relations", "knowledge_provenance",
}
SEARCH_ONLY_MARKERS = ("FORBIDDEN", "拒绝", "看不到", "无正文", "正文空", "屏蔽")
READ_MARKERS = ("SUM(", "l_extendedprice", "unique-measure-token-zz9", "measureKey", "公式")


def load_agent_tasks(path: Path) -> list[dict[str, str]]:
    tasks: list[dict[str, str]] = []
    in_brief = False
    buf: list[str] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped != '"""':
            if in_brief:
                buf.append(stripped)
            continue
        if not in_brief:
            in_brief = True
            buf = []
            continue
        body = "\n".join(buf).strip()
        first, _, rest = body.partition("\n")
        match = AGENT_HEADER.match(first.strip())
        if match:
            tasks.append({"principal": match.group(1), "fixture": match.group(2), "brief": rest.strip()})
        in_brief = False
        buf = []
    return tasks


def check_only() -> None:
    for path in AGENT_TASK_FILES:
        if not path.is_file():
            raise RuntimeError(f"missing feature {path}")
    schema = scene_file("drafts-ingested", "schema.metric.definition.yaml")
    instance = scene_file("semantic-knowledge-constructed", "metric.gmv.json")
    construct = scene_file("semantic-knowledge-constructed", "construct.feature")
    if REPO not in schema.read_text(encoding="utf-8") or REPO not in construct.read_text(encoding="utf-8"):
        raise RuntimeError(f"schema/construct must target {REPO}")
    for path in (schema, instance, construct):
        text = path.read_text(encoding="utf-8")
        if "kr://dw/" in text or "warehouse-agent" in text or ".data/data-warehouse" in text:
            raise RuntimeError(f"{path} still points at the warehouse suite")
    if "access: [text, filter]" not in schema.read_text(encoding="utf-8"):
        raise RuntimeError(f"{schema} is not the scene AccessHints witness")
    if "unique-measure-token-zz9" not in instance.read_text(encoding="utf-8"):
        raise RuntimeError(f"{instance} is not the scene metric witness")
    contract = json.loads(SCENARIOS.read_text(encoding="utf-8"))
    companions = {item["id"]: item for item in contract.get("extendedCompanions", [])}
    spec = companions.get("KC-AGENT-01", {}).get("spec")
    if spec != ".data/scenes":
        raise RuntimeError(f"KC-AGENT-01 spec drifted: {spec}")
    tasks: list[dict[str, str]] = []
    for path in AGENT_TASK_FILES:
        tasks.extend(load_agent_tasks(path))
    want = [
        ("bot", "search-only"),
        ("bot", "search+read"),
        ("taihu:alice", "search-only"),
    ]
    got = [(task["principal"], task["fixture"]) for task in tasks]
    if got != want:
        raise RuntimeError(f"agent tasks {got} want {want}")
    for task in tasks:
        if not task["brief"]:
            raise RuntimeError(f"empty agent brief for {task}")
    print("PASS: KC-AGENT-01 briefs stay on the scene state directories")


def kc_json(*args: str, principal: str | None = None) -> dict | list:
    if not KC_SERVER_URL:
        raise RuntimeError("KC_SERVER_URL is required; Agent acceptance only uses KC Server")
    command = [KC_EXECUTABLE, "--server", KC_SERVER_URL, "--as", principal or KC_ADMIN_PRINCIPAL, *args]
    proc = subprocess.run(command, capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"{' '.join(command)} failed: {proc.stdout or proc.stderr}")
    return json.loads(proc.stdout)


def bootstrap() -> None:
    schema = {
        "entity": "Metric", "aspect": "definition", "pattern": "record",
        "fields": {
            "name": {"type": "string", "required": True, "access": ["text", "filter"]},
            "expression": {"type": "string", "required": True, "access": ["text"]},
            "unit": {"type": "string", "access": ["filter"]},
            "measureKey": {"type": "string", "required": True},
        },
    }
    instance = json.loads(scene_file("semantic-knowledge-constructed", "metric.gmv.json").read_text(encoding="utf-8"))
    kc_json("writer", "put", "--command-id", "metric-schema", "--repo", REPO,
            "--object", "schema/metric.definition", "--value", json.dumps(schema))
    kc_json("writer", "put", "--command-id", "metric-gmv", "--repo", REPO,
            "--object", OBJECT, "--aspect", "definition",
            "--schema-ref", "schema/metric.definition", "--value", json.dumps(instance))
    kc_json("catalog", "workspace", "define", "--workspace", WORKSPACE, "--revision", "1",
            "--source", f"{REPO}=refs/heads/main")
    kc_json("admin", "grant", "add", "--principal", "bot", "--action", "knowledge.search", "--repo", REPO)
    kc_json("admin", "grant", "add", "--principal", "taihu:alice", "--action", "workspace.consume",
            "--catalog", CATALOG, "--workspace", WORKSPACE)
    kc_json("admin", "grant", "add", "--principal", "taihu:alice", "--action", "knowledge.search", "--repo", REPO)
    kc_json("operations", "projection", "sync", "--repo", REPO)


def apply_fixture(fixture: str, principal: str) -> None:
    if fixture == "search+read":
        kc_json("admin", "grant", "add", "--principal", principal, "--action", "knowledge.read", "--repo", REPO)


def task_context(name: str, principal: str, workdir: Path, pin: dict | list) -> None:
    directory = KC_HOME / "tasks" / name
    directory.mkdir(parents=True, exist_ok=True)
    context = {
        "version": 1, "sessionId": name, "principal": principal,
        "catalog": CATALOG, "workspace": WORKSPACE, "pin": pin,
        "root": str(workdir), "readOnly": True, "mounts": [],
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


def run_task(index: int, task: dict[str, str], pin: dict | list) -> None:
    name = f"{index}-{task['principal']}-{task['fixture']}"
    apply_fixture(task["fixture"], task["principal"])
    prompt = (
        "Load and follow the bundled knowledge-catalog skill. Through the shell, "
        "use grouped `kc` commands. Do not edit Repository files or call git.\n\n"
        + task["brief"]
    )
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-metric-{name}-")).resolve()
    task_context(name, task["principal"], workdir, pin)
    env = os.environ.copy()
    env.update({
        "DSH_PERMISSION_MODE": "danger-full-access", "KC_HOME": str(KC_HOME),
        "KC_SERVER_URL": KC_SERVER_URL, "KC_WORKSPACE": WORKSPACE, "KC_AS": task["principal"],
    })
    proc = subprocess.run(
        [DSH_EXECUTABLE, "--profile", PROFILE, "--patch", str(MODEL_PATCH), "--patch", str(SKILL_ONLY_PATCH), prompt],
        cwd=workdir, env=env, capture_output=True, text=True, timeout=480,
    )
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / f"{name}.stdout.txt").write_text(proc.stdout)
    (ARTIFACTS / f"{name}.stderr.txt").write_text(proc.stderr)
    markers = READ_MARKERS if task["fixture"] == "search+read" else SEARCH_ONLY_MARKERS
    if proc.returncode != 0 or not any(marker in proc.stdout for marker in markers):
        raise RuntimeError(f"{name} failed; expected one of {markers}; stderr:\n{proc.stderr[-4000:]}")
    verify_trace(name, workdir)


def live() -> None:
    if not MODEL_PATCH.is_file() or not SKILL_ONLY_PATCH.is_file():
        raise RuntimeError("missing model or skill-only patch")
    check_only()
    bootstrap()
    pin = kc_json("catalog", "workspace", "resolve", "--workspace", WORKSPACE)
    tasks: list[dict[str, str]] = []
    for path in AGENT_TASK_FILES:
        tasks.extend(load_agent_tasks(path))
    for index, task in enumerate(tasks):
        run_task(index, task, pin)
    print("PASS: KC-AGENT-01 DSH companion used feature briefs")


def main() -> int:
    try:
        if "--check-only" in sys.argv:
            check_only()
            return 0
        live()
        return 0
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
