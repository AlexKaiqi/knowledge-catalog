#!/usr/bin/env python3
"""Ask a real Agent natural Knowledge Catalog questions and score the answers."""

from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-questions")
MODEL_PATCH = Path(os.environ.get("DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")))
SKILL_ONLY_PATCH = PLUGIN / "scripts" / "questions-skill-only.patch.yml"
SCENARIOS = PLUGIN / "scripts" / "agent-scenarios.json"
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
        "调用 knowledge list", "使用 knowledge list", "用 knowledge list", "use knowledge list",
    )


QUESTIONS = (
    Question(
        "first-use-model",
        """我是第一次使用这个插件，不知道 KC 命令。我想找支付告警的处理方法，应该怎么开始？
插件已经给我的任务准备了什么，我可以怎样直接提问或浏览，Agent 得到搜索结果后还应做什么？
请先加载 knowledge-catalog Skill，只依据公开能力回答，不调用任何知识、文件系统或 shell 工具。
用中文给出一个最小可执行路径，不要先讲完整架构。最后只输出用清晰短段落回答。""",
        (
            ("自然语言", "直接问", "直接提问"),
            ("只读", "read-only"), ("固定", "fixed", "pin"),
            ("kc knowledge search", "search", "搜索"),
            ("kc knowledge read", "read", "读取", "正文"),
            (
                "canonical", "正文", "正式内容", "权威内容", "候选知识内容",
                "最相关内容", "知识内容",
            ),
            ("知识", "侧栏"),
            ("__sidebar_discovery_limited__",),
        ),
    ),
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
            ("不存", "不保存", "不是内容", "not a content store"), ("固定", "冻结", "immutable"),
            ("新会话", "新任务", "新的任务", "重新解析", "重新 resolve", "new session", "re-resolution"),
            ("__host_coordinates_managed__",),
            ("query", "查询", "object", "对象"),
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
            ("source key", "source-key", "source_key"), ("object_id", "object id"),
            ("路径", "path"), ("schema/*",), ("知识", "knowledge"),
            ("provider/integration", "接入侧", "集成侧", "provider 侧"),
            ("__workspace_definition_optional__",),
            ("不能替代", "不替代", "不是替代", "does not replace"),
            ("不需要 workspace", "不需要。", "无需 workspace", "无需先建", "不必先建立", "发布前不必", "no workspace", "not required"),
            ("writer",), ("commit", "propose"), ("source-ref", "provenance"),
            ("不能直接", "不能。", "不要直接", "不要绕过", "never edit", "禁止直接", "禁止绕过"),
        ),
    ),
    Question(
        "troubleshooting-model",
        """本地 SEARCH 报 CAPABILITY_UNSATISFIED，是没有匹配吗？要不要补 SQLite 或 memory，
或改用 Knowledge LIST/全仓扫描兜底？
`kc knowledge read`、`kc knowledge search`、kcfs/rg 各适合什么？另外 audit、log、provenance 有何区别；
Binding 调 live resource 后是否会自动写回知识？请先加载 knowledge-catalog Skill，只依据公开概念
回答，不调用任何知识、文件系统或 shell 工具。保留英文规范术语，用中文给出判断和恢复路径。
用清晰短段落回答。""",
        (
            ("capability_unsatisfied",), ("不是", "not"), ("__search_provider_missing__",),
            ("opensearch",),
            ("__no_public_knowledge_list__",),
            ("kcfs",), ("rg",),
            ("kc knowledge read",), ("kc knowledge search",), ("canonical",),
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
    def normalized(value: str) -> str:
        plain = value.lower().replace("`", "").replace("_", " ").replace("-", " ")
        return re.sub(r"\s+", " ", plain)

    def matches(group: tuple[str, ...], rendered: str) -> bool:
        if group == ("__sidebar_discovery_limited__",):
            return bool(
                re.search(r"(?:不一定|未必|不是|并非|不保证|不能保证|无法保证).{0,20}(?:完整|全部|全量|涵盖|发现)", rendered)
                or re.search(r"(?:not|may not|does not guarantee).{0,30}(?:complete|all|full discovery)", rendered)
            )
        if group == ("__host_coordinates_managed__",):
            has_coordinates = "catalog" in rendered and ("workspace" in rendered or "pin" in rendered)
            responsibility = bool(
                re.search(r"(?:宿主|主机|agent|运行环境|任务).{0,40}(?:提供|准备|处理|配置|固定)", rendered)
                or re.search(r"(?:不需要|无需|不用).{0,12}(?:自己)?配置", rendered)
                or re.search(r"(?:host supplies|agent handles identity)", rendered)
            )
            return has_coordinates and responsibility
        if group == ("__workspace_definition_optional__",):
            has_workspace = "workspace" in rendered
            optional = bool(
                re.search(r"(?:必要时|按需|仅在|只有|only when).{0,30}(?:定义|创建|需要|requested)", rendered)
                or re.search(r"(?:不必|无需|不需要).{0,20}(?:定义|创建|发布|workspace)", rendered)
            )
            return has_workspace and optional
        if group == ("__no_public_knowledge_list__",):
            return bool(
                re.search(r"(?:没有|不存在|无|不提供|不能).{0,16}(?:公开的?)?.{0,8}(?:knowledge )?list", rendered)
                or re.search(r"(?:no|without).{0,12}(?:public )?(?:knowledge )?list", rendered)
                or "不能枚举" in rendered
            )
        if group == ("__search_provider_missing__",):
            return bool(
                re.search(r"(?:没有|未配置|缺少|不可用).{0,20}(?:index|索引|provider|搜索能力)", rendered)
                or re.search(r"(?:index|provider).{0,16}(?:none|missing|unavailable|not configured)", rendered)
            )
        return any(normalized(term) in rendered for term in group)

    rendered = normalized(json.dumps(answer, ensure_ascii=False))
    missing = [list(group) for group in question.required if not matches(group, rendered)]
    forbidden = [term for term in question.forbidden if normalized(term) in rendered]
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
        [
            DSH_EXECUTABLE,
            "--profile", PROFILE,
            "--patch", str(MODEL_PATCH),
            "--patch", str(SKILL_ONLY_PATCH),
            question.prompt,
        ],
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
    if not SKILL_ONLY_PATCH.is_file():
        raise RuntimeError(f"missing Skill-only patch: {SKILL_ONLY_PATCH}")
    contract = json.loads(SCENARIOS.read_text())
    expected = tuple(str(name) for name in contract.get("conceptQuestions", ()))
    actual = tuple(question.name for question in QUESTIONS)
    if contract.get("version") != 1 or actual != expected or len(set(actual)) != len(actual):
        raise RuntimeError(f"Agent concept questions drifted: expected {expected}, got {actual}")
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
