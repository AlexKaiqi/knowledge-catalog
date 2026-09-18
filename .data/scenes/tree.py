#!/usr/bin/env python3
"""Print this directory's use-case tree."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
META = "_meta.yaml"
BUNDLES = "_bundles.yaml"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true", help="full view as JSON")
    args = parser.parse_args()
    try:
        view = build_view(ROOT)
    except Exception as exc:
        print(f"tree.py: {exc}", file=sys.stderr)
        return 1
    if args.json:
        json.dump(view, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
        return 0
    sys.stdout.write(text_tree(view["tree"]))
    return 0


def build_view(root: Path) -> dict:
    states = walk_states(root)
    bundles = walk_bundles(root)
    by_id = {state["id"]: state for state in states}
    children: dict[str, list[str]] = {}
    roots: list[str] = []
    for state in states:
        deps = state.get("depends_on") or []
        if not deps:
            roots.append(state["id"])
            continue
        children.setdefault(deps[0], []).append(state["id"])
    for ids in children.values():
        ids.sort()
    roots.sort()
    tree = [view_node(i, by_id, children) for i in roots]
    return {
        "states": states,
        "bundles": bundles,
        "tree": tree,
    }


def walk_states(root: Path) -> list[dict]:
    states = []
    seen: dict[str, str] = {}
    for path in sorted(p for p in root.rglob("*") if p.is_dir()):
        if path == root or skip_dir(path, root):
            continue
        meta = path / META
        if not meta.is_file():
            raise ValueError(f"{path}: missing {META} (every state directory must self-describe)")
        state = load_state(root, path)
        prev = seen.get(state["id"])
        if prev:
            raise ValueError(f"state {state['id']} at {prev} and {state['dir']}")
        seen[state["id"]] = state["dir"]
        states.append(state)
    states.sort(key=lambda item: item["dir"])
    return states


def skip_dir(path: Path, root: Path) -> bool:
    try:
        rel = path.relative_to(root)
    except ValueError:
        return True
    return any(part.startswith("_") for part in rel.parts)


def load_state(root: Path, path: Path) -> dict:
    meta = load_yaml_file(path / META)
    if not isinstance(meta, dict):
        raise ValueError(f"{path}: {META} must be a mapping")
    state_id = path.name
    parent = path.parent.name
    state = {
        "id": state_id,
        "dir": str(path.relative_to(root)),
        "layer": "" if meta.get("layer") is None else str(meta.get("layer")).strip(),
        "role": meta.get("role") or "",
        "surface": meta.get("surface") or "",
    }
    if meta.get("source"):
        state["source"] = meta["source"]
    if parent != root.name:
        state["depends_on"] = [parent]
    if meta.get("publishes"):
        state["publishes"] = meta["publishes"]
    if meta.get("also_freezes"):
        state["also_freezes"] = meta["also_freezes"]
    if meta.get("summary"):
        state["summary"] = meta["summary"]
    if (path / "_build" / "construct.feature").is_file():
        state["construct"] = "_build/construct.feature"
    processes = []
    for ev in meta.get("evidence") or []:
        processes.append({
            "source": ev.get("source") or "",
            "surface": "go-test",
            "evidence": ev.get("tests") or [],
        })
    on_disk = {p.name for p in (path / "_probes").glob("*.feature")} if (path / "_probes").is_dir() else set()
    tagged = set()
    for probe in meta.get("probes") or []:
        base = Path(probe.get("file") or "").name
        if base not in on_disk:
            raise ValueError(f"{state_id}: probe {base} not in _probes/")
        tagged.add(base)
        processes.append({
            "file": f"_probes/{base}",
            "source": probe.get("source") or "",
            "surface": "feature",
        })
    for base in sorted(on_disk):
        if base not in tagged:
            raise ValueError(f"{state_id}: _probes/{base} has no source tag in {META}")
    if processes:
        state["processes"] = processes
    return state


def walk_bundles(root: Path) -> list[dict]:
    bundles = []
    for path in sorted(root.rglob(BUNDLES)):
        if skip_dir(path.parent, root):
            continue
        files = load_yaml_file(path)
        if not isinstance(files, list):
            raise ValueError(f"{path}: must be a list")
        entry = path.parent.name
        for item in files:
            bundle = {
                "id": item.get("id") or "",
                "suite": item.get("suite") or "",
                "summary": item.get("summary") or "",
                "entry_state": entry,
                "walk": item.get("walk") or [],
            }
            bundles.append(bundle)
    bundles.sort(key=lambda item: item["id"])
    return bundles


def view_node(state_id: str, by_id: dict, children: dict) -> dict:
    state = by_id[state_id]
    probes = [
        Path(proc["file"]).name
        for proc in state.get("processes") or []
        if proc.get("surface") == "feature" and proc.get("file")
    ]
    node = {
        "id": state_id,
        "dir": state["dir"],
        "construct": bool(state.get("construct")),
        "role": state.get("role") or "",
        "surface": state.get("surface") or "",
    }
    if state.get("summary"):
        node["summary"] = state["summary"]
    if probes:
        node["probes"] = probes
    kids = [view_node(child, by_id, children) for child in children.get(state_id, [])]
    if kids:
        node["children"] = kids
    return node


def text_tree(nodes: list[dict]) -> str:
    lines = [".data/scenes/"]
    for i, node in enumerate(nodes):
        write_text_node(lines, node, "", i == len(nodes) - 1)
    return "\n".join(lines) + "\n"


def write_text_node(lines: list[str], node: dict, prefix: str, last: bool) -> None:
    branch, nxt = ("└── ", "    ") if last else ("├── ", "│   ")
    label = node["id"] + "/"
    if node.get("summary"):
        label += "  # " + node["summary"]
    lines.append(f"{prefix}{branch}{label}")
    kids = node.get("children") or []
    for i, child in enumerate(kids):
        write_text_node(lines, child, prefix + nxt, i == len(kids) - 1)


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
    if lines[0][2].startswith("- "):
        value, i = _parse_list(lines, 0, lines[0][1])
    else:
        value, i = _parse_map(lines, 0, lines[0][1])
    if i != len(lines):
        raise ValueError(f"line {lines[i][0]}: leftover after parse")
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
        if ch in "'\"":
            quote = ch
            out.append(ch)
            continue
        if ch == "#":
            break
        out.append(ch)
    return "".join(out)


def _parse_map(lines, i, indent):
    result = {}
    while i < len(lines):
        no, ind, content = lines[i]
        if ind < indent:
            break
        if ind > indent:
            raise ValueError(f"line {no}: unexpected indent")
        if content.startswith("- "):
            break
        if ":" not in content:
            raise ValueError(f"line {no}: expected key:")
        key, rest = content.split(":", 1)
        key, rest = key.strip(), rest.strip()
        i += 1
        if rest == "":
            result[key], i = _parse_nested(lines, i, indent)
        else:
            result[key] = _scalar(rest)
    return result, i


def _parse_list(lines, i, indent):
    result = []
    while i < len(lines):
        no, ind, content = lines[i]
        if ind < indent:
            break
        if ind > indent:
            raise ValueError(f"line {no}: unexpected indent")
        if not content.startswith("- "):
            break
        rest = content[2:].strip()
        i += 1
        if rest == "":
            value, i = _parse_nested(lines, i, indent)
            result.append(value)
            continue
        if ":" in rest:
            key, val = rest.split(":", 1)
            item = {key.strip(): _scalar(val.strip()) if val.strip() else None}
            if i < len(lines) and lines[i][1] > indent:
                nested, i = _parse_map(lines, i, lines[i][1])
                for nested_key, nested_val in nested.items():
                    if item.get(key.strip()) is None and nested_key == key.strip():
                        item[nested_key] = nested_val
                    else:
                        item[nested_key] = nested_val
            result.append(item)
            continue
        result.append(_scalar(rest))
    return result, i


def _parse_nested(lines, i, parent_indent):
    if i >= len(lines) or lines[i][1] <= parent_indent:
        return None, i
    if lines[i][2].startswith("- "):
        return _parse_list(lines, i, lines[i][1])
    return _parse_map(lines, i, lines[i][1])


def _scalar(text):
    if text in ("null", "~"):
        return None
    if text in ("true", "True"):
        return True
    if text in ("false", "False"):
        return False
    if len(text) >= 2 and text[0] == text[-1] and text[0] in "'\"":
        return text[1:-1]
    if text.isdigit() or (text.startswith("-") and text[1:].isdigit()):
        return int(text)
    return text


if __name__ == "__main__":
    sys.exit(main())
