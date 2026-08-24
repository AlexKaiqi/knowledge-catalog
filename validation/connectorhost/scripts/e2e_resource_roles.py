#!/usr/bin/env python3
"""Real DSH role conversations for knowledge collection and live resource access."""

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

EVIDENCE = Path(os.environ["RESOURCE_E2E_EVIDENCE"])
PROFILE = os.environ["DSH_PROFILE"]
MODEL_PATCH = Path(os.environ["DSH_MODEL_PATCH"])
INTEGRATION_REPO = Path(os.environ["INTEGRATION_REPO"])
REMOTE = os.environ["INTEGRATION_REMOTE"]
SOURCE = Path(os.environ["PAYMENT_OPS_SOURCE"])
KC_URL = os.environ["KC_SERVE"]
HOST_URL = os.environ["KC_RESOURCE_ACCESS_URL"]
HOST_BIN = os.environ["INTEGRATION_HOST_BIN"]
HOST_HOME = os.environ["INTEGRATION_HOST_HOME"]
HOST_LISTEN = os.environ["INTEGRATION_HOST_LISTEN"]
SUPERVISOR_URL = os.environ["INTEGRATION_SUPERVISOR_URL"]

CATALOG = "kr://demo/catalog"
REPO = "kr://demo/payments/operations"
WORKSPACE = "payments-agent"
SERVICE = "Service:payment-api"
RUNBOOK = "runbooks/payment-api"
DESCRIPTOR = "resource/traces/payment-api"


def request(method: str, url: str, body: dict | None = None) -> object:
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url, data=data, headers={"content-type": "application/json"}, method=method
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        payload = error.read().decode("utf-8", "replace")
        raise RuntimeError(f"{method} {url} HTTP {error.code}: {payload}") from error


def post(verb: str, flags: dict) -> object:
    return request("POST", f"{KC_URL}/v1/{verb}", flags)


def wait_health(url: str, timeout: float = 30) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            body = request("GET", f"{url}/health")
            if isinstance(body, dict) and body.get("ok"):
                return
        except Exception:
            time.sleep(0.2)
    raise RuntimeError(f"service did not become healthy: {url}")


def session_traces() -> set[Path]:
    dsh_home = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh")))
    return set((dsh_home / "sessions").glob("**/session.jsonl.zstd"))


def decode_trace(path: Path) -> tuple[list[str], list[str]]:
    decoded = subprocess.run(
        ["zstd", "-dc", str(path)], capture_output=True, text=True, check=True
    ).stdout
    tools: list[str] = []
    skills: list[str] = []
    for line in decoded.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "tool/call":
            continue
        data = event.get("data", {})
        name = str(data.get("name", ""))
        tools.append(name)
        if name != "skill":
            continue
        try:
            arguments = json.loads(data.get("arguments", "{}"))
        except (json.JSONDecodeError, TypeError):
            continue
        if isinstance(arguments.get("name"), str):
            skills.append(arguments["name"])
    return tools, skills


def run_role(
    name: str,
    principal: str | None,
    task: str,
    marker: str,
    cwd: Path | None = None,
    binding: bool = False,
    required_tools: tuple[str, ...] = (),
    required_skills: tuple[str, ...] = (),
) -> None:
    workdir = cwd or Path(tempfile.mkdtemp(prefix=f"dsh-resource-{name}-"))
    workdir.mkdir(parents=True, exist_ok=True)
    if binding:
        (workdir / ".dsh-loom-workspace.json").write_text(
            json.dumps({"version": 1, "catalog": CATALOG, "workspace": WORKSPACE})
            + "\n"
        )
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    if principal:
        env["KC_AS"] = principal
    else:
        env.pop("KC_AS", None)
    before = session_traces()
    proc = None
    for attempt in range(4):
        proc = subprocess.run(
            ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), task],
            cwd=workdir,
            env=env,
            capture_output=True,
            text=True,
            timeout=720,
        )
        if proc.returncode == 0 or proc.stdout.strip() or proc.stderr.strip() or attempt == 3:
            break
        print(f"{name}: empty DSH exit; reopening one clean role session")
    assert proc is not None
    (EVIDENCE / f"{name}.stdout.txt").write_text(proc.stdout)
    (EVIDENCE / f"{name}.stderr.txt").write_text(proc.stderr)
    print(f"{name}: exit={proc.returncode} tail={proc.stdout.strip()[-400:]}")
    markers = {line.strip() for line in proc.stdout.splitlines()}
    if proc.returncode != 0 or marker not in markers:
        raise RuntimeError(
            f"{name} failed; expected {marker}; stderr tail:\n{proc.stderr[-5000:]}"
        )
    candidates = session_traces() - before
    if not candidates:
        raise RuntimeError(f"{name} produced no DSH session trace")
    trace = max(candidates, key=lambda path: path.stat().st_mtime)
    tools, skills = decode_trace(trace)
    evidence = {"trace": str(trace), "tools": tools, "skills": skills}
    (EVIDENCE / f"{name}.trace.json").write_text(json.dumps(evidence, indent=2) + "\n")
    for tool in required_tools:
        if tool not in tools:
            raise RuntimeError(f"{name} did not call required DSH tool {tool}: {tools}")
    for skill in required_skills:
        if skill not in skills:
            raise RuntimeError(f"{name} did not load required DSH Skill {skill}: {skills}")


OWNER_TASK = f"""You are the Payments Knowledge Owner. Load the bundled
knowledge-catalog skill and use the kc tool for every Catalog action; do not run
a host kc command.

Create Catalog {CATALOG} and Repository {REPO}. Publish these two SOURCE knowledge
objects:
1. {RUNBOOK} with value
{{"title":"Payment API on-call","steps":["check live status","lookup the failing trace","page the current on-call"]}}
using command-id payments-runbook-v1, source-ref business://payments/runbook and
actor-ref payments-knowledge-owner.
2. {DESCRIPTOR} with value
{{"kind":"ResourceDescriptor","description":"Payment API live status and traces","runtime":"payment-ops","protocol":"resource-access/v1","access":{{"status":{{"call":"status","returns":["cut","status","owner","oncall"]}},"window":{{"call":"window","input":{{"from":"timestamp","to":"timestamp","cursor":"optional-string"}}}},"lookup":{{"call":"lookup","input":{{"traceId":"string"}}}}}}}}
using command-id payments-descriptor-v1, source-ref integration://payment-ops/descriptor
and actor-ref payments-knowledge-owner.

Define Workspace {WORKSPACE} revision 1 with source
{REPO}=refs/heads/main. Grant connector/payment-ops commit on {REPO} main. Grant
consumer read-workspace for Catalog {CATALOG}/Workspace {WORKSPACE}, and the
read, list and resolve commands on {REPO}. Grant auditor audit on {CATALOG}, and
read, log and provenance on {REPO}. Verify the consumer read-workspace and
connector commit grants with allowed. Reply exactly OWNER=ready.
"""

DEVELOPER_TASK = f"""You are the Payments Integration Developer working in the
authoritative integration repository. Load the bundled integration-development
skill and read its runtime contract. Use shell commands for repository files.

Develop connectors/payment-ops with connector.yaml, collector.py, access.py and
Python standard-library unit tests. The source location comes only from the
PAYMENT_OPS_SOURCE environment variable. The Collector reads the JSON source and
emits one FULL STATE reconcile Address: objectId {SERVICE}, aspectName observed,
target {REPO} refs/heads/main. Its value must contain exactly name, owner, oncall,
status and sourceRevision. Use the source's sourceRef and observedAt. Preserve
the prior Address/digest in nextCheckpoint.observed so update/delete is safe.

The same package must provide resource-access/v1 operations status, window and
lookup. status returns cut=source revision plus service status/owner/oncall;
lookup returns only the requested traceId; window filters trace timestamps and
returns nextCursor null. Every result includes the source revision as cut.

Declare manual plus schedule every 1s, a 30s Collector timeout, access timeout
30s, owner payments-platform, and Python unittest discovery. Tests must use a
temporary source fixture and cover Collector output, lookup and status. Run the
tests. The strict process ABI matters: a desired unit may contain only address,
value, sourceKey, pathHint and schemaRef--never observedAt. Read prior
Address/digests from the RunRequest field `checkpoint`, emit them as `observed`,
and return the new state as `nextCheckpoint`; do not read a nonexistent request
field named `nextCheckpoint`. Compute each observed digest as lowercase SHA-256
of canonical compact sorted JSON of the value alone, matching KC CanonicalDigest.
The desired unit must contain exactly address, value and sourceKey; omit
pathHint and schemaRef so Writer chooses its canonical readable path.

Then git add the new package, commit with message 'add payment ops
integration' using per-command test identity, and push main to origin. Do not
write the ResourceDescriptor or invoke KC. Reply exactly
INTEGRATION_DEVELOPED=payment-ops.
"""

OPERATOR_TASK = f"""You are the Integration Runtime Operator. Use shell commands
to deploy the already-pushed payment-ops package. Do not edit package code and
do not call kc directly.

Run:
1. {HOST_BIN} --home {HOST_HOME} repo-set --repo {REMOTE} --ref refs/heads/main
   --sync-every 1s --kc-url {KC_URL}
2. validate payment-ops
3. preview payment-ops and require PREVIEWED with one addition
4. activate payment-ops
5. Ask the platform runtime supervisor to start its preconfigured Host by POSTing
   to {SUPERVISOR_URL}/start. The supervisor, not this DSH session, owns the
   process and its PAYMENT_OPS_SOURCE binding.

Do not perform a non-preview manual run: the activated scheduler must do the
first knowledge write automatically. Wait until {HOST_URL}/health succeeds,
then reply exactly RUNTIME=active.
"""

CONSUMER_INITIAL_TASK = f"""You are the Payments Knowledge Consumer in a DSH
session already bound to Workspace {WORKSPACE}. Load the knowledge-catalog skill.
Use the kc tool to resolve the Workspace and read {RUNBOOK} and {SERVICE}. Then
use the resource tool with Descriptor {DESCRIPTOR}: call status and lookup for
trace-001. Do not use shell, do not write knowledge, and do not invent an
endpoint. Verify the knowledge owner is payments-platform, live status is
healthy and trace-001 status is OK. Reply exactly CONSUMER_INITIAL=verified.
"""

SOURCE_OWNER_TASK = f"""You are the external Payment Operations source owner.
The source-of-truth file is {SOURCE}. Use shell commands to update that external
JSON file without touching the integration Git repo or Knowledge Catalog.
Change revision to ops-r2 and observedAt to 2026-08-24T10:00:00Z; change the
service owner to payments-sre, oncall to pay-incident and status to degraded;
preserve trace-001 and append trace-002 at 2026-08-24T09:59:00Z with status ERROR
and summary 'payment authorization timeout'. Validate the resulting JSON. Do not
run the Collector or runtime manually. Reply exactly SOURCE=ops-r2.
"""

CONSUMER_UPDATED_TASK = f"""You are a new Payments Knowledge Consumer session
bound to Workspace {WORKSPACE}. Load the knowledge-catalog skill. Resolve the
Workspace afresh and read {SERVICE}. Then use the resource tool with Descriptor
{DESCRIPTOR} to call status and lookup trace-002. Verify both collected knowledge
and live status show owner payments-sre, oncall pay-incident, status degraded,
and the live trace reports ERROR with summary payment authorization timeout.
Do not write. Reply exactly CONSUMER_UPDATED=verified.
"""

AUDITOR_TASK = f"""You are the Auditor. Load the knowledge-catalog skill. Use the
kc tool to inspect provenance and log for {SERVICE} in {REPO} at
refs/heads/main. Verify SOURCE provenance points to
payments://operations/payment-api and there are two collected revisions. Then
read {HOST_URL}/api/access-traces with curl and verify consumer accesses record
the pinned Descriptor object/repository/commit, runtime generation, principal,
Agent preset and DSH session. Do not mutate anything. Reply exactly
AUDITOR=closed-loop.
"""


def read_service() -> object:
    return post("read", {"repo": REPO, "ref": "refs/heads/main", "object": SERVICE})


def wait_for_service(*needles: str, timeout: float = 40) -> object:
    deadline = time.monotonic() + timeout
    last: object = {}
    while time.monotonic() < deadline:
        try:
            last = read_service()
            encoded = json.dumps(last, sort_keys=True)
            if all(needle in encoded for needle in needles):
                return last
        except Exception:
            pass
        time.sleep(0.5)
    raise RuntimeError(f"collected knowledge did not reach {needles}: {last}")


def main() -> int:
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    try:
        run_role(
            "owner", None, OWNER_TASK, "OWNER=ready",
            required_tools=("skill", "kc"), required_skills=("knowledge-catalog",),
        )
        wait_health(KC_URL)
        run_role(
            "developer", None, DEVELOPER_TASK, "INTEGRATION_DEVELOPED=payment-ops",
            cwd=INTEGRATION_REPO, required_tools=("skill",),
            required_skills=("integration-development",),
        )
        run_role("operator", None, OPERATOR_TASK, "RUNTIME=active")
        wait_health(HOST_URL)
        initial = wait_for_service("payments-platform", "healthy", "ops-r1")
        run_role(
            "consumer-initial", "consumer", CONSUMER_INITIAL_TASK,
            "CONSUMER_INITIAL=verified", binding=True,
            required_tools=("skill", "kc", "resource"),
            required_skills=("knowledge-catalog",),
        )
        run_role("source-owner", None, SOURCE_OWNER_TASK, "SOURCE=ops-r2", cwd=SOURCE.parent)
        updated = wait_for_service("payments-sre", "degraded", "ops-r2")
        run_role(
            "consumer-updated", "consumer", CONSUMER_UPDATED_TASK,
            "CONSUMER_UPDATED=verified", binding=True,
            required_tools=("skill", "kc", "resource"),
            required_skills=("knowledge-catalog",),
        )
        run_role(
            "auditor", "auditor", AUDITOR_TASK, "AUDITOR=closed-loop",
            required_tools=("skill", "kc"), required_skills=("knowledge-catalog",),
        )

        runs = request("GET", f"{HOST_URL}/api/connectors/payment-ops/runs?limit=500")
        traces = request("GET", f"{HOST_URL}/api/access-traces?limit=20")
        provenance = post("provenance", {"repo": REPO, "ref": "refs/heads/main", "object": SERVICE})
        log = post("log", {"repo": REPO, "ref": "refs/heads/main", "object": SERVICE})
        if not isinstance(traces, list) or len(traces) < 4:
            raise RuntimeError(f"expected at least four live access traces: {traces}")
        if not all(trace.get("descriptor", {}).get("commit") for trace in traces):
            raise RuntimeError(f"access trace lost Descriptor commit: {traces}")
        successful = [run for run in runs if run.get("outcome") == "SUCCEEDED"] if isinstance(runs, list) else []
        if len(successful) < 2:
            raise RuntimeError(f"scheduler did not collect both source revisions: {runs}")
        oracle = {
            "scenario": "payment-api-operations",
            "roles": ["owner", "developer", "operator", "source-owner", "consumer", "auditor"],
            "initialKnowledge": initial,
            "updatedKnowledge": updated,
            "collectorRuns": runs,
            "resourceAccessTraces": traces,
            "provenance": provenance,
            "log": log,
        }
        (EVIDENCE / "oracle.json").write_text(json.dumps(oracle, indent=2) + "\n")
        print("PASS: DSH roles developed, published, auto-collected, accessed and audited payment operations")
        return 0
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
