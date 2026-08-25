#!/usr/bin/env python3
"""Real DSH Agent oracle for a read-only remote Workspace mounted in a project."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

BASE = os.environ["KC_SERVE"]
PROFILE = os.environ["DSH_PROFILE"]
PATCH = Path(os.environ["DSH_MODEL_PATCH"])
CONTROL = Path(os.environ["KC_E2E_CONTROL"])
ARTIFACTS = Path(os.environ["KC_E2E_ARTIFACTS"])
REMOTE_REPO = os.environ["KC_E2E_REMOTE_REPO"]
WORKSPACE = os.environ.get("KC_WORKSPACE", "project-knowledge")

PIN_TASK = f"""You are working in an ordinary local project with remote knowledge mounted at
`.knowledge/`. You must call all of these tools: Read PROJECT.md, list `.knowledge`, grep
for `release_code=` under `.knowledge`, then Read the matched remote policy. Compute the
decision from the policy and the local project's approval count. Next use Bash exactly:
`touch {CONTROL / 'pin-ready'} && while [ ! -f {CONTROL / 'pin-done'} ]; do sleep 1; done`
After Bash completes, read the same remote policy again and recompute. Do not write or edit.
Do not call kc, Skill, Glob, or any other Bash command. If a required tool fails, do not
diagnose or retry it; reply exactly PIN_ERROR=<failed-tool-name>.
Reply exactly: PIN=<first-revision>:<first-decision>-><second-revision>:<second-decision> CODE=<first-release-code>
"""

LIVE_TASK = """This is a fresh Agent task in a Git project. Remote knowledge is mounted at
`.knowledge/`. Read PROJECT.md, use glob and Read to locate and verify the current remote
release policy, and compute the decision from its minimum approvals and the local approval
count. Do not write or edit. Reply exactly:
LIVE=<revision>:<decision> CODE=<release-code> SOURCE=<project-relative-policy-path>
"""

READONLY_TASK = """Remote knowledge is mounted read-only at `.knowledge/`. Use the filesystem
Write tool exactly once to try creating `.knowledge/governance/agent-must-not-write.txt`
with content `forbidden`. The Write must be denied. Do not retry and do not use kc or Bash.
After observing the denial, reply exactly: READONLY=ok
"""


def post(verb: str, body: dict) -> dict:
    request = urllib.request.Request(
        f"{BASE}/v1/{verb}",
        data=json.dumps(body).encode(),
        headers={"content-type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def write_remote(content: str, command_id: str) -> str:
    result = post(
        "vfs-write",
        {
            "workspace": WORKSPACE,
            "command-id": command_id,
            "path": "governance/release-policy.md",
            "content": base64.b64encode(content.encode()).decode(),
        },
    )["result"]
    if result["repositoryId"] != REMOTE_REPO:
        raise AssertionError(f"write routed to wrong Repository: {result}")
    return str(result["newCommit"])


def tree_digest(root: Path, include_git: bool) -> str:
    digest = hashlib.sha256()
    for item in sorted(root.rglob("*")):
        relative = item.relative_to(root)
        if not include_git and relative.parts and relative.parts[0] == ".git":
            continue
        digest.update(str(relative).encode() + b"\0")
        if item.is_file():
            digest.update(item.read_bytes())
    return digest.hexdigest()


def agent_env() -> dict[str, str]:
    env = os.environ.copy()
    env["DSH_PERMISSION_MODE"] = "danger-full-access"
    env["KC_SERVE"] = BASE
    env["KC_WORKSPACE"] = WORKSPACE
    env["KC_MOUNT_PATH"] = ".knowledge"
    env.pop("KC_WORKSPACE_DIR", None)
    return env


def command(task: str) -> list[str]:
    return ["dsh", "--profile", PROFILE, "--patch", str(PATCH), task]


def wait_for(path: Path, process: subprocess.Popen[str], timeout: float = 120) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline and process.poll() is None:
        if path.exists():
            return
        time.sleep(0.1)
    raise RuntimeError(f"Agent did not create {path}; exit={process.poll()}")


def redact(value: str) -> str:
    result = value
    for key in ("OPENAI_API_KEY", "DEEPSEEK_API_KEY", "KC_GITEA_TOKEN"):
        secret = os.environ.get(key, "")
        if secret:
            result = result.replace(secret, "***")
    return result


def capture_trace(project: Path, label: str) -> tuple[list[dict], str]:
    sessions = Path(os.environ.get("DSH_HOME", str(Path.home() / ".dsh"))) / "sessions"
    candidates = [
        item
        for item in sessions.rglob("session.jsonl.zstd")
        if project.name in str(item)
    ]
    if not candidates:
        raise AssertionError(f"no DSH session transcript found for {project}")
    trace = max(candidates, key=lambda item: item.stat().st_mtime)
    copied = ARTIFACTS / f"{label}.session.jsonl.zstd"
    copied.write_bytes(trace.read_bytes())
    decoded = subprocess.run(["zstd", "-dc", str(trace)], check=True, capture_output=True, text=True).stdout
    if REMOTE_REPO in decoded:
        raise AssertionError(f"{label} transcript leaked the routed Repository ID")
    calls: list[dict] = []
    for line in decoded.splitlines():
        event = json.loads(line)
        if event.get("type") == "tool/call":
            calls.append(
                {
                    "name": event.get("data", {}).get("name"),
                    "arguments": event.get("data", {}).get("arguments"),
                }
            )
    (ARTIFACTS / f"{label}.tool-calls.json").write_text(json.dumps(calls, indent=2) + "\n")
    return calls, decoded


def require_calls(calls: list[dict], required: set[str], label: str) -> None:
    names = [str(call.get("name")) for call in calls]
    missing = required - set(names)
    if missing:
        raise AssertionError(f"{label} transcript is missing tool calls {sorted(missing)}; got {names}")
    forbidden = {"write", "edit", "kc"} & set(names)
    if forbidden:
        raise AssertionError(f"{label} made forbidden mutation calls: {sorted(forbidden)}")


def main() -> int:
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    CONTROL.mkdir(parents=True, exist_ok=True)
    plain = Path(tempfile.mkdtemp(prefix="dsh-plain-project-"))
    git_project = Path(tempfile.mkdtemp(prefix="dsh-git-project-"))
    readonly_project = Path(tempfile.mkdtemp(prefix="dsh-readonly-project-"))
    plain.joinpath("PROJECT.md").write_text(
        "Current production approvals: 2. Use the mounted release policy to decide PROCEED or HOLD.\n"
    )
    git_project.joinpath("PROJECT.md").write_text(
        "Current production approvals: 2. Use the mounted release policy to decide PROCEED or HOLD.\n"
    )
    readonly_project.joinpath("PROJECT.md").write_text("This project consumes remote knowledge read-only.\n")
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=git_project, check=True)
    subprocess.run(["git", "add", "PROJECT.md"], cwd=git_project, check=True)
    subprocess.run(
        ["git", "-c", "core.hooksPath=/dev/null", "-c", "user.name=fixture", "-c", "user.email=fixture@local", "commit", "-q", "-m", "root"],
        cwd=git_project,
        check=True,
    )
    plain_before = tree_digest(plain, include_git=True)
    git_before = subprocess.run(
        ["git", "status", "--porcelain=v1"], cwd=git_project, check=True, capture_output=True, text=True
    ).stdout
    readonly_before = tree_digest(readonly_project, include_git=True)

    v1 = write_remote(
        "revision=V1\nrelease_code=ORBIT-731\nminimum_approvals=2\n"
        "decision rule: PROCEED when current approvals are at least minimum_approvals; otherwise HOLD.\n",
        "project-mount-v1",
    )
    initial_listing = post("vfs-list", {"workspace": WORKSPACE})
    if not any(entry.get("path") == "governance/release-policy.md" for entry in initial_listing.get("entries", [])):
        raise AssertionError("remote V1 is not readable immediately before the Agent task")
    pin = subprocess.Popen(
        command(PIN_TASK), cwd=plain, env=agent_env(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    try:
        wait_for(CONTROL / "pin-ready", pin)
        v2 = write_remote(
            "revision=V2\nrelease_code=NOVA-944\nminimum_approvals=3\n"
            "decision rule: PROCEED when current approvals are at least minimum_approvals; otherwise HOLD.\n",
            "project-mount-v2",
        )
        (CONTROL / "pin-done").write_text("ok\n")
        pin_out, pin_err = pin.communicate(timeout=360)
    except Exception:
        pin.terminate()
        pin_out, pin_err = pin.communicate(timeout=15)
        (ARTIFACTS / "pin.failed.stdout.txt").write_text(pin_out)
        (ARTIFACTS / "pin.failed.stderr.txt").write_text(redact(pin_err))
        raise
    (ARTIFACTS / "pin.stdout.txt").write_text(pin_out)
    (ARTIFACTS / "pin.stderr.txt").write_text(redact(pin_err))
    print("pinned Agent:", pin_out.strip())
    if pin.returncode != 0 or "PIN=V1:PROCEED->V1:PROCEED CODE=ORBIT-731" not in pin_out:
        print(redact(pin_err)[-5000:], file=sys.stderr)
        raise AssertionError("old Agent task did not retain the V1 Workspace pin")

    live = subprocess.run(
        command(LIVE_TASK), cwd=git_project, env=agent_env(), capture_output=True, text=True, timeout=360
    )
    (ARTIFACTS / "live.stdout.txt").write_text(live.stdout)
    (ARTIFACTS / "live.stderr.txt").write_text(redact(live.stderr))
    print("new Agent:", live.stdout.strip())
    expected = "LIVE=V2:HOLD CODE=NOVA-944 SOURCE=.knowledge/governance/release-policy.md"
    if live.returncode != 0 or expected not in live.stdout:
        print(redact(live.stderr)[-5000:], file=sys.stderr)
        raise AssertionError("new Agent task did not consume the updated remote commit")

    pin_calls, _ = capture_trace(plain, "pin")
    live_calls, _ = capture_trace(git_project, "live")
    require_calls(pin_calls, {"read", "list", "grep", "bash"}, "pinned Agent")
    require_calls(live_calls, {"read", "glob"}, "new Agent")
    pin_remote_reads = [
        call for call in pin_calls
        if call.get("name") == "read" and ".knowledge/governance/release-policy.md" in str(call.get("arguments"))
    ]
    if len(pin_remote_reads) < 2:
        raise AssertionError(f"pinned Agent did not read the same remote path twice: {pin_calls}")

    head_before_denial = post("resolve", {"workspace": WORKSPACE})["repositories"][REMOTE_REPO]
    denied = subprocess.run(
        command(READONLY_TASK), cwd=readonly_project, env=agent_env(), capture_output=True, text=True, timeout=360
    )
    (ARTIFACTS / "readonly.stdout.txt").write_text(denied.stdout)
    (ARTIFACTS / "readonly.stderr.txt").write_text(redact(denied.stderr))
    print("read-only Agent:", denied.stdout.strip())
    if denied.returncode != 0 or "READONLY=ok" not in denied.stdout:
        print(redact(denied.stderr)[-5000:], file=sys.stderr)
        raise AssertionError("Agent did not observe the remote read-only boundary")
    readonly_calls, readonly_trace = capture_trace(readonly_project, "readonly")
    readonly_names = [call.get("name") for call in readonly_calls]
    if readonly_names.count("write") != 1 or "kc" in readonly_names or "bash" in readonly_names:
        raise AssertionError(f"read-only Agent used the wrong tools: {readonly_calls}")
    denial_events = []
    for line in readonly_trace.splitlines():
        event = json.loads(line)
        data = event.get("data", {})
        if event.get("type") == "tool/result" and data.get("error", {}).get("code") == "FS_SANDBOX_DENIED":
            denial_events.append(event)
    if len(denial_events) != 1:
        raise AssertionError(f"read-only transcript has {len(denial_events)} structured remote-write denials, want 1")
    head_after_denial = post("resolve", {"workspace": WORKSPACE})["repositories"][REMOTE_REPO]
    if head_after_denial != head_before_denial:
        raise AssertionError("denied Agent write moved the remote Repository HEAD")
    listing = post("vfs-list", {"workspace": WORKSPACE})
    if any(entry.get("path") == "governance/agent-must-not-write.txt" for entry in listing.get("entries", [])):
        raise AssertionError("denied Agent write created a remote file")
    if tree_digest(readonly_project, include_git=True) != readonly_before:
        raise AssertionError("denied remote write changed the local project")

    if tree_digest(plain, include_git=True) != plain_before:
        raise AssertionError("read-only mount changed the non-Git project")
    git_after = subprocess.run(
        ["git", "status", "--porcelain=v1"], cwd=git_project, check=True, capture_output=True, text=True
    ).stdout
    if git_after != git_before or git_project.joinpath(".knowledge").exists() or plain.joinpath(".knowledge").exists():
        raise AssertionError("read-only mount changed or materialized into a project")
    live_pin = post("resolve", {"workspace": WORKSPACE})
    if live_pin["repositories"][REMOTE_REPO] != v2 or v1 == v2:
        raise AssertionError(f"unexpected remote commits: V1={v1} V2={v2} pin={live_pin}")
    (ARTIFACTS / "oracle.json").write_text(
        json.dumps(
            {
                "nonGitProject": "unchanged",
                "gitProject": "clean",
                "mountMaterialized": False,
                "oldTask": "V1->V1",
                "newTask": "V2",
                "remoteWrite": "DENIED_WITHOUT_HEAD_CHANGE",
                "repositoryIdentityVisibleToAgent": False,
                "remoteRepository": REMOTE_REPO,
                "v1Commit": v1,
                "v2Commit": v2,
                "livePin": live_pin,
            },
            indent=2,
            sort_keys=True,
        )
        + "\n"
    )
    print(f"PASS: Git and non-Git project mount, pin/update, remote commits {v1[:12]} -> {v2[:12]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
