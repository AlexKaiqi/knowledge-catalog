#!/usr/bin/env python3
"""Performance scene tree: read, project and check .data/scale/scenes.

与 .data/scenes/tree.py 同一精神：目录是唯一结构来源，标签就近声明，工具只投影与
检查，不保存第二份树。压测树回答容量与时延问题；协议行为归协议场景树。
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
META = "_meta.yaml"
CONSTRUCT = "_build/construct.yaml"
PROBES = "_probes"
VIEWS = "_views.yaml"
METRICS = "metrics.yaml"
ENV_EXAMPLE = "env.example.yaml"

SCALES = ["S0", "S1", "S2", "S3", "S4", "S5"]
HISTORIES = ["H0", "H1", "H2", "H3", "H4"]

# runner 能力词汇表（scenes/README.md §4）
RUNNER_CAPABILITIES = {
    "arrival-models", "long-window", "fault-injection", "history-generation",
    "checkpoint-resume", "multi-repo", "resource-sampling", "server-metrics",
}
# 探针 isolation 词汇表
ISOLATION_MODES = {"read-only", "per-run", "per-point-scratch-repository"}
CLEANUP_REQUIRED = {"per-run", "per-point-scratch-repository"}
# 指标 dims 的定性维度；其余 dim 必须是 env 阶梯名
QUALITATIVE_DIMS = {
    "scaleProfile", "historyDepth", "phase", "coldHot", "concurrency",
    "pageSize", "fileCursor", "queryType", "limit", "refreshInterval",
    "lagDepth", "failureMode", "eventType",
}


# ---------------------------------------------------------------- YAML 子集
def load_yaml_file(path: Path):
    return load_yaml(path.read_text(encoding="utf-8"))


def load_yaml(text: str):
    lines = []
    for no, raw in enumerate(text.splitlines(), 1):
        stripped = _strip_comment(raw).rstrip()
        if not stripped.strip():
            continue
        if "\t" in stripped:
            raise ValueError(f"line {no}: tabs are not allowed")
        indent = len(stripped) - len(stripped.lstrip(" "))
        lines.append((no, indent, stripped.strip()))
    if not lines:
        return None
    value, i = _parse_block(lines, 0, lines[0][1])
    if i != len(lines):
        raise ValueError(f"line {lines[i][0]}: leftover content after block")
    return value


def _strip_comment(raw: str) -> str:
    out = []
    quote = None
    for ch in raw:
        if quote:
            out.append(ch)
            if ch == quote:
                quote = None
            continue
        if ch in "\"'":
            quote = ch
            out.append(ch)
            continue
        if ch == "#" and (not out or out[-1] == " "):
            break
        out.append(ch)
    return "".join(out)


def _parse_block(lines, i, indent):
    if lines[i][2].startswith("- "):
        return _parse_list(lines, i, indent)
    return _parse_map(lines, i, indent)


def _parse_map(lines, i, indent):
    result = {}
    while i < len(lines):
        no, ind, content = lines[i]
        if ind < indent:
            break
        if ind > indent:
            raise ValueError(f"line {no}: unexpected deeper indent")
        if content.startswith("- "):
            raise ValueError(f"line {no}: list item where map key expected")
        if ":" not in content:
            raise ValueError(f"line {no}: expected 'key: value'")
        key, rest = content.split(":", 1)
        key = key.strip()
        rest = rest.strip()
        if not key:
            raise ValueError(f"line {no}: empty key")
        if rest:
            result[key] = _parse_scalar(rest)
            i += 1
        else:
            i += 1
            if i < len(lines) and lines[i][1] > indent:
                value, i = _parse_block(lines, i, lines[i][1])
                result[key] = value
            else:
                result[key] = None
    return result, i


def _parse_list(lines, i, indent):
    items = []
    while i < len(lines):
        no, ind, content = lines[i]
        if ind != indent or not content.startswith("- "):
            if ind > indent:
                raise ValueError(f"line {no}: unexpected deeper indent in list")
            break
        items.append(_parse_scalar(content[2:].strip()))
        i += 1
    return items, i


def _parse_scalar(text: str):
    text = text.strip()
    if text.startswith("[") and text.endswith("]"):
        inner = text[1:-1].strip()
        if not inner:
            return []
        return [_parse_scalar(part.strip()) for part in _split_commas(inner)]
    if len(text) >= 2 and text[0] in "\"'" and text[-1] == text[0]:
        return text[1:-1]
    if text in ("null", "~", ""):
        return None
    if text in ("true", "True"):
        return True
    if text in ("false", "False"):
        return False
    try:
        return int(text)
    except ValueError:
        pass
    try:
        return float(text)
    except ValueError:
        pass
    return text


def _split_commas(text: str):
    parts, buf, quote = [], [], None
    for ch in text:
        if quote:
            buf.append(ch)
            if ch == quote:
                quote = None
            continue
        if ch in "\"'":
            quote = ch
            buf.append(ch)
            continue
        if ch == ",":
            parts.append("".join(buf))
            buf = []
            continue
        buf.append(ch)
    if buf:
        parts.append("".join(buf))
    return parts


# ---------------------------------------------------------------- 树模型
def state_dirs():
    dirs = []
    for path in sorted(ROOT.rglob("_meta.yaml")):
        dirs.append(path.parent)
    return dirs


def is_state(path: Path) -> bool:
    return path.is_dir() and (path / META).is_file()


def build_model():
    views_doc = load_yaml_file(ROOT / VIEWS) or {}
    views = views_doc.get("views") or {}
    metrics_doc = load_yaml_file(ROOT / METRICS) or {}
    families = metrics_doc.get("families") or {}
    registry = {}
    for family, body in families.items():
        for metric_id, spec in ((body or {}).get("metrics") or {}).items():
            registry[metric_id] = dict(spec or {}, family=family)
    env = load_yaml_file(ROOT / ENV_EXAMPLE) or {}
    ladders = ((env.get("profile") or {}).get("ladders")) or {}

    states = []
    for directory in state_dirs():
        parent = directory.parent if is_state(directory.parent) else None
        meta = load_yaml_file(directory / META) or {}
        construct = load_yaml_file(directory / CONSTRUCT) if (directory / CONSTRUCT).is_file() else None
        probes = []
        probe_dir = directory / PROBES
        if probe_dir.is_dir():
            for probe_file in sorted(probe_dir.glob("*.yaml")):
                probes.append({
                    "file": probe_file.name,
                    "id": probe_file.stem,
                    "spec": load_yaml_file(probe_file) or {},
                })
        rel = directory.relative_to(ROOT).as_posix()
        states.append({
            "id": rel,
            "dir": directory,
            "parent": parent.relative_to(ROOT).as_posix() if parent else None,
            "meta": meta,
            "construct": construct,
            "probes": probes,
        })
    states.sort(key=lambda s: s["id"])
    return {
        "views": views,
        "metrics": registry,
        "families": families,
        "ladders": ladders,
        "states": states,
    }


# ---------------------------------------------------------------- 检查
def check(model):
    errors = []
    warnings = []
    views = set(model["views"])
    metrics = model["metrics"]
    ladders = set(model["ladders"])
    if not views:
        errors.append("_views.yaml declares no views")
    if not metrics:
        errors.append("metrics.yaml registers no metrics")

    used_ladders = set()
    for metric_id, spec in metrics.items():
        for dim in spec.get("dims") or []:
            if dim in QUALITATIVE_DIMS or dim in ladders:
                if dim in ladders:
                    used_ladders.add(dim)
            else:
                errors.append(f"metric {metric_id}: unknown dim {dim!r}")
        if not spec.get("unit"):
            errors.append(f"metric {metric_id}: missing unit")
        if spec.get("collect") not in {"client-timer", "server-metrics", "ops-api", "resource-sample", "derived"}:
            errors.append(f"metric {metric_id}: unknown collect {spec.get('collect')!r}")

    for state in model["states"]:
        sid = state["id"]
        meta = state["meta"]
        for field in ("title", "fixture"):
            if not str(meta.get(field) or "").strip():
                errors.append(f"{sid}: _meta.yaml missing {field}")
        node_views = meta.get("views") or []
        if not node_views:
            errors.append(f"{sid}: no views declared")
        for view in node_views:
            if view not in views:
                errors.append(f"{sid}: unknown view {view!r}")
        for capability in meta.get("requires") or []:
            if capability not in RUNNER_CAPABILITIES:
                errors.append(f"{sid}: unknown runner capability {capability!r}")
        for metric_id in meta.get("metrics") or []:
            if metric_id not in metrics:
                errors.append(f"{sid}: unknown metric {metric_id!r}")

        construct = state["construct"]
        if construct is None:
            errors.append(f"{sid}: missing _build/construct.yaml")
        else:
            if not str(construct.get("summary") or "").strip():
                errors.append(f"{sid}: construct missing summary")
            steps = construct.get("steps")
            if not isinstance(steps, dict) or not steps:
                errors.append(f"{sid}: construct has no steps")
            else:
                for step_id, step in steps.items():
                    if not isinstance(step, dict) or not step.get("surface") or not step.get("action"):
                        errors.append(f"{sid}: construct step {step_id} needs surface and action")
            for capability in construct.get("requires") or []:
                if capability not in RUNNER_CAPABILITIES:
                    errors.append(f"{sid}: construct unknown capability {capability!r}")
            for metric_id in construct.get("metrics") or []:
                if metric_id not in metrics:
                    errors.append(f"{sid}: construct unknown metric {metric_id!r}")

        if not state["probes"] and state["parent"] is None:
            warnings.append(f"{sid}: root state declares no probes")
        for probe in state["probes"]:
            spec = probe["spec"]
            where = f"{sid}/{probe['file']}"
            if probe["id"] != str(spec.get("id") or ""):
                errors.append(f"{where}: id does not match filename")
            if not str(spec.get("question") or "").strip():
                errors.append(f"{where}: missing question")
            probe_views = spec.get("views") or []
            if not probe_views:
                errors.append(f"{where}: no views declared")
            for view in probe_views:
                if view not in views:
                    errors.append(f"{where}: unknown view {view!r}")
            ladder = spec.get("ladder")
            if ladder is not None:
                if ladder not in ladders:
                    errors.append(f"{where}: unknown ladder {ladder!r}")
                else:
                    used_ladders.add(ladder)
            probe_metrics = spec.get("metrics") or []
            if not probe_metrics:
                errors.append(f"{where}: no metrics declared")
            for metric_id in probe_metrics:
                if metric_id not in metrics:
                    errors.append(f"{where}: unknown metric {metric_id!r}")
            isolation = spec.get("operation", {}).get("isolation") if isinstance(spec.get("operation"), dict) else None
            if isolation is not None and isolation not in ISOLATION_MODES:
                errors.append(f"{where}: unknown isolation {isolation!r}")
            cleanup = str(spec.get("cleanup") or "").strip()
            if isolation in CLEANUP_REQUIRED:
                if not cleanup:
                    errors.append(f"{where}: isolation {isolation} requires cleanup")
                elif cleanup.startswith("无临时资源"):
                    errors.append(f"{where}: isolation {isolation} contradicts cleanup text")
            if not (spec.get("correctness") or []):
                errors.append(f"{where}: correctness list is empty")
            for capability in spec.get("requires") or []:
                if capability not in RUNNER_CAPABILITIES:
                    errors.append(f"{where}: unknown runner capability {capability!r}")

    for ladder in sorted(ladders - used_ladders):
        warnings.append(f"ladder {ladder!r} declared in env.example but referenced by no probe")

    gaps = [(s["id"], g) for s in model["states"] for g in (s["meta"].get("gaps") or [])]
    return {"errors": errors, "warnings": warnings, "gaps": gaps, "used_ladders": used_ladders}


# ---------------------------------------------------------------- 投影
def text_tree(model, view=None):
    lines = []
    states = model["states"]
    by_id = {state["id"]: state for state in states}
    children = {}
    roots = []
    for state in states:
        if state["parent"] and state["parent"] in by_id:
            children.setdefault(state["parent"], []).append(state)
        elif state["parent"] is None:
            roots.append(state)
        else:
            lines.append(f"orphan state: {state['id']} (parent {state['parent']} not found)")

    def emit(state, depth):
        meta = state["meta"]
        gap = " [gap]" if meta.get("gaps") else ""
        lines.append(f"{'  ' * depth}{state['dir'].name}{gap} — {meta.get('title', '')}")
        construct = state["construct"] or {}
        if construct.get("summary"):
            lines.append(f"{'  ' * (depth + 1)}build: {construct['summary']}")
        for probe in state["probes"]:
            spec = probe["spec"]
            if view is not None and view not in (spec.get("views") or []):
                continue
            ladder = f" × {spec['ladder']}" if spec.get("ladder") else ""
            caps = f" (requires: {', '.join(spec.get('requires') or [])})" if spec.get("requires") else ""
            lines.append(f"{'  ' * (depth + 1)}{probe['id']}{ladder}{caps}")
            for metric_id in spec.get("metrics") or []:
                lines.append(f"{'  ' * (depth + 2)}· {metric_id}")
        for child in children.get(state["id"], []):
            emit(child, depth + 1)

    for root in roots:
        emit(root, 0)
    return "\n".join(lines)


def text_metrics(model):
    lines = []
    for family, body in (model.get("families") or {}).items():
        lines.append(f"## {family} — {body.get('title', '')}")
        for metric_id, spec in (body.get("metrics") or {}).items():
            dims = ", ".join(spec.get("dims") or []) or "—"
            gate = f" | gate: {spec['gateRef']}" if spec.get("gateRef") else ""
            note = f" | {spec['note']}" if spec.get("note") else ""
            lines.append(f"  {metric_id}  [{spec.get('unit', '?')}] dims: {dims} ({spec.get('collect', '?')}){gate}{note}")
        lines.append("")
    return "\n".join(lines)


def check_env(path: Path):
    errors = []
    doc = load_yaml_file(path)
    if not isinstance(doc, dict):
        return [f"{path}: not a mapping"], {}
    for key in ("run", "target", "profile", "load", "limits", "cleanup"):
        if key not in doc:
            errors.append(f"missing section {key}")
    target = doc.get("target") or {}
    profile = doc.get("profile") or {}
    load = doc.get("load") or {}
    cleanup = doc.get("cleanup") or {}
    url = str(target.get("serverURL") or "")
    if not re.match(r"^https?://", url):
        errors.append("target.serverURL must be an http(s) URL of an already-deployed KC server")
    credentials = str(target.get("credentialsEnv") or "")
    if not re.match(r"^[A-Z_][A-Z0-9_]*$", credentials):
        errors.append("target.credentialsEnv must be an env var NAME (uppercase), never a secret value")
    if not str(target.get("principal") or "").strip():
        errors.append("target.principal is required (pre-provisioned perf principal)")
    if profile.get("scale") not in SCALES:
        errors.append(f"profile.scale must be one of {SCALES}")
    if profile.get("history") not in HISTORIES:
        errors.append(f"profile.history must be one of {HISTORIES}")
    ladders = profile.get("ladders")
    if ladders is not None:
        if not isinstance(ladders, dict):
            errors.append("profile.ladders must be a mapping of ladder name to list")
        else:
            for name, values in ladders.items():
                if not re.match(r"^[a-z][A-Za-z0-9]*$", str(name)):
                    errors.append(f"ladder name {name!r} is not an identifier")
                if not isinstance(values, list) or not values:
                    errors.append(f"ladder {name} must be a non-empty list")
    if load.get("arrivalModel") not in ("open-loop", "closed-loop"):
        errors.append("load.arrivalModel must be open-loop or closed-loop")
    if cleanup.get("scratchRepositories") not in ("delete", "retain"):
        errors.append("cleanup.scratchRepositories must be delete or retain")
    limits = doc.get("limits") or {}
    for key in ("generatorCPUPercent", "diskUtilizationStop", "maxInFlight"):
        value = limits.get(key)
        if not isinstance(value, (int, float)):
            errors.append(f"limits.{key} must be numeric")
    if errors:
        return errors, doc
    return [], doc


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true", help="full tree as JSON")
    parser.add_argument("--view", help="show one concern view (write/read/index/search/capacity/resilience)")
    parser.add_argument("--metrics", action="store_true", help="print the metric registry")
    parser.add_argument("--check", action="store_true", help="check tree structure and references")
    parser.add_argument("--env", metavar="FILE", help="validate an environment config file")
    args = parser.parse_args()
    chosen = sum(1 for flag in (args.json, args.metrics, args.check) if flag) + (1 if args.view or args.env else 0)
    if chosen > 1:
        parser.error("choose one mode")
    try:
        if args.env:
            errors, _ = check_env(Path(args.env))
            if errors:
                print("\n".join(errors), file=sys.stderr)
                return 1
            print(f"{args.env}: environment config shape OK (binding still requires a live deployment).")
            return 0
        model = build_model()
        if args.metrics:
            print(text_metrics(model))
            return 0
        if args.check:
            result = check(model)
            for warning in result["warnings"]:
                print(f"warning: {warning}")
            for gap_id, gap in result["gaps"]:
                print(f"gap: {gap_id}: {gap}")
            if result["errors"]:
                print("\n".join(result["errors"]), file=sys.stderr)
                return 1
            print("Performance scene tree: structure, views, metrics and capability references are consistent.")
            return 0
        if args.json:
            serializable = {k: v for k, v in model.items() if k != "states"}
            serializable["states"] = [
                {k: (str(v) if k == "dir" else v) for k, v in state.items() if k != "dir"}
                for state in model["states"]
            ]
            print(json.dumps(serializable, ensure_ascii=False, indent=2))
            return 0
        print(text_tree(model, view=args.view))
        return 0
    except (ValueError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
