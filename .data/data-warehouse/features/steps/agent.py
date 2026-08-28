from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
from collections import Counter
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
    event_times: list[int] = []
    step_starts: dict[tuple[int, int], int] = {}
    tool_starts: dict[str, int] = {}
    model_duration_ms = 0
    tool_duration_ms = 0
    model_steps = 0
    token_usage: Counter[str] = Counter()
    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        data = event.get("data") or {}
        event_time = event.get("time") or event.get("createdAt")
        if isinstance(event_time, int):
            event_times.append(event_time)
        step_key = (data.get("turn"), data.get("step"))
        if event.get("type") == "step/start" and isinstance(event_time, int):
            step_starts[step_key] = event_time
        if event.get("type") == "assistant/message" and isinstance(event_time, int):
            started = step_starts.get(step_key)
            if started is not None:
                model_duration_ms += event_time - started
            model_steps += 1
            for name, value in (data.get("usage") or {}).items():
                if isinstance(value, int):
                    token_usage[str(name)] += value
        if event.get("type") == "tool/call":
            name = str(data.get("name", "unknown"))
            tools.append(name)
            if data.get("callId"):
                calls[str(data["callId"])] = (name, str(data.get("arguments", "")))
                if isinstance(event_time, int):
                    tool_starts[str(data["callId"])] = event_time
            try:
                arguments = json.loads(data.get("arguments", "{}"))
            except (json.JSONDecodeError, TypeError):
                arguments = {}
            if name == "skill":
                if arguments.get("name"):
                    loaded_skills.append(str(arguments["name"]))
        if event.get("type") == "tool/result":
            call_id = str((data.get("message") or {}).get("source", {}).get("callId", ""))
            if isinstance(event_time, int) and call_id in tool_starts:
                tool_duration_ms += event_time - tool_starts[call_id]
            if '"isError":true' not in json.dumps(data, separators=(",", ":")):
                continue
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
        "metrics": {
            "durationSeconds": round((max(event_times) - min(event_times)) / 1000, 3) if event_times else 0,
            "modelSteps": model_steps,
            "toolCalls": len(tools),
            "modelDurationSeconds": round(model_duration_ms / 1000, 3),
            "toolDurationSeconds": round(tool_duration_ms / 1000, 3),
            "toolCounts": dict(Counter(tools)),
            "tokens": dict(token_usage),
        },
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
    command = [executable, "--profile", profile, "--patch", patch, prompt]
    try:
        result = subprocess.run(
            command,
            cwd=workdir,
            env=env,
            capture_output=True,
            text=True,
            timeout=600,
        )
    except subprocess.TimeoutExpired as error:
        def decoded(value) -> str:
            if value is None:
                return ""
            return value.decode(errors="replace") if isinstance(value, bytes) else str(value)

        result = subprocess.CompletedProcess(
            command,
            124,
            stdout=decoded(error.stdout),
            stderr=decoded(error.stderr) + "\nDSH Agent timed out after 600 seconds\n",
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
    def normalized(value: str) -> str:
        plain = value.lower().translate(str.maketrans("", "", "*_`"))
        return re.sub(r"\s*[:：=]\s*", ":", plain)

    rendered = normalized(context.agent["stdout"])
    for row in context.table:
        alternatives = [normalized(item.strip()) for item in row["one of"].split(";")]
        assert any(item in rendered for item in alternatives), f"answer contains none of {alternatives}"


@then("the Agent trace includes:")
def agent_trace_includes(context) -> None:
    tools = context.agent["trace"]["tools"]
    skills = context.agent["trace"]["loadedSkills"]
    for row in context.table:
        kind, name = row["kind"], row["name"]
        values = skills if kind == "skill" else tools
        assert name in values, f"trace has no {kind} {name}; got {values}"


@then("the Agent trace excludes retired KC model tools")
def agent_trace_excludes_retired_tools(context) -> None:
    retired = {
        "kc", "resource", "knowledge_context", "knowledge_list", "knowledge_search",
        "knowledge_read", "knowledge_schema", "knowledge_relations", "knowledge_provenance",
    }
    present = retired.intersection(context.agent["trace"]["tools"])
    assert not present, f"retired KC model tools appeared in trace: {sorted(present)}"


@then("the Agent trace quality is recorded")
def agent_trace_quality_is_recorded(context) -> None:
    trace = context.agent["trace"]
    assert trace["quality"] in ("clean", "recovered-with-tool-errors")
    assert len(trace["failedTools"]) == len(trace["failedToolCalls"])
    assert trace["metrics"]["modelSteps"] > 0
    assert trace["metrics"]["toolCalls"] == len(trace["tools"])


@then('the Agent trace stays within the "{journey}" quality budget')
def agent_trace_stays_within_budget(context, journey: str) -> None:
    trace = context.agent["trace"]
    metrics = trace["metrics"]
    agent_trace_excludes_retired_tools(context)
    assert "integration-development" not in trace["loadedSkills"]
    assert "knowledge_search" not in trace["tools"]
    assert "create_goal" not in trace["tools"]
    assert "update_goal" not in trace["tools"]
    if journey == "provider":
        assert len(trace["failedToolCalls"]) <= 2, trace["failedToolCalls"]
        assert metrics["modelSteps"] <= 60, metrics
        assert metrics["toolCalls"] <= 60, metrics
        assert "read_image" not in trace["tools"], trace["tools"]
        return
    if journey == "consumer":
        assert trace["quality"] == "clean", trace["failedToolCalls"]
        assert metrics["modelSteps"] <= 20, metrics
        assert metrics["toolCalls"] <= 20, metrics
        return
    raise AssertionError(f"unknown Agent journey budget {journey}")
