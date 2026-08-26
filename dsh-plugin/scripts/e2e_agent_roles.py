#!/usr/bin/env python3
"""Six independent real DSH Agent roles over one clean Knowledge Catalog."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
BASE = os.environ.get("KC_SERVE", "http://127.0.0.1:18380")
PROFILE = os.environ.get("DSH_PROFILE", "loom-agent-roles")
MODEL_PATCH = Path(
    os.environ.get(
        "DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")
    )
)
ARTIFACTS = Path(
    os.environ.get(
        "KC_ROLE_ARTIFACTS", tempfile.mkdtemp(prefix="kc-agent-role-evidence-")
    )
)
CATALOG = "kr://acme/catalog"
REPO = "kr://acme/public/core"
WORKSPACE = "agent"
OBJECT = "policy/P-103"
DSH_EXECUTABLE = os.environ.get("DSH_EXECUTABLE", "dsh")

OWNER_TASK = f"""You are the Catalog Owner in a brand-new empty working directory.
Load and follow the bundled knowledge-catalog skill, then use the kc tool for every
Knowledge Catalog action. Do not call a host kc command directly.

Create Catalog {CATALOG}, add Repository {REPO}, and seed object {OBJECT} with
JSON value {{"v":1,"status":"draft"}} using command-id owner-seed-1 and SOURCE
provenance (source-ref agent://owner/bootstrap, actor-ref workspace-owner).
Define Workspace {WORKSPACE} revision 1 with the root mount
{REPO}=refs/heads/main@. Configure one merge gate on {REPO} with the single
comma-separated require value validate,suite:approval:steward.

Create least-privilege grants for these future independent principals:
- producer: propose on {REPO}
- reviewer: preview,validate,record-validation on {CATALOG}; merge on {REPO}
- consumer: read-workspace on Catalog {CATALOG} and Workspace {WORKSPACE};
  read,list,resolve,search on {REPO}
- auditor: audit on {CATALOG}; read,log,provenance on {REPO}
Do not grant anything to mallory. Verify producer propose, consumer
read-workspace for Catalog {CATALOG}/Workspace {WORKSPACE}, and consumer read on
{REPO} with allowed. Use only skill and kc tools; any bash, filesystem, source,
or KC-home inspection is a failed run. When complete reply exactly OWNER=ready.
"""

PRODUCER_TASK = f"""You are the Producer principal. Load and follow the
knowledge-catalog skill. Use only the kc tool under your fixed current identity.
Create proposal PR-AGENT-1 against {REPO}, target refs/heads/main, candidate
refs/heads/candidates/PR-AGENT-1, changing object {OBJECT} to
{{"v":2,"status":"governed"}}. Attach SOURCE provenance with source-ref
agent://producer/PR-AGENT-1 and actor-ref producer. Do not preview, validate, or
merge. Do not use host or filesystem tools. Reply exactly PRODUCER=proposed
after the proposal succeeds.
"""

REVIEWER_TASK = f"""You are the Reviewer/Gatekeeper principal. Load and follow
the knowledge-catalog skill. Use only the kc tool under your fixed identity.
For proposal PR-AGENT-1 create a preview against Workspace {WORKSPACE}, run the
built-in structural validation and require PASSED, then record the independent
suite approval:steward as PASSED for that same preview. Merge using the exact
proposal and preview IDs; KC must evaluate the stored reports required by the
gate. Do not pass a validation array, alter proposal content, inspect files or
source, or bypass a missing gate. Reply exactly REVIEWER=merged when the public
merge receipt reports both required checks PASSED.
"""

CONSUMER_TASK = f"""You are the Consumer principal in a new independent session.
Load and follow the knowledge-catalog skill. Without calling knowledge_context
first, use knowledge_list with objectPrefix policy/ to discover the canonical
object ID, then knowledge_read that object. Do not pass Catalog, Workspace,
principal, or pin yourself. Do not use bash or filesystem tools and do not write.
Reply exactly CONSUMER=v2 when the automatically bound fixed-pin read shows v=2
and status=governed.
"""

AUDITOR_TASK = f"""You are the Auditor principal in a new independent session.
Load and follow the knowledge-catalog skill. Use only the kc tool. Inspect Catalog
{CATALOG} audit history for Workspace {WORKSPACE}, object log for {OBJECT} in
{REPO} at refs/heads/main, and provenance for that object. Verify the history
contains the governed publication and provenance identifies producer/source
agent://producer/PR-AGENT-1. Do not mutate anything or use host/filesystem tools.
Reply exactly AUDITOR=verified.
"""

UNAUTHORIZED_TASK = f"""You are Unauthorized Actor mallory. Load and follow the
knowledge-catalog skill. Attempt exactly one kc put of object {OBJECT} in {REPO}
using command-id mallory-denied-1 and value {{"v":999}}. It must fail with
FORBIDDEN. Do not retry, do not omit or change identity, and do not use Bash or
the filesystem to bypass Writer. Reply exactly UNAUTHORIZED=FORBIDDEN after the
denial.
"""


def request(method: str, path: str, body: dict | None = None) -> dict:
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        f"{BASE}{path}",
        data=data,
        headers={"content-type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        payload = error.read().decode("utf-8", "replace")
        raise RuntimeError(f"{method} {path} HTTP {error.code}: {payload}") from error


def post(verb: str, flags: dict) -> dict:
    return request("POST", f"/v1/{verb}", flags)


def wait_service() -> None:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        try:
            if request("GET", "/health").get("ok"):
                return
        except Exception:
            time.sleep(0.1)
    raise RuntimeError(f"kc service did not become healthy at {BASE}")


def verify_skill_trace(name: str, workdir: Path) -> None:
    """Prove the real Agent loaded the plugin-bundled Skill, not just the prompt."""
    dsh_home = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh")))
    traces = list(
        (dsh_home / "sessions").glob(
            f"*{workdir.name}*/session-*/session.jsonl.zstd"
        )
    )
    if not traces:
        raise RuntimeError(f"{name} produced no DSH session trace")
    trace = max(traces, key=lambda path: path.stat().st_mtime)
    decoded = subprocess.run(
        ["zstd", "-dc", str(trace)],
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    loaded = False
    kc_calls = 0
    typed_calls = 0
    forbidden_host_calls: list[str] = []
    call_names: dict[str, str] = {}
    unexpected_tool_errors: list[str] = []

    def contains_tool_error(value: object) -> bool:
        if isinstance(value, dict):
            if value.get("isError") is True:
                return True
            return any(contains_tool_error(item) for item in value.values())
        if isinstance(value, list):
            return any(contains_tool_error(item) for item in value)
        return False

    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "tool/result":
            data = event.get("data", {})
            if contains_tool_error(data):
                call_id = str(data.get("message", {}).get("source", {}).get("callId", ""))
                tool_name = call_names.get(call_id, "unknown")
                rendered = json.dumps(data, ensure_ascii=False)
                expected_denial = (
                    name == "unauthorized"
                    and tool_name == "kc"
                    and "not allowed to put" in rendered
                )
                if not expected_denial:
                    unexpected_tool_errors.append(tool_name)
            continue
        if event.get("type") != "tool/call":
            continue
        data = event.get("data", {})
        if data.get("callId"):
            call_names[str(data.get("callId"))] = str(data.get("name", "unknown"))
        if data.get("name") == "kc":
            kc_calls += 1
        if data.get("name") in {
            "knowledge_context", "knowledge_list", "knowledge_read", "knowledge_search",
            "knowledge_schema", "knowledge_relations", "knowledge_provenance", "resource"
        }:
            typed_calls += 1
        if data.get("name") in {
            "bash", "read", "write", "glob", "grep", "find", "list", "shell"
        }:
            forbidden_host_calls.append(str(data.get("name")))
        if data.get("name") != "skill":
            continue
        try:
            arguments = json.loads(data.get("arguments", "{}"))
        except (json.JSONDecodeError, TypeError):
            continue
        if arguments.get("name") == "knowledge-catalog":
            loaded = True
    trace_evidence = {
        "trace": str(trace),
        "knowledgeCatalogSkillLoaded": loaded,
        "kcToolCalls": kc_calls,
        "typedKnowledgeToolCalls": typed_calls,
        "forbiddenHostToolCalls": forbidden_host_calls,
        "unexpectedToolErrors": unexpected_tool_errors,
    }
    (ARTIFACTS / f"{name}.trace.json").write_text(
        json.dumps(trace_evidence, indent=2) + "\n"
    )
    if not loaded:
        raise RuntimeError(f"{name} did not load the bundled knowledge-catalog Skill")
    if kc_calls + typed_calls == 0:
        raise RuntimeError(f"{name} made no Knowledge Catalog tool call")
    if forbidden_host_calls:
        raise RuntimeError(
            f"{name} bypassed the public Agent surface with host tools: "
            + ", ".join(forbidden_host_calls)
        )
    if unexpected_tool_errors:
        raise RuntimeError(
            f"{name} needed failed tool-call retries: "
            + ", ".join(unexpected_tool_errors)
        )


def run_role(name: str, principal: str | None, task: str, marker: str) -> None:
    workdir = Path(tempfile.mkdtemp(prefix=f"dsh-role-{name}-"))
    if any(workdir.iterdir()):
        raise RuntimeError(f"role workspace is not empty: {workdir}")
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    if principal:
        env["KC_AS"] = principal
    else:
        env.pop("KC_AS", None)
    proc = subprocess.run(
        [DSH_EXECUTABLE, "--profile", PROFILE, "--patch", str(MODEL_PATCH), task],
        cwd=workdir,
        env=env,
        capture_output=True,
        text=True,
        timeout=480,
    )
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / f"{name}.stdout.txt").write_text(proc.stdout)
    (ARTIFACTS / f"{name}.stderr.txt").write_text(proc.stderr)
    print(f"{name}: exit={proc.returncode} output={proc.stdout.strip()[-500:]}")
    if proc.returncode != 0 or marker not in proc.stdout:
        raise RuntimeError(
            f"{name} failed; expected {marker}; stderr tail:\n{proc.stderr[-4000:]}"
        )
    verify_skill_trace(name, workdir)


def current_value() -> dict:
    return post(
        "read", {"repo": REPO, "object": OBJECT, "ref": "refs/heads/main"}
    )["value"]


def main() -> int:
    if not MODEL_PATCH.is_file():
        print(f"missing model patch: {MODEL_PATCH}", file=sys.stderr)
        return 1
    try:
        run_role("owner", None, OWNER_TASK, "OWNER=ready")
        wait_service()
        status = post("status", {"catalog": CATALOG})
        if CATALOG not in {row["id"] for row in status.get("catalogs", [])}:
            raise RuntimeError("owner did not create the Catalog")
        if current_value() != {"v": 1, "status": "draft"}:
            raise RuntimeError("owner seed does not match the requested value")

        run_role("producer", "producer", PRODUCER_TASK, "PRODUCER=proposed")
        state = request("GET", "/v1/_state")
        # GET /v1/_state exposes FileControlState.LoadBundle(), whose value is
        # already the catalog-id keyed map. The persisted control.json wraps
        # that map in {"catalogs": ...}; do not conflate the two shapes.
        proposals = state.get("control", {}).get(CATALOG, {}).get("proposals", {})
        if "PR-AGENT-1" not in proposals:
            raise RuntimeError("producer proposal is absent from control state")
        main_before_merge = current_value()
        if main_before_merge.get("v") != 1:
            raise RuntimeError("proposal moved main before review")

        run_role("reviewer", "reviewer", REVIEWER_TASK, "REVIEWER=merged")
        if current_value() != {"v": 2, "status": "governed"}:
            raise RuntimeError("reviewed proposal is not live on main")

        run_role("consumer", "consumer", CONSUMER_TASK, "CONSUMER=v2")
        run_role("auditor", "auditor", AUDITOR_TASK, "AUDITOR=verified")

        before_denial = post("status", {"catalog": CATALOG})
        head_before = next(row["head"] for row in before_denial["repos"] if row["id"] == REPO)
        run_role(
            "unauthorized",
            "mallory",
            UNAUTHORIZED_TASK,
            "UNAUTHORIZED=FORBIDDEN",
        )
        after_denial = post("status", {"catalog": CATALOG})
        head_after = next(row["head"] for row in after_denial["repos"] if row["id"] == REPO)
        if head_before != head_after or current_value().get("v") != 2:
            raise RuntimeError("unauthorized attempt changed authoritative state")

        evidence = {
            "catalog": CATALOG,
            "repository": REPO,
            "workspace": WORKSPACE,
            "object": OBJECT,
            "headBeforeUnauthorized": head_before,
            "headAfterUnauthorized": head_after,
            "value": current_value(),
            "audit": post("audit", {"catalog": CATALOG, "workspace": WORKSPACE}),
            "log": post("log", {"repo": REPO, "object": OBJECT, "ref": "refs/heads/main"}),
            "provenance": post(
                "provenance",
                {"repo": REPO, "object": OBJECT, "ref": "refs/heads/main"},
            ),
        }
        (ARTIFACTS / "oracle.json").write_text(json.dumps(evidence, indent=2) + "\n")
        print("PASS: empty workspace -> owner -> producer -> reviewer -> consumer -> auditor -> unauthorized")
        return 0
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
