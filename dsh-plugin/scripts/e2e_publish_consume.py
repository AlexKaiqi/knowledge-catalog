#!/usr/bin/env python3
"""Publisher + consumer + skill e2e against a live kc serve and real DSH."""

from __future__ import annotations

import base64
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

PLUGIN = Path(__file__).resolve().parent.parent
MODEL_PATCH = Path(
    os.environ.get(
        "DSH_MODEL_PATCH", str(PLUGIN / "scripts" / "deepseek-official.patch.yml")
    )
)
BASE = os.environ.get("KC_SERVE", "http://127.0.0.1:17380")
PROFILE = os.environ.get("DSH_PROFILE", "loom-e2e")
IS_DOLT_RUN = os.environ.get("KC_E2E_TOPOLOGY") == "dolt"
AGENT_READY_TIMEOUT = 600 if IS_DOLT_RUN else 300
AGENT_FINISH_TIMEOUT = 900 if IS_DOLT_RUN else 360
NOTES_LINE = "gmv is daily gross merchandise value"
ORION_CODE = "ORION-NEBULA-731"
ORION_SOURCE = "refs/semantic/governance/audit/window-17.md"
SKILL_BODY = """---
name: notes-ops
description: Read the shared analysis note.
---
Read /analysis/notes.md with filesystem Read.
"""
PUBLISH_TASK = f"""Publish two files to the virtual Workspace. Act immediately; do not discuss
formatting or whitespace. Use exactly two filesystem Write calls and never use Shell.

First write /analysis/notes.md with content `{NOTES_LINE}`.
Then write /.dsh/skills/notes-ops/SKILL.md with this short valid skill content:
{SKILL_BODY}After both Write calls succeed, reply exactly: PUBLISHED=ok
"""
CONSUME_TASK = """You are a knowledge consumer in a virtual multi-repository workspace.
Find the release control code for Project Orion from evidence in the mounted workspace.
You do not know the evidence path. Use the filesystem glob or grep tool to discover it,
then use the filesystem Read tool to verify the source. Do not use Shell and do not write.
Reply with exactly two space-separated fields:
ANSWER=<the-code> SOURCE=<workspace-relative-source-path>
"""
DEVELOP_TASK = """You are a developer working in a virtual Workspace with a disposable shell mirror.
Read /dev/TASK.md and the referenced source and test files with filesystem Read.
Make the required source change using filesystem Edit, not shell redirection or sed.
Then use Bash only to run: cd dev && go test ./...
Do not change the test. After the test passes, reply exactly: DEVELOPED=ok
"""
AUTHOR_TASK = """You are Author Agent A in an independent session.
Read /feedback/source.md. Create /feedback/release.md using filesystem Write.
This is a deliberately preliminary V1: include only the Alpha status in one sentence.
Do not create review feedback. Reply exactly: AUTHORED=V1
"""
REVIEW_TASK = """You are an independent Feedback Agent.
Read /feedback/spec.md, /feedback/source.md, and /feedback/release.md.
Identify what V1 is missing, then write actionable feedback to /feedback/review.md
using filesystem Write. Do not edit release.md. Reply exactly: REVIEWED=ok
"""
REVISE_TASK = """You are Agent B in a new session with no memory of the author or reviewer.
Read /feedback/spec.md, /feedback/source.md, /feedback/release.md, and /feedback/review.md.
Use filesystem Edit or Write to revise /feedback/release.md so it satisfies every spec item
and the persisted feedback. Do not delete review.md. Reply exactly: REVISED=V2
"""
RACE_TASK = """You are testing recoverable multi-repository development.
1. Read /race/root.txt and /refs/semantic/race/shared.txt with filesystem Read.
2. After both reads, use Bash to run exactly: touch agent-ready && sleep 8
3. Append a new line `agent-root` to /race/root.txt with filesystem Edit.
4. Append a new line `agent-semantic` to /refs/semantic/race/shared.txt with filesystem Edit.
The second edit may report that the file changed since your read. If so, re-read that file,
preserve the external line, and retry your Edit against the new content.
After both authoritative writes succeed, reply exactly: RECOVERED=ok
"""
PIN_TASK = """You are verifying reproducible Workspace reads.
Read /updates/version.txt and remember its exact first line.
Then use Bash to run exactly: touch pin-ready && sleep 8
After the command completes, read /updates/version.txt again with filesystem Read.
Reply exactly: PINNED=<first-read>-><second-read>
"""
LIVE_TASK = """You are a new independent Agent session.
Read /updates/version.txt with filesystem Read and reply exactly: LIVE=<first-line>
"""
REVOKE_TASK = """You are verifying immediate authorization revocation.
Read /auth/secret.txt with filesystem Read.
Then use Bash to run exactly: touch revoke-ready && sleep 8
After the command completes, try to read /auth/secret.txt again.
The second filesystem Read must be denied. When you observe the denial, reply exactly: REVOKED=ok
Do not use the shell mirror to bypass the filesystem tool.
"""
KEEPER_TASK = """Read /auth/secret.txt with filesystem Read.
Reply exactly: KEEPER=<first-line>
"""
GOV_OBJECT_PATH = "refs/semantic/objects/policy/release.json"
GOV_PIN_TASK = f"""You are a consumer holding one reproducible Workspace pin.
Read /{GOV_OBJECT_PATH} and extract the JSON body's version field.
Then use Bash to run this exact command with timeoutMs 300000:
touch governance-ready && while [ ! -f governance-done ]; do sleep 1; done
After the command completes, read /{GOV_OBJECT_PATH} again.
Reply exactly: GOVPIN=<first-version>-><second-version>
"""
MAINTAINER_TASK = f"""You are a Maintainer Agent preparing a governed update.
Read /governance/spec.md and /{GOV_OBJECT_PATH}.
Write /governance/candidate.json as strict JSON with exactly these fields:
version, message, approved. Set version to V2, message to the spec's required message,
and approved to false. Do not edit the published object. Reply exactly: CANDIDATE=V2
"""
GOV_REVIEW_TASK = """You are an independent governance Reviewer Agent.
Read /governance/spec.md, /governance/candidate.json, and /governance/preview.json.
If the candidate matches the spec and preview names PR-agent, write /governance/review.json
as strict JSON with fields suite=`agent-review`, outcome=`PASSED`, and a non-empty evidence string.
Do not edit candidate.json. Reply exactly: EVIDENCE=PASSED
"""
GOV_LIVE_TASK = f"""You are a new consumer after governed publication.
Read /{GOV_OBJECT_PATH}, extract the JSON body's version, and reply exactly: GOVLIVE=<version>
"""
QUERY_TASK = """Verify every Workspace query primitive. You must perform all five calls:
1. list with path `query`
2. glob with pattern `**/*.md` and path `query`
3. grep for `NEEDLE-ALPHA` under `query`
4. rg for `NEEDLE-BETA` under `query`
5. filesystem Read of `query/nested/evidence.md`
Use the evidence file's FINAL value and reply exactly: QUERY=<value>
Do not use Bash.
"""
SOURCE_A_PATH = "refs/semantic/objects/Table:source.a/structure.json"
SOURCE_B_PATH = "refs/semantic/objects/Table:source.b/structure.json"
SOURCE_C_PATH = "refs/semantic/objects/Table:source.c/structure.json"
CONNECTOR_PIN_TASK = f"""You are consuming a SOURCE mirror through one reproducible Workspace pin.
Read /{SOURCE_A_PATH}. Extract the JSON body's version and the provenance sourceRefs value.
Then use Bash to run this exact command with timeoutMs 300000:
touch connector-ready && while [ ! -f connector-done ]; do sleep 1; done
After it completes, read /{SOURCE_A_PATH} again. Reply exactly:
CONNPIN=<first-version>-><second-version> SOURCE=<first-source-ref>
"""
CONNECTOR_LIVE_TASK = f"""You are a new Agent task after an external source refresh.
Read /{SOURCE_A_PATH} and /{SOURCE_C_PATH}. Verify both JSON bodies have version V2
and their provenance sourceRefs is source://snapshot/S2. Try to read /{SOURCE_B_PATH}
and verify it is absent because the source removed it. Reply exactly:
CONNLIVE=A:V2+C:V2+B:REMOVED SOURCE=source://snapshot/S2
"""
DELETE_TASK = """You are completing a cleanup task in the mounted Workspace.
Read /cleanup/obsolete.txt with filesystem Read. Then use the kc tool exactly once with
verb `vfs-write` and flags containing workspace=`notes`, path=`cleanup/obsolete.txt`,
remove=true, and command-id=`agent-remove-obsolete`. After the tool reports success,
reply exactly: REMOVED=ok. Do not try to read the removed path again: this Agent task's
filesystem is intentionally pinned, and the acceptance oracle verifies the new live state.
"""


def post(verb: str, body: dict, as_principal: str | None = None) -> dict:
    req = urllib.request.Request(
        f"{BASE}/v1/{verb}",
        data=json.dumps(body).encode(),
        headers={"content-type": "application/json"},
        method="POST",
    )
    if as_principal:
        req.add_header("X-Kc-As", as_principal)
    try:
        with urllib.request.urlopen(req) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as e:
        payload = e.read().decode("utf-8", "replace")
        raise RuntimeError(f"{verb} HTTP {e.code}: {payload}") from e


def expect_error(verb: str, body: dict, code: str) -> None:
    try:
        post(verb, body)
    except RuntimeError as error:
        if code not in str(error):
            raise AssertionError(f"{verb} returned the wrong error: {error}") from error
        return
    raise AssertionError(f"{verb} unexpectedly succeeded; wanted {code}")


def save_evidence(name: str, value: object) -> None:
    home = os.environ.get("KC_E2E_HOME")
    if not home:
        return
    (Path(home) / name).write_text(
        json.dumps(value, indent=2, sort_keys=True) + "\n"
    )


def vfs_read(path: str, as_principal: str | None = None) -> str:
    payload = post("vfs-read", {"workspace": "notes", "path": path}, as_principal)
    return base64.b64decode(payload.get("content") or "").decode()


def vfs_write(path: str, content: str, command_id: str) -> dict:
    return post(
        "vfs-write",
        {
            "workspace": "notes",
            "command-id": command_id,
            "path": path,
            "content": base64.b64encode(content.encode()).decode(),
        },
    )


def run_agent(task: str, as_principal: str | None, workdir: Path) -> tuple[int, str, str]:
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_SERVE"] = BASE
    env["KC_WORKSPACE"] = "notes"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    if as_principal:
        env["KC_AS"] = as_principal
    else:
        env.pop("KC_AS", None)
    cmd = ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), task]
    try:
        proc = subprocess.run(
            cmd,
            cwd=str(workdir),
            env=env,
            capture_output=True,
            text=True,
            timeout=AGENT_FINISH_TIMEOUT,
        )
    except subprocess.TimeoutExpired as error:
        stdout = error.stdout if isinstance(error.stdout, str) else ""
        stderr = error.stderr if isinstance(error.stderr, str) else ""
        return 124, stdout, stderr + f"\nAgent task timed out after {AGENT_FINISH_TIMEOUT} seconds"
    return proc.returncode, proc.stdout, proc.stderr


def run_racing_agent(workdir: Path) -> tuple[int, str, str]:
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_SERVE"] = BASE
    env["KC_WORKSPACE"] = "notes"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    env.pop("KC_AS", None)
    cmd = ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), RACE_TASK]
    proc = subprocess.Popen(
        cmd, cwd=str(workdir), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    marker = workdir / "agent-ready"
    deadline = time.monotonic() + AGENT_READY_TIMEOUT
    while time.monotonic() < deadline and proc.poll() is None and not marker.exists():
        time.sleep(0.1)
    if not marker.exists():
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "race marker was not created before timeout\n" + err
    vfs_write(
        "refs/semantic/race/shared.txt",
        "semantic-base\nexternal-change\n",
        "external-race-advance",
    )
    try:
        out, err = proc.communicate(timeout=AGENT_FINISH_TIMEOUT)
    except subprocess.TimeoutExpired:
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "racing agent timed out\n" + err
    return proc.returncode, out, err


def run_pinned_agent(workdir: Path) -> tuple[int, str, str]:
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_SERVE"] = BASE
    env["KC_WORKSPACE"] = "notes"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    env.pop("KC_AS", None)
    cmd = ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), PIN_TASK]
    proc = subprocess.Popen(
        cmd, cwd=str(workdir), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    marker = workdir / "pin-ready"
    deadline = time.monotonic() + AGENT_READY_TIMEOUT
    while time.monotonic() < deadline and proc.poll() is None and not marker.exists():
        time.sleep(0.1)
    if not marker.exists():
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "pin marker was not created before timeout\n" + err
    vfs_write("updates/version.txt", "V2\n", "external-publish-v2")
    try:
        out, err = proc.communicate(timeout=AGENT_FINISH_TIMEOUT)
    except subprocess.TimeoutExpired:
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "pinned agent timed out\n" + err
    return proc.returncode, out, err


def run_revoked_agent(workdir: Path, rule_id: str) -> tuple[int, str, str]:
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_SERVE"] = BASE
    env["KC_WORKSPACE"] = "notes"
    env["KC_WORKSPACE_DIR"] = str(workdir)
    env["KC_AS"] = "revokee"
    cmd = ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), REVOKE_TASK]
    proc = subprocess.Popen(
        cmd, cwd=str(workdir), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    marker = workdir / "revoke-ready"
    deadline = time.monotonic() + AGENT_READY_TIMEOUT
    while time.monotonic() < deadline and proc.poll() is None and not marker.exists():
        time.sleep(0.1)
    if not marker.exists():
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "revoke marker was not created before timeout\n" + err
    post("revoke", {"id": rule_id})
    try:
        out, err = proc.communicate(timeout=AGENT_FINISH_TIMEOUT)
    except subprocess.TimeoutExpired:
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        return 1, out, "revoked agent timed out\n" + err
    return proc.returncode, out, err


def redact(text: str, env: dict[str, str]) -> str:
    out = text
    for secret in (env.get("OPENAI_API_KEY") or "",):
        if secret:
            out = out.replace(secret, "***")
    return out


def accept_race(race_dir: Path, env: dict[str, str]) -> int:
    print("==> multi-repository stale-write recovery agent")
    code, out, err = run_racing_agent(race_dir)
    print("racing agent stdout:", out.strip())
    if code != 0 or "RECOVERED=ok" not in out:
        print("racing agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    root_result = vfs_read("race/root.txt")
    semantic_result = vfs_read("refs/semantic/race/shared.txt")
    if "agent-root" not in root_result:
        print("FAIL: first repository write did not remain applied", repr(root_result), file=sys.stderr)
        return 1
    if "external-change" not in semantic_result or "agent-semantic" not in semantic_result:
        print("FAIL: stale recovery lost an external or agent change", repr(semantic_result), file=sys.stderr)
        return 1
    sessions = Path.home() / ".dsh" / "sessions"
    traces = [p for p in sessions.rglob("session.jsonl.zstd") if "dsh-race-" in str(p)]
    if not traces:
        print("FAIL: racing agent transcript not found", file=sys.stderr)
        return 1
    trace = max(traces, key=lambda p: p.stat().st_mtime)
    decoded = subprocess.run(["zstd", "-dc", str(trace)], capture_output=True, text=True)
    if decoded.returncode != 0 or "FS_STALE_VERSION" not in decoded.stdout:
        print("FAIL: transcript has no machine-routable stale-write evidence", file=sys.stderr)
        return 1
    return 0


def accept_pin(pin_dir: Path, live_dir: Path, env: dict[str, str]) -> int:
    print("==> frozen pin and new-session update visibility")
    code, out, err = run_pinned_agent(pin_dir)
    print("pinned agent stdout:", out.strip())
    if code != 0 or "PINNED=V1->V1" not in out:
        print("pinned agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    if vfs_read("updates/version.txt") != "V2\n":
        print("FAIL: external V2 was not published", file=sys.stderr)
        return 1
    code, out, err = run_agent(LIVE_TASK, None, live_dir)
    print("new-session agent stdout:", out.strip())
    if code != 0 or "LIVE=V2" not in out:
        print("new-session agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    return 0


def accept_revoke(revoke_dir: Path, keeper_dir: Path, env: dict[str, str]) -> int:
    print("==> immediate revocation without collateral grant loss")
    workspace_rule = post(
        "allow",
        {"principal": "revokee", "cmd": "read-workspace", "catalog": "kr://acme/catalog", "workspace": "notes"},
    )
    post("allow", {"principal": "revokee", "cmd": "read", "repo": "kr://acme/personals/alice"})
    post("allow", {"principal": "revokee", "cmd": "read", "repo": "kr://acme/shared/semantic"})
    post(
        "allow",
        {"principal": "keeper", "cmd": "read-workspace", "catalog": "kr://acme/catalog", "workspace": "notes"},
    )
    post("allow", {"principal": "keeper", "cmd": "read", "repo": "kr://acme/personals/alice"})
    post("allow", {"principal": "keeper", "cmd": "read", "repo": "kr://acme/shared/semantic"})
    rule_id = str(workspace_rule.get("id") or "")
    if not rule_id:
        print("FAIL: allow returned no rule id", workspace_rule, file=sys.stderr)
        return 1
    code, out, err = run_revoked_agent(revoke_dir, rule_id)
    print("revoked agent stdout:", out.strip())
    if code != 0 or "REVOKED=ok" not in out:
        print("revoked agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    try:
        vfs_read("auth/secret.txt", "revokee")
        print("FAIL: revoked principal still reads through HTTP", file=sys.stderr)
        return 1
    except RuntimeError as error:
        if "FORBIDDEN" not in str(error):
            print("FAIL: revoked HTTP read returned wrong error", error, file=sys.stderr)
            return 1
    code, out, err = run_agent(KEEPER_TASK, "keeper", keeper_dir)
    print("keeper agent stdout:", out.strip())
    if code != 0 or "KEEPER=ACCESS-GRANTED" not in out:
        print("keeper agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    return 0


def accept_governance(
    gov_pin_dir: Path,
    maintainer_dir: Path,
    gov_review_dir: Path,
    gov_live_dir: Path,
    env: dict[str, str],
) -> int:
    print("==> governed Agent candidate, evidence gate, merge, and old-pin replay")
    old_env = os.environ.copy()
    old_env["DSH_PERMISSION_MODE"] = "danger-full-access"
    old_env["KC_SERVE"] = BASE
    old_env["KC_WORKSPACE"] = "notes"
    old_env["KC_WORKSPACE_DIR"] = str(gov_pin_dir)
    old_env.pop("KC_AS", None)
    old_cmd = ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), GOV_PIN_TASK]
    old = subprocess.Popen(
        old_cmd,
        cwd=str(gov_pin_dir),
        env=old_env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    ready = gov_pin_dir / "governance-ready"
    done = gov_pin_dir / "governance-done"
    deadline = time.monotonic() + AGENT_READY_TIMEOUT
    while time.monotonic() < deadline and old.poll() is None and not ready.exists():
        time.sleep(0.1)
    if not ready.exists():
        old.terminate()
        out, err = old.communicate(timeout=15)
        print("old-pin agent failed to become ready", out, redact(err, env), file=sys.stderr)
        return 1
    published = False
    try:
        code, out, err = run_agent(MAINTAINER_TASK, None, maintainer_dir)
        print("maintainer agent stdout:", out.strip())
        if code != 0 or "CANDIDATE=V2" not in out:
            print("maintainer agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        try:
            candidate = json.loads(vfs_read("governance/candidate.json"))
        except (ValueError, RuntimeError) as error:
            print("FAIL: candidate is not strict authoritative JSON", error, file=sys.stderr)
            return 1
        if candidate != {
            "version": "V2",
            "message": "Production rollout requires two approvals.",
            "approved": False,
        }:
            print("FAIL: candidate does not match governance spec", candidate, file=sys.stderr)
            return 1

        post(
            "gate-add",
            {"on": "merge", "repo": "kr://acme/shared/semantic", "require": "suite:agent-review"},
        )
        proposal = post(
            "propose",
            {
                "proposal-id": "PR-agent",
                "repo": "kr://acme/shared/semantic",
                "target": "refs/heads/main",
                "candidate": "refs/heads/candidates/PR-agent",
                "object": "policy/release",
                "value": candidate,
            },
        )
        preview = post(
            "preview", {"proposal": proposal["proposalId"], "workspace": "notes"}
        )
        vfs_write(
            "governance/preview.json",
            json.dumps(
                {
                    "proposalId": proposal["proposalId"],
                    "previewId": preview["previewId"],
                    "candidateCommit": proposal["candidateCommit"],
                },
                separators=(",", ":"),
            )
            + "\n",
            "publish-agent-preview",
        )

        code, out, err = run_agent(GOV_REVIEW_TASK, None, gov_review_dir)
        print("governance reviewer stdout:", out.strip())
        if code != 0 or "EVIDENCE=PASSED" not in out:
            print("governance reviewer stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        try:
            review = json.loads(vfs_read("governance/review.json"))
        except (ValueError, RuntimeError) as error:
            print("FAIL: review evidence is not strict authoritative JSON", error, file=sys.stderr)
            return 1
        if review.get("suite") != "agent-review" or review.get("outcome") != "PASSED" or not review.get("evidence"):
            print("FAIL: invalid review evidence", review, file=sys.stderr)
            return 1

        try:
            post(
                "merge",
                {"proposal": proposal["proposalId"], "preview": preview["previewId"]},
            )
            print("FAIL: merge passed without the required validation report", file=sys.stderr)
            return 1
        except RuntimeError as error:
            if "GATE_UNSATISFIED" not in str(error):
                print("FAIL: missing evidence returned wrong error", error, file=sys.stderr)
                return 1
        validation = post(
            "record-validation",
            {
                "preview": preview["previewId"],
                "suite": review["suite"],
                "outcome": review["outcome"],
            },
        )
        post(
            "merge",
            {
                "proposal": proposal["proposalId"],
                "preview": preview["previewId"],
                "validation": validation["reportId"],
            },
        )
        published = True
    finally:
        done.touch()
        if not published and old.poll() is None:
            old.terminate()
            old.communicate(timeout=15)

    try:
        old_out, old_err = old.communicate(timeout=AGENT_FINISH_TIMEOUT)
    except subprocess.TimeoutExpired:
        old.terminate()
        old_out, old_err = old.communicate(timeout=15)
        print("old-pin governance agent timed out", redact(old_err, env), file=sys.stderr)
        return 1
    print("old-pin governance agent stdout:", old_out.strip())
    if old.returncode != 0 or "GOVPIN=V1->V1" not in old_out:
        print("old-pin governance agent stderr:", redact(old_err, env)[-4000:], file=sys.stderr)
        return 1
    code, out, err = run_agent(GOV_LIVE_TASK, None, gov_live_dir)
    print("new governance consumer stdout:", out.strip())
    if code != 0 or "GOVLIVE=V2" not in out:
        print("new governance consumer stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    if '"V2"' not in vfs_read(GOV_OBJECT_PATH):
        print("FAIL: merged Canonical object is not V2", file=sys.stderr)
        return 1
    return 0


def accept_query(query_dir: Path, env: dict[str, str]) -> int:
    print("==> list, glob, grep, rg, and exact Read through one pinned Workspace")
    code, out, err = run_agent(QUERY_TASK, None, query_dir)
    print("query agent stdout:", out.strip())
    if code != 0 or "QUERY=QUERY-OK" not in out:
        print("query agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    sessions = Path.home() / ".dsh" / "sessions"
    traces = [p for p in sessions.rglob("session.jsonl.zstd") if "dsh-query-" in str(p)]
    if not traces:
        print("FAIL: query agent transcript not found", file=sys.stderr)
        return 1
    trace = max(traces, key=lambda p: p.stat().st_mtime)
    decoded = subprocess.run(["zstd", "-dc", str(trace)], capture_output=True, text=True)
    if decoded.returncode != 0:
        print("FAIL: query transcript could not be decoded", file=sys.stderr)
        return 1
    for tool in ("list", "glob", "grep", "rg", "read"):
        if f'"name":"{tool}"' not in decoded.stdout:
            print(f"FAIL: query transcript has no {tool} tool call", file=sys.stderr)
            return 1
    if '"isError":true' in decoded.stdout:
        print("FAIL: at least one query primitive returned an error", file=sys.stderr)
        return 1
    return 0


def source_changeset(base: str, version: str) -> dict:
    address = lambda name: {
        "kind": "Aspect",
        "objectId": f"Table:source.{name}",
        "aspectName": "structure",
    }
    if version == "V1":
        operations = [
            {"op": "PUT", "address": address("a"), "value": {"name": "a", "version": "V1"}},
            {"op": "PUT", "address": address("b"), "value": {"name": "b", "version": "V1"}},
        ]
        source_ref = "source://snapshot/S1"
    else:
        operations = [
            {"op": "PUT", "address": address("a"), "value": {"name": "a", "version": "V2"}},
            {"op": "REMOVE", "address": address("b"), "reason": "absent-from-source"},
            {"op": "PUT", "address": address("c"), "value": {"name": "c", "version": "V2"}},
        ]
        source_ref = "source://snapshot/S2"
    return {
        "targetRepository": "kr://acme/shared/semantic",
        "targetRef": "refs/heads/main",
        "baseCommit": base,
        "expectedTargetCommit": base,
        "operations": operations,
        "message": f"connector source-structure {version}",
        "provenance": {
            "originKind": "SOURCE",
            "actorRef": "source-structure",
            "activityRef": "source-structure",
            "sourceRefs": [source_ref],
            "producedAt": "2026-08-23T02:00:00Z" if version == "V2" else "2026-08-23T01:00:00Z",
        },
    }


def accept_connector(old_dir: Path, live_dir: Path, env: dict[str, str]) -> int:
    print("==> connector SOURCE change, old pin, new Agent, and removal")
    resolved = post("resolve", {"workspace": "notes"})
    base = resolved["repositories"]["kr://acme/shared/semantic"]
    first = post(
        "commit",
        {"command-id": "connector:source-structure:S1", "changeset": source_changeset(base, "V1")},
    )
    old_env = os.environ.copy()
    old_env["DSH_PERMISSION_MODE"] = "danger-full-access"
    old_env["KC_SERVE"] = BASE
    old_env["KC_WORKSPACE"] = "notes"
    old_env["KC_WORKSPACE_DIR"] = str(old_dir)
    old_env.pop("KC_AS", None)
    proc = subprocess.Popen(
        ["dsh", "--profile", PROFILE, "--patch", str(MODEL_PATCH), CONNECTOR_PIN_TASK],
        cwd=str(old_dir), env=old_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    ready = old_dir / "connector-ready"
    done = old_dir / "connector-done"
    deadline = time.monotonic() + AGENT_READY_TIMEOUT
    while time.monotonic() < deadline and proc.poll() is None and not ready.exists():
        time.sleep(0.1)
    if not ready.exists():
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        print("connector old-pin Agent did not become ready", out, redact(err, env), file=sys.stderr)
        return 1
    try:
        current = post("resolve", {"workspace": "notes"})["repositories"]["kr://acme/shared/semantic"]
        second = post(
            "commit",
            {"command-id": "connector:source-structure:S2", "changeset": source_changeset(current, "V2")},
        )
        replay = post(
            "commit",
            {"command-id": "connector:source-structure:S2", "changeset": source_changeset(current, "V2")},
        )
        if replay.get("disposition") != "REPLAYED":
            print("FAIL: connector batch replay was not idempotent", replay, file=sys.stderr)
            return 1
        if first.get("disposition") != "APPLIED" or second.get("disposition") != "APPLIED":
            print("FAIL: connector commits did not apply", first, second, file=sys.stderr)
            return 1
    finally:
        done.touch()
    try:
        out, err = proc.communicate(timeout=AGENT_FINISH_TIMEOUT)
    except subprocess.TimeoutExpired:
        proc.terminate()
        out, err = proc.communicate(timeout=15)
        print("connector old-pin Agent timed out", redact(err, env), file=sys.stderr)
        return 1
    print("connector old-pin Agent stdout:", out.strip())
    if proc.returncode != 0 or "CONNPIN=V1->V1 SOURCE=source://snapshot/S1" not in out:
        print("connector old-pin Agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    code, out, err = run_agent(CONNECTOR_LIVE_TASK, None, live_dir)
    print("connector live Agent stdout:", out.strip())
    if code != 0 or "CONNLIVE=A:V2+C:V2+B:REMOVED SOURCE=source://snapshot/S2" not in out:
        print("connector live Agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    try:
        vfs_read(SOURCE_B_PATH)
        print("FAIL: connector-removed source B remains visible", file=sys.stderr)
        return 1
    except RuntimeError:
        pass
    if 'source://snapshot/S2' not in vfs_read(SOURCE_A_PATH):
        print("FAIL: V2 SOURCE provenance is missing", file=sys.stderr)
        return 1
    return 0


def accept_agent_remove(workdir: Path, env: dict[str, str]) -> int:
    print("==> Agent removes an authoritative Workspace file through Writer")
    code, out, err = run_agent(DELETE_TASK, None, workdir)
    print("remove Agent stdout:", out.strip())
    if code != 0 or "REMOVED=ok" not in out:
        print("remove Agent stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    try:
        vfs_read("cleanup/obsolete.txt")
        print("FAIL: Agent-reported removal left the file authoritative", file=sys.stderr)
        return 1
    except RuntimeError:
        return 0


def accept_product_boundaries() -> int:
    print("==> Workspace-only, routing, CAS, observability, checkout, and lifecycle boundaries")
    try:
        expect_error("define-view", {}, "USAGE_INVALID")
        expect_error(
            "read",
            {"workspace": "notes", "view": "legacy", "object": "anything"},
            "USAGE_INVALID",
        )
        expect_error(
            "define-workspace",
            {
                "workspace": "overlap-invalid",
                "revision": 1,
                "source": [
                    "kr://acme/personals/alice=refs/heads/main@",
                    "kr://acme/shared/semantic=refs/heads/main@nested",
                    "kr://acme/clone/reference=refs/heads/main@nested/child",
                ],
            },
            "WORKSPACE_INVALID",
        )
        post(
            "define-workspace",
            {
                "workspace": "no-root",
                "revision": 1,
                "source": ["kr://acme/clone/reference=refs/heads/main@owned"],
            },
        )
        expect_error(
            "vfs-write",
            {
                "workspace": "no-root",
                "command-id": "unowned-must-fail",
                "path": "outside.txt",
                "content": base64.b64encode(b"no\n").decode(),
            },
            "USAGE_INVALID",
        )
        expect_error(
            "vfs-read", {"workspace": "notes", "path": "analysis"},
            "KNOWLEDGE_REF_UNRESOLVED",
        )

        stable = {
            "workspace": "notes",
            "command-id": "boundary-idempotency",
            "path": "boundaries/idempotency.txt",
            "content": base64.b64encode(b"first\n").decode(),
        }
        post("vfs-write", stable)
        conflict = dict(stable)
        conflict["content"] = base64.b64encode(b"different\n").decode()
        expect_error("vfs-write", conflict, "IDEMPOTENCY_CONFLICT")

        pin = post("resolve", {"workspace": "notes"})
        old_base = pin["repositories"]["kr://acme/personals/alice"]
        vfs_write("boundaries/advance.txt", "advance\n", "boundary-advance")
        expect_error(
            "vfs-write",
            {
                "workspace": "notes",
                "command-id": "boundary-stale",
                "path": "boundaries/stale.txt",
                "base": old_base,
                "content": base64.b64encode(b"stale\n").decode(),
            },
            "NON_FAST_FORWARD",
        )
        expect_error(
            "vfs-read", {"workspace": "notes", "path": "boundaries/stale.txt"},
            "KNOWLEDGE_REF_UNRESOLVED",
        )

        status = post("status", {})
        inspected = post("inspect", {"workspace": "notes"})
        audited = post("audit", {})
        checked = post("checkout", {"workspace": "notes"})
        if not status.get("repos") or not inspected.get("pin") or not audited.get("entries"):
            raise AssertionError("status/inspect/audit did not expose operational evidence")
        if not checked.get("mounts"):
            raise AssertionError("checkout did not report per-mount capability")
        semantic_mount = next(
            (item for item in checked["mounts"] if item.get("repository") == "kr://acme/shared/semantic"),
            None,
        )
        topology = os.environ.get("KC_E2E_TOPOLOGY", "ordinary-git")
        if semantic_mount is None:
            raise AssertionError("checkout omitted the semantic mount")
        if topology in {"gitea", "dolt"} and not semantic_mount.get("skipped"):
            raise AssertionError(f"{topology} falsely claimed a local Git worktree: {semantic_mount!r}")
        if topology == "ordinary-git" and semantic_mount.get("skipped"):
            raise AssertionError(f"ordinary Git checkout was unexpectedly skipped: {semantic_mount!r}")

        post(
            "define-workspace",
            {
                "workspace": "lifecycle",
                "revision": 1,
                "source": ["kr://acme/clone/reference=refs/heads/main"],
            },
        )
        post("retire-workspace", {"workspace": "lifecycle"})
        expect_error("resolve", {"workspace": "lifecycle"}, "WORKSPACE_INVALID")
        post("archive-repo", {"repo": "kr://acme/clone/reference"})
        expect_error(
            "put",
            {
                "repo": "kr://acme/clone/reference",
                "command-id": "archived-write-must-fail",
                "object": "after/archive",
                "value": {"v": 1},
            },
            "REPOSITORY_ARCHIVED",
        )
        audit_before_archive = post("audit", {})
        post("archive-catalog", {})
        expect_error(
            "define-workspace",
            {
                "workspace": "after-catalog-archive",
                "revision": 1,
                "source": ["kr://acme/personals/alice=refs/heads/main"],
            },
            "CATALOG_ARCHIVED",
        )
        audit_after_archive = post("audit", {})
        if len(audit_after_archive.get("entries", [])) <= len(audit_before_archive.get("entries", [])):
            raise AssertionError("Catalog archive was not retained in readable audit history")
    except (AssertionError, RuntimeError, KeyError) as error:
        print("FAIL: product boundary oracle:", error, file=sys.stderr)
        return 1
    print("boundaries: all negative and lifecycle oracles passed")
    return 0


def accept_publisher(workdir: Path, env: dict[str, str]) -> int:
    print("==> publisher agent")
    code, out, err = run_agent(PUBLISH_TASK, None, workdir)
    print("publisher exit", code)
    print("publisher stdout:", out.strip())
    if code != 0 or "PUBLISHED=ok" not in out:
        print("publisher stderr:", redact(err, env)[-4000:], file=sys.stderr)
        return 1
    notes = vfs_read("analysis/notes.md")
    skill = vfs_read(".dsh/skills/notes-ops/SKILL.md")
    if NOTES_LINE not in notes:
        print("FAIL: publisher notes missing", repr(notes), file=sys.stderr)
        return 1
    if "name: notes-ops" not in skill:
        print("FAIL: publisher skill missing", repr(skill[:200]), file=sys.stderr)
        return 1
    print("publisher wrote analysis/notes.md and .dsh/skills/notes-ops/SKILL.md")
    return 0


def main() -> int:
    env = os.environ.copy()
    if not MODEL_PATCH.is_file():
        print(f"FAIL: DSH model patch does not exist: {MODEL_PATCH}", file=sys.stderr)
        return 1

    pub_dir = Path(tempfile.mkdtemp(prefix="dsh-pub-"))
    con_dir = Path(tempfile.mkdtemp(prefix="dsh-con-"))
    dev_dir = Path(tempfile.mkdtemp(prefix="dsh-dev-"))
    author_dir = Path(tempfile.mkdtemp(prefix="dsh-author-"))
    review_dir = Path(tempfile.mkdtemp(prefix="dsh-review-"))
    revise_dir = Path(tempfile.mkdtemp(prefix="dsh-revise-"))
    race_dir = Path(tempfile.mkdtemp(prefix="dsh-race-"))
    pin_dir = Path(tempfile.mkdtemp(prefix="dsh-pin-"))
    live_dir = Path(tempfile.mkdtemp(prefix="dsh-live-"))
    revoke_dir = Path(tempfile.mkdtemp(prefix="dsh-revoke-"))
    keeper_dir = Path(tempfile.mkdtemp(prefix="dsh-keeper-"))
    gov_pin_dir = Path(tempfile.mkdtemp(prefix="dsh-gov-pin-"))
    maintainer_dir = Path(tempfile.mkdtemp(prefix="dsh-maintainer-"))
    gov_review_dir = Path(tempfile.mkdtemp(prefix="dsh-gov-review-"))
    gov_live_dir = Path(tempfile.mkdtemp(prefix="dsh-gov-live-"))
    query_dir = Path(tempfile.mkdtemp(prefix="dsh-query-"))
    connector_old_dir = Path(tempfile.mkdtemp(prefix="dsh-connector-old-"))
    connector_live_dir = Path(tempfile.mkdtemp(prefix="dsh-connector-live-"))
    remove_dir = Path(tempfile.mkdtemp(prefix="dsh-remove-"))
    try:
        save_evidence(
            "acceptance.initial.json",
            {
                "status": post("status", {}),
                "pin": post("resolve", {"workspace": "notes"}),
                "audit": post("audit", {}),
            },
        )
        if os.environ.get("KC_E2E_ONLY") == "race":
            vfs_write("race/root.txt", "root-base\n", "seed-race-root")
            vfs_write(
                "refs/semantic/race/shared.txt", "semantic-base\n", "seed-race-semantic"
            )
            return accept_race(race_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "pin":
            vfs_write("updates/version.txt", "V1\n", "seed-update-v1")
            return accept_pin(pin_dir, live_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "revoke":
            vfs_write("auth/secret.txt", "ACCESS-GRANTED\n", "seed-auth-secret")
            return accept_revoke(revoke_dir, keeper_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "governance":
            post(
                "put",
                {
                    "command-id": "seed-governed-v1",
                    "repo": "kr://acme/shared/semantic",
                    "object": "policy/release",
                    "value": {"version": "V1", "message": "legacy", "approved": False},
                },
            )
            vfs_write(
                "governance/spec.md",
                "Candidate version must be V2.\n"
                "Required message: Production rollout requires two approvals.\n"
                "Candidate approved must remain false until merge.\n",
                "seed-governance-spec",
            )
            return accept_governance(
                gov_pin_dir, maintainer_dir, gov_review_dir, gov_live_dir, env
            )
        if os.environ.get("KC_E2E_ONLY") == "query":
            vfs_write("query/alpha.md", "NEEDLE-ALPHA\n", "seed-query-alpha")
            vfs_write("query/beta.txt", "NEEDLE-BETA\n", "seed-query-beta")
            vfs_write(
                "query/nested/evidence.md", "FINAL=QUERY-OK\n", "seed-query-evidence"
            )
            return accept_query(query_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "connector":
            return accept_connector(connector_old_dir, connector_live_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "remove":
            vfs_write("cleanup/obsolete.txt", "remove me\n", "seed-agent-remove")
            return accept_agent_remove(remove_dir, env)
        if os.environ.get("KC_E2E_ONLY") == "boundaries":
            vfs_write("analysis/file.txt", "directory witness\n", "seed-boundary-directory")
            return accept_product_boundaries()
        if os.environ.get("KC_E2E_ONLY") == "publisher":
            return accept_publisher(pub_dir, env)
        if accept_publisher(pub_dir, env) != 0:
            return 1

        print("==> seed unknown-path evidence in nested semantic mount")
        vfs_write(
            ORION_SOURCE,
            "Project Orion release control code: " + ORION_CODE + "\n"
            "This code is authoritative for the August launch window.\n",
            "seed-orion-policy",
        )
        vfs_write(
            "analysis/orion-brief.md",
            "Project Orion uses a release control code defined by the governance team.\n"
            "Find the authoritative value in the mounted semantic knowledge.\n",
            "seed-orion-brief",
        )
        vfs_write(
            "dev/TASK.md",
            "Fix calc.Add so it returns the sum of a and b. Do not modify the test.\n"
            "Acceptance: run `go test ./...` from the dev directory.\n",
            "seed-dev-task",
        )
        vfs_write(
            "dev/go.mod",
            "module example.com/workspace-dev\n\ngo 1.23\n",
            "seed-dev-module",
        )
        vfs_write(
            "dev/calc/calc.go",
            "package calc\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n",
            "seed-dev-source",
        )
        vfs_write(
            "dev/calc/calc_test.go",
            "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n"
            "\tif got := Add(7, 5); got != 12 { t.Fatalf(\"Add(7, 5) = %d, want 12\", got) }\n}\n",
            "seed-dev-test",
        )
        vfs_write(
            "feedback/source.md",
            "Alpha rollout is ready for production.\n"
            "Beta rollout requires security approval before production.\n",
            "seed-feedback-source",
        )
        vfs_write(
            "feedback/spec.md",
            "The release note must state Alpha readiness and Beta's security approval requirement.\n"
            "It must not contain TBD or claim that Beta is ready.\n",
            "seed-feedback-spec",
        )
        vfs_write("race/root.txt", "root-base\n", "seed-race-root")
        vfs_write(
            "refs/semantic/race/shared.txt",
            "semantic-base\n",
            "seed-race-semantic",
        )
        vfs_write("updates/version.txt", "V1\n", "seed-update-v1")
        vfs_write("auth/secret.txt", "ACCESS-GRANTED\n", "seed-auth-secret")
        post(
            "put",
            {
                "command-id": "seed-governed-v1",
                "repo": "kr://acme/shared/semantic",
                "object": "policy/release",
                "value": {"version": "V1", "message": "legacy", "approved": False},
            },
        )
        vfs_write(
            "governance/spec.md",
            "Candidate version must be V2.\n"
            "Required message: Production rollout requires two approvals.\n"
            "Candidate approved must remain false until merge.\n",
            "seed-governance-spec",
        )
        vfs_write("query/alpha.md", "NEEDLE-ALPHA\n", "seed-query-alpha")
        vfs_write("query/beta.txt", "NEEDLE-BETA\n", "seed-query-beta")
        vfs_write(
            "query/nested/evidence.md", "FINAL=QUERY-OK\n", "seed-query-evidence"
        )
        vfs_write("cleanup/obsolete.txt", "remove me\n", "seed-agent-remove")
        if ORION_CODE not in vfs_read(ORION_SOURCE):
            print("FAIL: nested semantic evidence missing", file=sys.stderr)
            return 1

        print("==> grant consumer bob read-only")
        post("allow", {"principal": "bob", "cmd": "read-workspace", "catalog": "kr://acme/catalog", "workspace": "notes"})
        post("allow", {"principal": "bob", "cmd": "read", "repo": "kr://acme/personals/alice"})
        post("allow", {"principal": "bob", "cmd": "read", "repo": "kr://acme/shared/semantic"})

        print("==> consumer vfs-write must be FORBIDDEN")
        try:
            post(
                "vfs-write",
                {
                    "workspace": "notes",
                    "command-id": "bob-should-fail",
                    "path": "analysis/hacked.md",
                    "content": base64.b64encode(b"nope").decode(),
                },
                as_principal="bob",
            )
            print("FAIL: bob vfs-write was allowed", file=sys.stderr)
            return 1
        except RuntimeError as e:
            if "FORBIDDEN" not in str(e):
                print("FAIL: expected FORBIDDEN, got", e, file=sys.stderr)
                return 1
            print("bob write blocked")

        print("==> consumer agent")
        code, out, err = run_agent(CONSUME_TASK, "bob", con_dir)
        Path("/tmp/kc-loom-e2e-con.out").write_text(out)
        Path("/tmp/kc-loom-e2e-con.err").write_text(redact(err, env))
        print("consumer exit", code)
        print("consumer stdout:", out.strip())
        expected = f"ANSWER={ORION_CODE} SOURCE={ORION_SOURCE}"
        if code != 0 or expected not in out:
            print("consumer stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        if list(con_dir.rglob("hacked.md")) or list(pub_dir.rglob("hacked.md")):
            print("FAIL: consumer wrote on the host", file=sys.stderr)
            return 1

        before = post("vfs-list", {"workspace": "notes"})
        before_by_path = {row["path"]: row["commit"] for row in before.get("entries", [])}

        print("==> developer agent edits through VFS and tests in disposable mirror")
        code, out, err = run_agent(DEVELOP_TASK, None, dev_dir)
        Path("/tmp/kc-loom-e2e-dev.out").write_text(out)
        Path("/tmp/kc-loom-e2e-dev.err").write_text(redact(err, env))
        print("developer exit", code)
        print("developer stdout:", out.strip())
        if code != 0 or "DEVELOPED=ok" not in out:
            print("developer stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        source = vfs_read("dev/calc/calc.go")
        test = vfs_read("dev/calc/calc_test.go")
        if "return a + b" not in source or "got := Add(7, 5)" not in test:
            print("FAIL: authoritative development result is wrong", file=sys.stderr)
            return 1
        oracle_dir = Path(tempfile.mkdtemp(prefix="kc-dev-oracle-"))
        try:
            (oracle_dir / "calc").mkdir()
            (oracle_dir / "go.mod").write_text(vfs_read("dev/go.mod"))
            (oracle_dir / "calc" / "calc.go").write_text(source)
            (oracle_dir / "calc" / "calc_test.go").write_text(test)
            oracle = subprocess.run(
                ["go", "test", "./..."], cwd=oracle_dir, capture_output=True, text=True
            )
            if oracle.returncode != 0:
                print("FAIL: independent development oracle failed", oracle.stdout, oracle.stderr, file=sys.stderr)
                return 1
        finally:
            shutil.rmtree(oracle_dir, ignore_errors=True)
        after = post("vfs-list", {"workspace": "notes"})
        after_by_path = {row["path"]: row["commit"] for row in after.get("entries", [])}
        if before_by_path.get(ORION_SOURCE) != after_by_path.get(ORION_SOURCE):
            print("FAIL: development advanced the nested semantic repository", file=sys.stderr)
            return 1
        if before_by_path.get("dev/calc/calc.go") == after_by_path.get("dev/calc/calc.go"):
            print("FAIL: development did not advance the owning root repository", file=sys.stderr)
            return 1

        print("==> independent author, feedback, and revision agents")
        code, out, err = run_agent(AUTHOR_TASK, None, author_dir)
        print("author stdout:", out.strip())
        if code != 0 or "AUTHORED=V1" not in out:
            print("author stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        v1 = vfs_read("feedback/release.md")
        if "Alpha" not in v1 or "Beta" in v1:
            print("FAIL: author V1 did not establish the intended review gap", repr(v1), file=sys.stderr)
            return 1

        code, out, err = run_agent(REVIEW_TASK, None, review_dir)
        print("reviewer stdout:", out.strip())
        if code != 0 or "REVIEWED=ok" not in out:
            print("reviewer stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        feedback = vfs_read("feedback/review.md")
        if "Beta" not in feedback or "security" not in feedback.lower():
            print("FAIL: feedback is not actionable", repr(feedback), file=sys.stderr)
            return 1

        code, out, err = run_agent(REVISE_TASK, None, revise_dir)
        print("reviser stdout:", out.strip())
        if code != 0 or "REVISED=V2" not in out:
            print("reviser stderr:", redact(err, env)[-4000:], file=sys.stderr)
            return 1
        v2 = vfs_read("feedback/release.md")
        lowered = v2.lower()
        if "alpha" not in lowered or "beta" not in lowered or "security approval" not in lowered:
            print("FAIL: revised artifact did not adopt persisted feedback", repr(v2), file=sys.stderr)
            return 1
        if "tbd" in lowered or "beta is ready" in lowered:
            print("FAIL: revised artifact violates the spec", repr(v2), file=sys.stderr)
            return 1
        if vfs_read("feedback/review.md") != feedback:
            print("FAIL: reviser altered the authoritative feedback", file=sys.stderr)
            return 1

        if accept_race(race_dir, env) != 0:
            return 1
        if accept_pin(pin_dir, live_dir, env) != 0:
            return 1
        if accept_revoke(revoke_dir, keeper_dir, env) != 0:
            return 1
        if accept_governance(
            gov_pin_dir, maintainer_dir, gov_review_dir, gov_live_dir, env
        ) != 0:
            return 1
        if accept_query(query_dir, env) != 0:
            return 1
        if accept_connector(connector_old_dir, connector_live_dir, env) != 0:
            return 1
        if accept_agent_remove(remove_dir, env) != 0:
            return 1
        listing = post("vfs-list", {"workspace": "notes"})
        final_live_pin = post("resolve", {"workspace": "notes"})
        paths = {row["path"] for row in listing.get("entries", [])}
        if "analysis/hacked.md" in paths:
            print("FAIL: consumer mutated the catalog", file=sys.stderr)
            return 1
        if accept_product_boundaries() != 0:
            return 1
        save_evidence(
            "acceptance.final.json",
            {
                "status": post("status", {}),
                "pinBeforeLifecycleArchive": final_live_pin,
                "audit": post("audit", {}),
                "filesBeforeLifecycleArchive": listing,
            },
        )
        print("PASS: J1-J8 Agent journeys and service oracles")
        return 0
    finally:
        shutil.rmtree(pub_dir, ignore_errors=True)
        shutil.rmtree(con_dir, ignore_errors=True)
        shutil.rmtree(dev_dir, ignore_errors=True)
        shutil.rmtree(author_dir, ignore_errors=True)
        shutil.rmtree(review_dir, ignore_errors=True)
        shutil.rmtree(revise_dir, ignore_errors=True)
        shutil.rmtree(race_dir, ignore_errors=True)
        shutil.rmtree(pin_dir, ignore_errors=True)
        shutil.rmtree(live_dir, ignore_errors=True)
        shutil.rmtree(revoke_dir, ignore_errors=True)
        shutil.rmtree(keeper_dir, ignore_errors=True)
        shutil.rmtree(gov_pin_dir, ignore_errors=True)
        shutil.rmtree(maintainer_dir, ignore_errors=True)
        shutil.rmtree(gov_review_dir, ignore_errors=True)
        shutil.rmtree(gov_live_dir, ignore_errors=True)
        shutil.rmtree(query_dir, ignore_errors=True)
        shutil.rmtree(connector_old_dir, ignore_errors=True)
        shutil.rmtree(connector_live_dir, ignore_errors=True)
        shutil.rmtree(remove_dir, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
