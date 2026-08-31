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

CATALOG = "kr://dw/catalog"
WORKSPACE = "warehouse-agent"
PHYSICAL_REPO = "kr://dw/physical"
SEMANTIC_REPO = "kr://dw/semantic"
CONSUMER_PRINCIPAL = "agent:dw-consumer"


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
    tool_calls: list[dict[str, str]] = []
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
            tool_calls.append({"name": name, "arguments": str(data.get("arguments", ""))})
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
            name, arguments = calls.get(call_id, ("unknown", ""))
            rendered_result = json.dumps(data.get("message") or {}, ensure_ascii=False)
            tool_failed = '"isError":true' in json.dumps(data, separators=(",", ":"))
            shell_failed = name in {"bash", "shell"} and bool(
                re.search(r"\[exit code:\s*[1-9][0-9]*\]", rendered_result)
            )
            if not tool_failed and not shell_failed:
                continue
            failed.append(name)
            failed_calls.append({
                "name": name,
                "arguments": arguments,
                "result": rendered_result,
            })
    return {
        "trace": str(path),
        "tools": tools,
        "toolCalls": tool_calls,
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


def _kc_json(context, *args: str) -> dict:
    result = subprocess.run(
        [str(context.kc), "--home", str(context.home), *args],
        cwd=context.repo,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise AssertionError(f"host KC setup failed: {result.stdout or result.stderr}")
    return json.loads(result.stdout)


def _prepare_consumer_context(context, workdir: Path) -> None:
    _kc_json(
        context, "admin", "grant", "add", "--principal", CONSUMER_PRINCIPAL,
        "--action", "workspace.consume", "--catalog", CATALOG, "--workspace", WORKSPACE,
    )
    for repository in (PHYSICAL_REPO, SEMANTIC_REPO):
        _kc_json(
            context, "admin", "grant", "add", "--principal", CONSUMER_PRINCIPAL,
            "--action", "knowledge.read,knowledge.search,knowledge.provenance", "--repo", repository,
        )
    _kc_json(
        context, "admin", "grant", "add", "--principal", CONSUMER_PRINCIPAL,
        "--action", "resource.access", "--catalog", CATALOG, "--workspace", WORKSPACE,
    )
    pin = _kc_json(
        context, "catalog", "workspace", "resolve", "--catalog", CATALOG,
        "--workspace", WORKSPACE,
    )
    task = context.home / "tasks" / "dw-agent-consumer"
    task.mkdir(parents=True, exist_ok=True)
    task_context = {
        "version": 1,
        "sessionId": "dw-agent-consumer",
        "principal": CONSUMER_PRINCIPAL,
        "catalog": CATALOG,
        "workspace": WORKSPACE,
        "pin": pin,
        "root": str(workdir),
        "readOnly": True,
        "mounts": [],
    }
    (task / "context.json").write_text(
        json.dumps(task_context, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )


@when("a provider asks the DSH Agent to preview synchronization:")
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
    if index == 2:
        _prepare_consumer_context(context, workdir)
    prompt = Template(context.text).safe_substitute(env)
    executable = os.environ.get("DSH_EXECUTABLE", "dsh")
    profile = os.environ.get("DSH_PROFILE", "loom-data-warehouse")
    patch = os.environ.get(
        "DSH_MODEL_PATCH",
        str(context.repo / "dsh-plugin" / "scripts" / "deepseek-official.patch.yml"),
    )
    skill_only_patch = context.repo / "dsh-plugin" / "scripts" / "questions-skill-only.patch.yml"
    command = [
        executable, "--profile", profile, "--patch", patch,
        "--patch", str(skill_only_patch), prompt,
    ]
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
    evidence = context.run / "agent"
    evidence.mkdir(exist_ok=True)
    (evidence / f"{index}.stdout.txt").write_text(result.stdout, encoding="utf-8")
    (evidence / f"{index}.stderr.txt").write_text(result.stderr, encoding="utf-8")
    try:
        trace = _decode_trace(_trace_for(workdir))
    except AssertionError as trace_error:
        detail = (result.stderr or result.stdout).strip()[-4000:]
        raise AssertionError(
            f"DSH exited {result.returncode} without a trace for {workdir}:\n{detail}"
        ) from trace_error
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


@then("the Agent shell trace contains:")
def agent_shell_trace_contains(context) -> None:
    rendered = "\n".join(
        call["arguments"] for call in context.agent["trace"]["toolCalls"]
        if call["name"] in {"bash", "shell"}
    )
    for row in context.table:
        needle = row["text"]
        assert needle in rendered, f"Agent shell trace does not contain {needle!r}"


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
        assert trace["quality"] == "clean", trace["failedToolCalls"]
        allowed = {"skill", "bash", "shell", "read", "grep", "glob"}
        unexpected = sorted(set(trace["tools"]) - allowed)
        assert not unexpected, f"provider used unrelated tools: {unexpected}"
        assert metrics["modelSteps"] <= 15, metrics
        assert metrics["toolCalls"] <= 15, metrics
        return
    if journey == "consumer":
        assert trace["quality"] == "clean", trace["failedToolCalls"]
        unexpected = sorted(set(trace["tools"]) - {"skill", "bash", "shell", "todo_write"})
        assert not unexpected, f"consumer used unrelated tools: {unexpected}"
        assert metrics["modelSteps"] <= 20, metrics
        assert metrics["toolCalls"] <= 20, metrics
        return
    raise AssertionError(f"unknown Agent journey budget {journey}")
