#!/usr/bin/env python3
"""Prepare a shared live fixture, then optionally apply one explicit walkthrough."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tree import load_yaml_file, walk_states  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", help="state id or path under .data/scenes")
    parser.add_argument("--probe", help="run this state's named positive probe after preparing its fixture")
    args = parser.parse_args()
    try:
        apply_target(args.target, probe=args.probe)
    except GotoError as exc:
        print(f"goto: {exc}", file=sys.stderr)
        return 1
    return 0


class GotoError(Exception):
    pass


def apply_target(target: str, *, probe: str | None = None) -> None:
    states = walk_states(ROOT)
    node = resolve(states, target)
    chain = ancestor_chain(states, node)
    selected = None
    if probe is not None:
        declared = {Path(case["file"]).name: case["file"] for case in node.get("processes", [])
                    if case["surface"] == "feature"}
        if probe not in declared:
            raise GotoError(f"{node['id']}: unknown probe {probe!r}")
        selected = ROOT / node["dir"] / declared[probe]
        validate_live_feature(selected)
    for state in chain:
        if state["id"] == "catalog-initialized":
            continue
        if not state.get("scene_construct"):
            raise GotoError(f"{state['id']} has no construct.feature")
        validate_live_feature(ROOT / state["dir"] / "_build" / "construct.feature")
    print(f"goto {node['id']}  ({node['dir']})")
    for state in chain:
        if state["id"] == "catalog-initialized":
            print(f"  skip {state['id']} (deployment already initialized)")
            continue
        start_runtime(state)
        apply_construct(state)
        print(f"  applied {state['id']}")
    if selected is not None:
        apply_feature(node, selected)
        print(f"  applied probe {probe}")
    print(f"ready {node['id']}")


def validate_live_feature(feature: Path) -> None:
    """Reject unsupported walkthrough syntax before starting any live setup."""
    for step in parse_feature(feature.read_text()):
        if step["kind"] == "error":
            raise GotoError(f"{feature}: live goto supports positive walkthroughs; use the test runner for error cases")
        values = [step["command"]] if step["kind"] == "run" else [cell for row in step.get("rows", []) for cell in row]
        for value in values:
            for variable in re.findall(r"\$(?:\{[^}]+\}|[A-Za-z_][A-Za-z0-9_.]*)", value):
                if step["kind"] != "run" or variable != "$materials":
                    raise GotoError(f"{feature}: unsupported variable {variable}; use the test runner")
        if step["kind"] == "run":
            split_command(step["command"])


def resolve(states: list[dict], target: str) -> dict:
    raw = target.strip().rstrip("/")
    if raw.startswith(".data/scenes/"):
        raw = raw[len(".data/scenes/") :]
    by_id = {state["id"]: state for state in states}
    by_dir = {state["dir"]: state for state in states}
    if raw in by_id:
        return by_id[raw]
    if raw in by_dir:
        return by_dir[raw]
    raise GotoError(f"unknown state {target!r}; use an id or path under .data/scenes")


def ancestor_chain(states: list[dict], node: dict) -> list[dict]:
    by_id = {state["id"]: state for state in states}
    chain = []
    current = node
    while True:
        chain.append(current)
        deps = current.get("depends_on") or []
        if not deps:
            break
        parent = by_id.get(deps[0])
        if parent is None:
            raise GotoError(f"{current['id']} missing parent {deps[0]}")
        current = parent
    chain.reverse()
    return chain


def start_runtime(state: dict) -> None:
    meta = load_yaml_file(ROOT / state["dir"] / "_meta.yaml")
    accessors = (meta.get("runtime") or {}).get("accessors") or {}
    if not accessors:
        return
    repo = accessors.get("repository") or "table-meta"
    script = REPO / "scripts" / "deploy" / "accessors.sh"
    env = os.environ.copy()
    env["KC_REPOSITORY"] = repo
    env["KC_NOTICE_REPOSITORY"] = repo
    env["KC_AS"] = os.environ.get("KC_AS", "admin")
    print(f"  accessors {repo}")
    subprocess.run([str(script), "up"], check=True, env=env, cwd=str(REPO))


def apply_construct(state: dict) -> None:
    feature = ROOT / state["dir"] / "_build" / "construct.feature"
    apply_feature(state, feature)


def apply_feature(state: dict, feature: Path) -> None:
    materials = ROOT / state["dir"] / "_materials"
    last = None
    for step in parse_feature(feature.read_text()):
        if step["kind"] == "skip":
            continue
        if step["kind"] == "run":
            command = expand_materials(step["command"], materials)
            last = run_kc(split_command(command))
            continue
        if step["kind"] == "has":
            if last is None:
                raise GotoError(f"{state['id']}: Then without When")
            for path, want in step["rows"]:
                match_has(last, path, want)
            continue
        if step["kind"] == "includes":
            if last is None:
                raise GotoError(f"{state['id']}: Then without When")
            for path, want in step["rows"]:
                match_includes(last, path, want)
            continue
        if step["kind"] == "error":
            raise GotoError(f"{state['id']}: live goto does not apply error constructs")
        raise GotoError(f"{state['id']}: unsupported step {step['kind']}")


def parse_feature(text: str) -> list[dict]:
    steps: list[dict] = []
    pending: dict | None = None
    for raw in text.splitlines():
        line = raw.split("#", 1)[0].rstrip()
        if not line.strip():
            continue
        stripped = line.strip()
        if stripped.startswith("|") and pending is not None:
            cells = [cell.strip() for cell in stripped.strip("|").split("|")]
            if len(cells) != 2:
                raise GotoError(f"table row must have 2 cells: {stripped}")
            pending["rows"].append((cells[0], cells[1]))
            continue
        pending = None
        keyword, _, rest = stripped.partition(" ")
        if keyword in {"Feature:", "Scenario:"}:
            continue
        if keyword in {"Given", "When", "Then", "And"}:
            rest = rest.strip()
            if rest == "deployment fixture" or rest.startswith("existing repository "):
                steps.append({"kind": "skip"})
                continue
            if rest.startswith("I run `") and rest.endswith("`"):
                steps.append({"kind": "run", "command": rest[len("I run `") : -1]})
                continue
            if rest == "the output has:":
                pending = {"kind": "has", "rows": []}
                steps.append(pending)
                continue
            if rest == "the output includes:":
                pending = {"kind": "includes", "rows": []}
                steps.append(pending)
                continue
            if rest.startswith("error "):
                steps.append({"kind": "error", "code": rest[len("error ") :]})
                continue
        raise GotoError(f"unsupported feature line: {stripped}")
    return steps


def expand_materials(command: str, materials: Path) -> str:
    return command.replace("$materials", str(materials))


def split_command(command: str) -> list[str]:
    if command.startswith("kc "):
        command = command[3:]
    return argv_split(command)


def argv_split(command: str) -> list[str]:
    out: list[str] = []
    buf: list[str] = []
    quote = ""
    i = 0
    while i < len(command):
        ch = command[i]
        if quote:
            if ch == quote:
                quote = ""
            else:
                buf.append(ch)
            i += 1
            continue
        if ch in {"'", '"'}:
            quote = ch
            i += 1
            continue
        if ch.isspace():
            if buf:
                out.append("".join(buf))
                buf = []
            i += 1
            continue
        buf.append(ch)
        i += 1
    if quote:
        raise GotoError(f"unclosed quote in {command!r}")
    if buf:
        out.append("".join(buf))
    return out


def run_kc(args: list[str]) -> any:
    catalog = os.environ.get("KC_CATALOG", "kr://acme/catalog")
    principal = os.environ.get("KC_AS", "admin")
    image = os.environ.get("KC_DEPLOY_CLI_IMAGE") or "kc-deploy-local-cli:local"
    network = os.environ.get("KC_ACCESSOR_NETWORK") or "kc-deploy-local_default"
    cmd = [
        "docker",
        "run",
        "--rm",
        "--network",
        network,
        "-v",
        f"{REPO}:{REPO}:ro",
        "-e",
        "KC_SERVER_URL=http://kc-server:7380",
        "-e",
        f"KC_CATALOG={catalog}",
        "-e",
        f"KC_AS={principal}",
        "--entrypoint",
        "/usr/local/bin/kc",
        image,
        *args,
    ]
    proc = subprocess.run(cmd, check=False, capture_output=True, text=True)
    text = proc.stdout.strip() or proc.stderr.strip()
    if proc.returncode != 0:
        if already_applied(args, text):
            return already_current(args)
        raise GotoError(f"kc {' '.join(args)} failed:\n{text}")
    try:
        return json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise GotoError(f"kc {' '.join(args)} did not return JSON:\n{proc.stdout}") from exc


def flag_value(args: list[str], name: str) -> str:
    for i, arg in enumerate(args):
        if arg == name and i + 1 < len(args):
            return args[i + 1]
    return ""


def already_applied(args: list[str], text: str) -> bool:
    blob = text.lower()
    markers = (
        "desired state already matches the current version",
        "commit: no changes",
        "idempotency_conflict",
        "already has a source binding",
        "already admitted",
        "already has a managed allocation",
        "already attached",
    )
    if args[:2] == ["writer", "remove"] and (
        "knowledge_ref_unresolved" in blob or "does not exist" in blob
    ):
        return True
    return any(marker in blob for marker in markers)


def already_current(args: list[str]) -> dict:
    if args and args[0] == "create":
        name = flag_value(args, "--name")
        return {"repositoryId": name, "name": name, "store": flag_value(args, "--store") or "lakefs"}
    repo = flag_value(args, "--repo")
    if args and args[0] == "attach":
        return {"repositoryId": repo}
    head = run_kc(["writer", "head", "--repo", repo]) if repo else {}
    commit = ""
    if isinstance(head, dict):
        commit = str(head.get("commit") or head.get("commitId") or "")
    return {"result": {"repositoryId": repo, "newCommit": commit}, "disposition": "UNCHANGED"}


def match_has(root: any, path: str, want: str) -> None:
    got, ok = lookup(root, path)
    if want == "absent":
        if ok:
            raise GotoError(f"{path}: want absent, got {got!r}")
        return
    if not ok:
        raise GotoError(f"{path}: missing in {json.dumps(root, ensure_ascii=False)}")
    if want == "nonempty":
        if not nonempty(got):
            raise GotoError(f"{path}: want nonempty, got {got!r}")
        return
    if stringify(got) != want:
        raise GotoError(f"{path}: got {got!r} want {want!r}")


def match_includes(root: any, path: str, want: str) -> None:
    parent, sep, field = path.partition("[]")
    if not sep:
        raise GotoError(f"{path}: includes requires []")
    got, ok = lookup(root, parent)
    if not ok or not isinstance(got, list):
        raise GotoError(f"{parent}: want array, got {got!r}")
    field = field[1:] if field.startswith(".") else field
    for item in got:
        if field:
            value, exists = lookup(item, field)
            if exists and stringify(value) == want:
                return
        elif isinstance(item, dict):
            for key in ("id", "setId", "objectId", "object", "principal"):
                if str(item.get(key, "")) == want:
                    return
        elif str(item) == want:
            return
    raise GotoError(f"{path}: {got!r} does not include {want!r}")


def lookup(root: any, path: str) -> tuple[any, bool]:
    if not path:
        return root, True
    cur = root
    for part in path.split("."):
        if isinstance(cur, dict) and part in cur:
            cur = cur[part]
            continue
        if isinstance(cur, list) and part.isdigit() and int(part) < len(cur):
            cur = cur[int(part)]
            continue
        return None, False
    return cur, True


def stringify(value: any) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value)


def nonempty(value: any) -> bool:
    if value is None:
        return False
    if isinstance(value, (str, list, dict)):
        return len(value) > 0
    return True


if __name__ == "__main__":
    raise SystemExit(main())
