#!/usr/bin/env python3
"""Ask a real Agent natural Knowledge Catalog questions and score the answers."""

from __future__ import annotations

import json
import os
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-questions")
MODEL_PATCH = Path(os.environ.get("DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")))
DSH_EXECUTABLE = os.environ.get("DSH_EXECUTABLE", "dsh")
ARTIFACTS = Path(os.environ.get("KC_QUESTION_ARTIFACTS", tempfile.mkdtemp(prefix="kc-agent-question-evidence-")))


@dataclass(frozen=True)
class Question:
    name: str
    prompt: str
    required: tuple[tuple[str, ...], ...]
    forbidden: tuple[str, ...] = (
        "调用 knowledge_log", "使用 knowledge_log", "用 knowledge_log", "use knowledge_log",
        "调用 knowledge_audit", "使用 knowledge_audit", "用 knowledge_audit", "use knowledge_audit",
    )


QUESTIONS = (
    Question(
        "consumer-model",
        """我是第一次使用。Catalog、Repository、Workspace、ResolvedWorkspace/pin 到底分别是什么？
如果上游在我分析到一半时提交了新版本，我前后两次读取会不会悄悄变化？Agent 会替我处理什么，
我还需要提供什么？请先加载 knowledge-catalog Skill，只依据公开概念回答，不调用任何知识、
文件系统或 shell 工具。保留英文规范术语，用中文给出结论、原因和最小下一步。最后只输出
用清晰短段落回答。""",
        (
            ("catalog",), ("repository",), ("workspace",),
            ("resolvedworkspace", "pin"), ("commit",),
            ("不存", "不是内容", "not a content store"), ("固定", "冻结", "immutable"),
            ("新会话", "重新解析", "new session", "re-resolution"),
            ("identity", "身份"), ("query", "查询", "object", "对象"),
        ),
    ),
    Question(
        "provider-model",
        """我要把数据库说明、字段 schema 和源系统 ID 接入。source key、object_id、文件路径
应该怎么区分、映射表放在哪，provenance 能否替代它？schema 是不是放进项目源码？发布前必须先建
Workspace 吗？能否直接改仓库里的文件或 git？怎样发布并留下来源？
请先加载 knowledge-catalog Skill，只依据公开概念回答，不调用任何知识、文件系统或 shell 工具。
保留英文规范术语，用中文给出推荐流程和禁止事项。最后只输出
用清晰短段落回答。""",
        (
            ("source key", "source-key"), ("object_id", "object id"),
            ("路径", "path"), ("schema/*",), ("知识", "knowledge"),
            ("provider/integration", "接入侧", "集成侧", "provider 侧"),
            ("不自动", "不会自动", "不必发布", "无需发布", "only when", "仅在"),
            ("不能替代", "不替代", "不是替代", "does not replace"),
            ("不需要 workspace", "不需要。", "无需 workspace", "无需先建", "no workspace", "not required"),
            ("writer",), ("commit", "propose"), ("source-ref", "provenance"),
            ("不能直接", "不能。", "不要直接", "never edit", "禁止直接", "禁止绕过"),
        ),
    ),
    Question(
        "troubleshooting-model",
        """本地 SEARCH 报 CAPABILITY_UNSATISFIED，是没有匹配吗？要不要补 SQLite 或 memory？
knowledge_read、knowledge_search、kcfs/rg 各适合什么？另外 audit、log、provenance 有何区别；
Binding 调 live resource 后是否会自动写回知识？请先加载 knowledge-catalog Skill，只依据公开概念
回答，不调用任何知识、文件系统或 shell 工具。保留英文规范术语，用中文给出判断和恢复路径。
用清晰短段落回答。""",
        (
            ("capability_unsatisfied",), ("不是", "not"), ("index: none", "index:none"),
            ("opensearch",), ("knowledge_list",), ("kcfs",), ("rg",),
            ("knowledge_read",), ("knowledge_search",), ("canonical",),
            ("audit",), ("log",), ("provenance",), ("binding",),
            ("collector",), ("commit",),
        ),
    ),
)


def decode_answer(output: str) -> dict:
    decoder = json.JSONDecoder()
    candidates: list[dict] = []
    for index, character in enumerate(output):
        if character != "{":
            continue
        try:
            value, _ = decoder.raw_decode(output[index:])
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict) and isinstance(value.get("answer"), str):
            candidates.append(value)
    if candidates:
        return candidates[-1]
    rendered = output.strip()
    if not rendered:
        raise RuntimeError("Agent returned an empty answer")
    return {"answer": rendered}


def trace_for(workdir: Path) -> Path:
    dsh_home = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh")))
    traces = list((dsh_home / "sessions").glob(f"*{workdir.name}*/session-*/session.jsonl.zstd"))
    if not traces:
        raise RuntimeError("Agent produced no DSH session trace")
    return max(traces, key=lambda path: path.stat().st_mtime)


def verify_trace(question: Question, workdir: Path) -> dict:
    trace = trace_for(workdir)
    decoded = subprocess.run(["zstd", "-dc", str(trace)], capture_output=True, text=True, check=True).stdout
    loaded = False
    other_tools: list[str] = []
    tool_errors: list[str] = []
    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "tool/result" and '"isError":true' in json.dumps(event.get("data", {}), separators=(",", ":")):
            tool_errors.append("tool/result")
        if event.get("type") != "tool/call":
            continue
        data = event.get("data", {})
        name = str(data.get("name", "unknown"))
        if name != "skill":
            other_tools.append(name)
            continue
        try:
            arguments = json.loads(data.get("arguments", "{}"))
        except (json.JSONDecodeError, TypeError):
            continue
        loaded = loaded or arguments.get("name") == "knowledge-catalog"
    evidence = {
        "trace": str(trace),
        "knowledgeCatalogSkillLoaded": loaded,
        "nonSkillToolCalls": other_tools,
        "toolErrors": tool_errors,
    }
    if not loaded:
        raise RuntimeError(f"{question.name}: bundled knowledge-catalog Skill was not loaded")
    if other_tools:
        raise RuntimeError(f"{question.name}: concept answer used unrelated tools: {other_tools}")
    if tool_errors:
        raise RuntimeError(f"{question.name}: tool errors occurred")
    return evidence


def score(question: Question, answer: dict) -> dict:
    rendered = json.dumps(answer, ensure_ascii=False).lower()
    missing = [list(group) for group in question.required if not any(term.lower() in rendered for term in group)]
    forbidden = [term for term in question.forbidden if term.lower() in rendered]
    evidence = {
        "requiredGroups": [list(group) for group in question.required],
        "missingGroups": missing,
        "forbiddenTerms": forbidden,
    }
    if missing:
        raise RuntimeError(f"{question.name}: answer missed semantic groups: {missing}")
    if forbidden:
        raise RuntimeError(f"{question.name}: answer invented unsupported tools: {forbidden}")
    return evidence


def run(question: Question) -> None:
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-question-{question.name}-"))
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    proc = subprocess.run(
        [DSH_EXECUTABLE, "--profile", PROFILE, "--patch", str(MODEL_PATCH), question.prompt],
        cwd=workdir,
        env=env,
        capture_output=True,
        text=True,
        timeout=300,
    )
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / f"{question.name}.stdout.txt").write_text(proc.stdout)
    (ARTIFACTS / f"{question.name}.stderr.txt").write_text(proc.stderr)
    if proc.returncode != 0:
        raise RuntimeError(f"{question.name}: Agent exited {proc.returncode}: {proc.stderr[-2000:]}")
    answer = decode_answer(proc.stdout)
    trace = verify_trace(question, workdir)
    oracle = score(question, answer)
    (ARTIFACTS / f"{question.name}.answer.json").write_text(json.dumps(answer, ensure_ascii=False, indent=2) + "\n")
    (ARTIFACTS / f"{question.name}.trace.json").write_text(json.dumps(trace, indent=2) + "\n")
    (ARTIFACTS / f"{question.name}.oracle.json").write_text(json.dumps(oracle, ensure_ascii=False, indent=2) + "\n")
    print(f"{question.name}: PASS")


def main() -> int:
    if not MODEL_PATCH.is_file():
        raise RuntimeError(f"missing model patch: {MODEL_PATCH}")
    selected_names = {name.strip() for name in os.environ.get("KC_QUESTION_FILTER", "").split(",") if name.strip()}
    selected = tuple(question for question in QUESTIONS if not selected_names or question.name in selected_names)
    unknown = selected_names - {question.name for question in QUESTIONS}
    if unknown:
        raise RuntimeError(f"unknown KC_QUESTION_FILTER names: {sorted(unknown)}")
    if not selected:
        raise RuntimeError("KC_QUESTION_FILTER selected no questions")
    for question in selected:
        run(question)
    (ARTIFACTS / "summary.json").write_text(json.dumps({"status": "PASS", "questions": [q.name for q in selected]}, indent=2) + "\n")
    print("PASS: Agent concept answers, entry selection, and recovery guidance")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
