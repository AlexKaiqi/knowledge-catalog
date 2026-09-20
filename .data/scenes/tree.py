#!/usr/bin/env python3
"""Project state and verification views from the physical scene tree."""
from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
META = "_meta.yaml"
BUNDLES = "_bundles.yaml"
VIEWS = "_views.yaml"
_product_spec = importlib.util.spec_from_file_location("scene_product_views", ROOT / "_product_views.py")
product_tools = importlib.util.module_from_spec(_product_spec)
_product_spec.loader.exec_module(product_tools)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true", help="full view as JSON")
    parser.add_argument("--list-views", action="store_true", help="list declared viewpoints")
    parser.add_argument("--family", choices=("product", "engineering"), help="list a view family; product lists claims, case links and gaps")
    parser.add_argument("--view", help="show one viewpoint, including connecting context")
    parser.add_argument("--from", dest="from_id", metavar="ID", help="display from this node; retain hidden prerequisites")
    parser.add_argument("--probes-at", metavar="ID", help="show one node's verification cases without descendants")
    parser.add_argument("--check-views", action="store_true", help="check the union of all viewpoints against the physical tree")
    parser.add_argument("--check-product", action="store_true", help="check owner document claims and case links, without executing evidence")
    parser.add_argument("--check-states", action="store_true", help="check that every state builds a fixture reused by scene probes")
    args = parser.parse_args()
    if (args.list_views or args.check_views or args.check_product or args.check_states or args.family) and (args.view or args.from_id or args.probes_at):
        parser.error("listing/checking all views cannot be combined with a display selection")
    if sum((args.list_views, args.check_views, args.check_product, args.check_states)) > 1 or args.family and (args.check_views or args.check_product or args.check_states):
        parser.error("choose one inventory or check mode")
    try:
        model = build_view(ROOT)
        if args.list_views:
            view = {"views": ordered_views(model, args.family)}
            output = text_view_list(view["views"])
        elif args.family == "product":
            view = model["product"]
            output = product_tools.inventory_text(view)
        elif args.family == "engineering":
            view = {"views": ordered_views(model, args.family)}
            output = text_view_list(view["views"])
        elif args.check_states:
            view = check_states(model)
            output = "\n".join(view["errors"]) + "\n" if view["errors"] else "Every state builds a declared fixture reused by multiple scene probes.\n"
        elif args.check_product:
            view = check_product(model)
            output = "\n".join(view["errors"]) + "\n" if view["errors"] else "Product claim links and explicit gaps are consistent; execution not evaluated.\n"
        elif args.check_views:
            view = check_views(model)
            output = "\n".join(view["errors"]) + "\n" if view["errors"] else "View union covers every node, edge, probe and Go evidence reference.\n"
        else:
            view = project_view(model, view_id=args.view, from_id=args.from_id, probes_at=args.probes_at)
            output = text_tree(view["tree"], detailed=args.probes_at is not None)
            if view.get("product_claim"):
                output = "\n".join(product_tools.claim_text(view["product_claim"])) + "\nExecution: not_evaluated\n" + output
            if view["prerequisites"]:
                output += "Hidden build prerequisites: " + ", ".join(view["prerequisites"]) + "\n"
    except Exception as exc:
        print(f"tree.py: {exc}", file=sys.stderr)
        return 1
    if args.json:
        json.dump(view, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
    else:
        sys.stdout.write(output)
    return 1 if (args.check_views or args.check_product or args.check_states) and view["errors"] else 0


def build_view(root: Path) -> dict:
    states = walk_states(root)
    bundles = walk_bundles(root)
    by_id = {state["id"]: state for state in states}
    for state in states:
        ancestors = ancestor_ids(state["id"], by_id)
        state["unbuildable_ancestors"] = [name for name in ancestors if not by_id[name]["scene_construct"]]
        state["executable"] = state["scene_construct"] and not state["unbuildable_ancestors"]
    definitions = load_views(root)
    config = load_yaml_file(root / VIEWS) if (root / VIEWS).is_file() else {}
    product = product_tools.build_product(root, config.get("product_documents", {}), states)
    generated = product_tools.product_definitions(product)
    if any(name.startswith("product/") for name in definitions):
        product["errors"].append("handwritten product view definitions are forbidden; product claims come from their owner document")
    return {
        "states": states,
        "bundles": bundles,
        "tree": state_tree(states),
        "views": {**definitions, **generated},
        "product": product,
        "edges": [{"parent": parent, "child": state["id"],
                   "kind": "build" if state["scene_construct"] else "evidence"}
                  for state in states for parent in state.get("depends_on", [])],
    }


def state_tree(states: list[dict]) -> list[dict]:
    by_id = {state["id"]: state for state in states}
    children: dict[str, list[str]] = {}
    roots: list[str] = []
    for state in states:
        deps = state.get("depends_on") or []
        if not deps or deps[0] not in by_id:
            roots.append(state["id"])
            continue
        children.setdefault(deps[0], []).append(state["id"])
    for ids in children.values():
        ids.sort()
    roots.sort()
    return [view_node(i, by_id, children) for i in roots]


def ancestor_ids(state_id: str, by_id: dict) -> list[str]:
    ancestors = []
    node = by_id[state_id]
    while node.get("depends_on"):
        parent = node["depends_on"][0]
        if parent not in by_id:
            raise ValueError(f"{state_id}: missing parent {parent}")
        ancestors.append(parent)
        node = by_id[parent]
    return list(reversed(ancestors))


def load_views(root: Path) -> dict:
    path = root / VIEWS
    if not path.is_file():
        return {}
    data = load_yaml_file(path)
    if not isinstance(data, dict) or set(data) - {"views", "product_documents"} or not isinstance(data.get("views", {}), dict):
        raise ValueError(f"{path}: expected views mapping")
    for name, definition in data.get("views", {}).items():
        if not isinstance(definition, dict) or set(definition) - {"title", "description", "entry"}:
            raise ValueError(f"{path}: view {name} permits only title, description and optional entry; no member or edge lists")
        if not isinstance(definition.get("title"), str) or not definition["title"].strip():
            raise ValueError(f"{path}: view {name} needs a title")
        for field in ("description", "entry"):
            if field in definition and not isinstance(definition[field], str):
                raise ValueError(f"{path}: view {name} {field} must be a string")
        definition["family"] = "engineering"
    return data.get("views", {})


def ordered_views(model: dict, family: str | None = None) -> dict:
    return {name: view for group in ("product", "engineering") for name, view in model["views"].items()
            if view["family"] == group and (family is None or family == group)}


def text_view_list(views: dict) -> str:
    lines, previous = [], None
    for name, definition in views.items():
        if definition["family"] != previous:
            previous = definition["family"]
            lines.append("Product" if previous == "product" else "Engineering")
        lines.append(f"  {name}: {definition['title']}")
        if definition.get("description"):
            lines.append(f"    {definition['description']}")
    return "\n".join(lines) + "\n"


def check_product(model: dict) -> dict:
    product = model["product"]
    errors = list(product["errors"])
    if not product["registered_documents"]:
        errors.append("no product_documents registered")
    return {"errors": errors, "claims": product["claims"], "execution_status": "not_evaluated"}


def check_states(model: dict) -> dict:
    """Reject test-group directories and one-off outcomes; expose actual consumers.

    Reachability is a necessary check. Review must also establish that each
    listed probe uses the fixture, rather than just adding probes to a directory.
    Independent Go tests build their own fixtures and never count as consumers.
    """
    states = {state["id"]: state for state in model["states"]}
    consumers = {name: set() for name in states}
    for state in states.values():
        probes = {case["id"] for case in state.get("processes", []) if case["surface"] == "feature"}
        if state["executable"]:
            for name in ancestor_ids(state["id"], states) + [state["id"]]:
                consumers[name].update(probes)
    errors, fixtures = [], []
    for name, state in states.items():
        fixture = state.get("fixture")
        if not state["executable"]:
            errors.append(f"{name}: state requires a real executable construct; move independent evidence to its host")
        if not isinstance(fixture, str) or not fixture.strip():
            errors.append(f"{name}: fixture must describe the constructed condition consumed by later cases")
        if len(consumers[name]) < 2:
            errors.append(f"{name}: no fixture reuse across distinct probes; keep a one-off transition inside its case")
        fixtures.append({"id": name, "fixture": fixture, "probe_consumers": sorted(consumers[name])})
    return {"errors": errors, "fixtures": fixtures, "execution_status": "not_evaluated"}


def project_view(model: dict, *, view_id: str | None = None, from_id: str | None = None,
                 probes_at: str | None = None) -> dict:
    """Keep actual parent edges; boundaries change display, never construction."""
    by_id = {state["id"]: state for state in model["states"]}
    definitions = model["views"]
    if view_id is not None and view_id not in definitions:
        raise ValueError(f"unknown view {view_id}")
    product_claim = next((claim for claim in model["product"]["claims"] if claim["view_id"] == view_id), None)
    claim_ref = product_claim["ref"] if product_claim else None
    entry = definitions[view_id].get("entry") if view_id is not None else None
    for label, state_id in (("entry", entry), ("from", from_id), ("probes-at", probes_at)):
        if state_id is not None and state_id not in by_id:
            raise ValueError(f"unknown {label} node {state_id}")
    ancestry = {name: ancestor_ids(name, by_id) for name in by_id}
    scope = {name for name in by_id
             if all(boundary is None or name == boundary or boundary in ancestry[name]
                    for boundary in (entry, from_id))}
    if probes_at is not None:
        if probes_at not in scope:
            raise ValueError(f"{probes_at}: outside the selected display boundary")
        scope = {probes_at}
    if claim_ref:
        selected = {name for name in scope if any(link["claim"] == claim_ref for link in by_id[name]["construct_verifies"])}
    else:
        selected = {name for name in scope if view_id is None or view_id in by_id[name]["views"]}
    processes = {name: [proc for proc in by_id[name].get("processes", [])
                        if (any(link["claim"] == claim_ref for link in proc["verifies"]) if claim_ref
                            else view_id is None or view_id in proc["views"])] for name in scope}
    visible = selected | {name for name, cases in processes.items() if cases}
    for name in list(visible):
        visible.update(parent for parent in ancestry[name] if parent in scope)
    states = [dict(state, processes=processes.get(state["id"], []),
                   selected=state["id"] in selected, context=state["id"] not in selected,
                   construct_selected=bool(claim_ref and state["id"] in selected),
                   construct_verifies=[link for link in state["construct_verifies"] if not claim_ref or link["claim"] == claim_ref])
              for state in model["states"] if state["id"] in visible]
    if product_claim:
        case_ids = {case["id"] for state in states for case in state["processes"]}
        case_ids.update(f"{name}::construct" for name in selected)
        selected_links = [link for link in product_claim["links"] if link["case_id"] in case_ids]
        product_claim = dict(product_claim, links=selected_links, declared_link_count=len(product_claim["links"]),
                             selected_link_count=len(selected_links))
    hidden = {parent for name in visible for parent in ancestry[name]} - visible
    # Go evidence groups are standalone test references, not fixture consumers.
    required = {parent for name in visible if by_id[name]["scene_construct"]
                for parent in ancestry[name] if parent in hidden and by_id[parent]["scene_construct"]}
    all_nodes = hidden | visible
    return {
        "states": states,
        "bundles": [bundle for bundle in model["bundles"] if bundle["entry_state"] in visible],
        "tree": state_tree(states),
        "views": definitions,
        "product_claim": product_claim,
        "execution_status": "not_evaluated",
        "selection": {"view": view_id, "entry": entry, "from": from_id, "probes_at": probes_at},
        "edges": [edge for edge in model["edges"] if edge["parent"] in visible and edge["child"] in visible],
        "prerequisites": sorted(required, key=lambda name: (len(ancestry[name]), name)),
        "prerequisite_edges": [edge for edge in model["edges"]
                               if edge["parent"] in all_nodes and edge["child"] in all_nodes
                               and (edge["parent"] in hidden or edge["child"] in hidden)],
    }


def case_ids(states: list[dict]) -> tuple[set[str], set[str]]:
    probes, evidence = set(), set()
    for state in states:
        for proc in state.get("processes", []):
            if proc["surface"] == "feature":
                probes.add(proc["id"])
            else:
                evidence.update(f"{state['id']}::go:{proc['source']}:{ref}" for ref in proc["evidence"])
    return probes, evidence


def check_views(model: dict) -> dict:
    """Check independently declared membership and the deduplicated view union."""
    errors = []
    definitions = {name: view for name, view in model["views"].items() if view["family"] == "engineering"}
    if not definitions:
        errors.append(f"{VIEWS}: no views declared")
    for state in model["states"]:
        for item in [state] + state.get("processes", []):
            if not item["views"]:
                errors.append(f"{item['id']}: missing views membership")
            for name in item["views"]:
                if name not in definitions:
                    errors.append(f"{item['id']}: unknown view {name}")
    probes, evidence = case_ids(model["states"])
    total = {"nodes": {state["id"] for state in model["states"]},
             "edges": {(edge["parent"], edge["child"]) for edge in model["edges"]},
             "probes": probes, "evidence": evidence}
    covered = {key: set() for key in total}
    for name in definitions:
        try:
            view = project_view(model, view_id=name)
        except ValueError as exc:
            errors.append(f"{name}: {exc}")
            continue
        covered["nodes"].update(state["id"] for state in view["states"] if state["selected"])
        covered["edges"].update((edge["parent"], edge["child"])
                                for edge in view["edges"] + view["prerequisite_edges"])
        selected_probes, selected_evidence = case_ids(view["states"])
        covered["probes"].update(selected_probes)
        covered["evidence"].update(selected_evidence)
    missing = {key: sorted(total[key] - covered[key]) for key in total}
    for kind, items in missing.items():
        for item in items:
            errors.append(f"view union omits {kind}: {item}")
    return {"errors": errors, "totals": {key: len(items) for key, items in total.items()},
            "covered": {key: len(items) for key, items in covered.items()}, "missing": missing}


def view_tags(meta: dict, label: str) -> list[str]:
    tags = meta.get("views", [])
    if not isinstance(tags, list) or any(not isinstance(tag, str) or not tag.strip() for tag in tags):
        raise ValueError(f"{label}: views must be a list of nonempty strings")
    return sorted(set(tags))


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
        "views": view_tags(meta, state_id),
        "fixture": meta.get("fixture"),
        "construct_verifies": meta.get("construct_verifies", []),
    }
    if "verifies" in meta:
        state["verifies"] = meta["verifies"]
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
    # A Go-only runtime may keep an illustrative feature without providing a
    # fixture that the scene executor can build or descendants can inherit.
    state["scene_construct"] = bool(state.get("construct")) and state["surface"] in ("feature", "both")
    state["kind"] = "state" if state["scene_construct"] else "evidence-group"
    processes = []
    for ev in meta.get("evidence") or []:
        evidence = ev.get("tests") or []
        source = ev.get("source") or ""
        case_id = ev.get("id") or ""
        process_id = f"{state_id}::go:{case_id or source + ':' + '|'.join(evidence)}"
        processes.append({
            "id": process_id,
            "case_id": case_id,
            "title": ev.get("title") or "",
            "source": source,
            "surface": "go-test",
            "evidence": evidence,
            "views": view_tags(ev, process_id),
            "verifies": ev.get("verifies", []),
        })
    on_disk = {p.name for p in (path / "_probes").glob("*.feature")} if (path / "_probes").is_dir() else set()
    tagged = set()
    for probe in meta.get("probes") or []:
        base = Path(probe.get("file") or "").name
        if base not in on_disk:
            raise ValueError(f"{state_id}: probe {base} not in _probes/")
        if base in tagged:
            raise ValueError(f"{state_id}: duplicate probe declaration {base}")
        tagged.add(base)
        process_id = f"{state_id}::probe:{base}"
        processes.append({
            "id": process_id,
            "case_id": probe.get("id") or "",
            "title": probe.get("title") or "",
            "file": f"_probes/{base}",
            "source": probe.get("source") or "",
            "surface": "feature",
            "views": view_tags(probe, process_id),
            "verifies": probe.get("verifies", []),
        })
    for base in sorted(on_disk):
        if base not in tagged:
            raise ValueError(f"{state_id}: _probes/{base} has no source tag in {META}")
    if processes:
        ids = [proc["id"] for proc in processes]
        if len(set(ids)) != len(ids):
            raise ValueError(f"{state_id}: duplicate verification case identity")
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
        "scene_construct": state["scene_construct"],
        "role": state.get("role") or "",
        "surface": state.get("surface") or "",
        "kind": state["kind"],
        "views": state["views"],
        "fixture": state.get("fixture"),
        "executable": state["executable"],
        "context": state.get("context", False),
        "construct_selected": state.get("construct_selected", False),
        "construct_verifies": state.get("construct_verifies", []),
    }
    if state.get("summary"):
        node["summary"] = state["summary"]
    if probes:
        node["probes"] = probes
        node["probe_cases"] = [proc for proc in state["processes"] if proc["surface"] == "feature"]
    evidence = [proc for proc in state.get("processes", []) if proc["surface"] == "go-test"]
    if evidence:
        node["evidence"] = evidence
    kids = [view_node(child, by_id, children) for child in children.get(state_id, [])]
    if kids:
        node["children"] = kids
    return node


def text_tree(nodes: list[dict], *, detailed: bool = False) -> str:
    lines = [".data/scenes/"]
    for i, node in enumerate(nodes):
        write_text_node(lines, node, "", i == len(nodes) - 1, detailed=detailed)
    return "\n".join(lines) + "\n"


def write_text_node(lines: list[str], node: dict, prefix: str, last: bool, *, detailed: bool = False) -> None:
    branch, nxt = ("└── ", "    ") if last else ("├── ", "│   ")
    label = node["id"] + "/"
    if node.get("context"):
        label += " [context]"
    if node.get("kind") == "evidence-group":
        label += " [Go evidence group; reference feature only]" if node["construct"] else " [Go evidence group; no construct]"
    elif not node.get("executable"):
        label += " [not constructable: ancestor has no scene construct]"
    if node.get("summary"):
        label += "  # " + node["summary"]
    lines.append(f"{prefix}{branch}{label}")
    if node.get("construct_selected") or detailed:
        for link in node.get("construct_verifies", []):
            lines.append(f"{prefix}{nxt}  construct verifies {link['claim']}: {link['detail']}")
    for probe in node.get("probe_cases", []):
        label = probe["title"] or Path(probe["file"]).name
        if probe["title"]:
            label += f" ({Path(probe['file']).name})"
        lines.append(f"{prefix}{nxt}  probe: {label}")
        if detailed:
            for link in probe["verifies"]:
                lines.append(f"{prefix}{nxt}    verifies {link['claim']}: {link['detail']}")
    for evidence in node.get("evidence", []):
        label = evidence["title"] or evidence["source"]
        lines.append(f"{prefix}{nxt}  Go: {label} [{len(evidence['evidence'])} references]")
        if detailed:
            for link in evidence["verifies"]:
                lines.append(f"{prefix}{nxt}    verifies {link['claim']}: {link['detail']}")
            for reference in evidence["evidence"]:
                lines.append(f"{prefix}{nxt}    {reference}")
    kids = node.get("children") or []
    for i, child in enumerate(kids):
        write_text_node(lines, child, prefix + nxt, i == len(kids) - 1, detailed=detailed)


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
    if text.startswith("[") and text.endswith("]"):
        inner = text[1:-1].strip()
        if not inner:
            return []
        return [_scalar(item.strip()) for item in next(csv.reader([inner], skipinitialspace=True))]
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
