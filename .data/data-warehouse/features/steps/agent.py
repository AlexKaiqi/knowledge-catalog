from __future__ import annotations

import json
import os
import subprocess
import tempfile
from pathlib import Path
from string import Template

from behave import then, when

from commands import _environment


def _trace_for(workdir: Path) -> Path:
    dsh_home = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh")))
    traces = list((dsh_home / "sessions").glob(f"*{workdir.name}*/session-*/session.jsonl.zstd"))
    if not traces:
        raise AssertionError(f"DSH produced no trace for {workdir}")
    return max(traces, key=lambda item: item.stat().st_mtime)


def _decode_trace(path: Path) -> dict:
    decoded = subprocess.run(
        ["zstd", "-dc", str(path)], capture_output=True, text=True, check=True,
    ).stdout
    tools: list[str] = []
    failed: list[str] = []
    failed_calls: list[dict[str, str]] = []
    calls: dict[str, tuple[str, str]] = {}
    loaded_skills: list[str] = []
    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        data = event.get("data") or {}
        if event.get("type") == "tool/call":
            name = str(data.get("name", "unknown"))
            tools.append(name)
            if data.get("callId"):
                calls[str(data["callId"])] = (name, str(data.get("arguments", "")))
            if name == "skill":
                try:
                    arguments = json.loads(data.get("arguments", "{}"))
                except (json.JSONDecodeError, TypeError):
                    arguments = {}
                if arguments.get("name"):
                    loaded_skills.append(str(arguments["name"]))
        if event.get("type") == "tool/result" and '"isError":true' in json.dumps(data, separators=(",", ":")):
            call_id = str((data.get("message") or {}).get("source", {}).get("callId", ""))
            name, arguments = calls.get(call_id, ("unknown", ""))
            failed.append(name)
            failed_calls.append({
                "name": name,
                "arguments": arguments,
                "result": json.dumps(data.get("message") or {}, ensure_ascii=False),
            })
    return {
        "trace": str(path),
        "tools": tools,
        "loadedSkills": loaded_skills,
        "failedTools": failed,
        "failedToolCalls": failed_calls,
        "quality": "clean" if not failed_calls else "recovered-with-tool-errors",
    }


def _fixture_view(context) -> Path:
    view = context.run / "agent-fixture"
    view.mkdir(exist_ok=True)
    for name in ("mysql", "knowledge", "connector"):
        link = view / name
        if not link.exists():
            link.symlink_to(context.fixture / name, target_is_directory=True)
    return view


@when("a first-time provider asks the DSH Agent:")
@when("a first-time consumer asks the DSH Agent:")
def ask_agent(context) -> None:
    index = len(context.agent_runs) + 1
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-dw-{index}-"))
    env = _environment(context)
    env.update({
        "FIXTURE": str(_fixture_view(context)),
        "DSH_PERMISSION_MODE": "danger-full-access",
        "KC_WORKSPACE_DIR": str(workdir),
    })
    prompt = Template(context.text).safe_substitute(env)
    executable = os.environ.get("DSH_EXECUTABLE", "dsh")
    profile = os.environ.get("DSH_PROFILE", "loom-data-warehouse")
    patch = os.environ.get(
        "DSH_MODEL_PATCH",
        str(context.repo / "dsh-plugin" / "scripts" / "deepseek-official.patch.yml"),
    )
    result = subprocess.run(
        [executable, "--profile", profile, "--patch", patch, prompt],
        cwd=workdir,
        env=env,
        capture_output=True,
        text=True,
        timeout=600,
    )
    trace = _decode_trace(_trace_for(workdir))
    evidence = context.run / "agent"
    evidence.mkdir(exist_ok=True)
    (evidence / f"{index}.stdout.txt").write_text(result.stdout, encoding="utf-8")
    (evidence / f"{index}.stderr.txt").write_text(result.stderr, encoding="utf-8")
    (evidence / f"{index}.trace.json").write_text(json.dumps(trace, ensure_ascii=False, indent=2) + "\n")
    context.agent = {
        "exitCode": result.returncode,
        "stdout": result.stdout,
        "stderr": result.stderr,
        "trace": trace,
    }
    context.agent_runs.append(context.agent)


@then("the Agent succeeds")
def agent_succeeds(context) -> None:
    assert context.agent["exitCode"] == 0, context.agent["stderr"][-4000:]
    assert context.agent["stdout"].strip(), "Agent returned an empty answer"


@then("the Agent answer contains:")
def agent_answer_contains(context) -> None:
    rendered = context.agent["stdout"].lower()
    for row in context.table:
        alternatives = [item.strip().lower() for item in row["one of"].split(";")]
        assert any(item in rendered for item in alternatives), f"answer contains none of {alternatives}"


@then("the Agent trace includes:")
def agent_trace_includes(context) -> None:
    tools = context.agent["trace"]["tools"]
    skills = context.agent["trace"]["loadedSkills"]
    for row in context.table:
        kind, name = row["kind"], row["name"]
        values = skills if kind == "skill" else tools
        assert name in values, f"trace has no {kind} {name}; got {values}"


@then("the Agent trace quality is recorded")
def agent_trace_quality_is_recorded(context) -> None:
    trace = context.agent["trace"]
    assert trace["quality"] in ("clean", "recovered-with-tool-errors")
    assert len(trace["failedTools"]) == len(trace["failedToolCalls"])
