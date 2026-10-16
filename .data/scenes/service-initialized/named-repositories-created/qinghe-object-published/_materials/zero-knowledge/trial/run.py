#!/usr/bin/env python3
"""Run one Snapshot/Object trial; credentials and review files stay outside the agent."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
CASES = {"env": "zk-colleague", "supplier": "zk-supplier", "colleague": "zk-colleague",
         "revision": "zk-supplier", "history": "zk-colleague"}


def stage_materials(case, destination, source=None):
    """Copy declared input only; provenance contains names and hashes, not bodies."""
    if source is not None:
        if case not in ("supplier", "revision"):
            raise ValueError("--materials is only supported for supplier or revision")
        source = source.expanduser()
        if source.is_symlink():
            raise ValueError("Materials directory must not be a symbolic link")
        if not source.is_dir():
            raise ValueError("Materials source must be an existing directory")
        entries = [(path, path.relative_to(source)) for path in sorted(source.rglob("*"))]
        for path, relative in entries:
            if path.is_symlink():
                raise ValueError("Materials contain a symbolic link: " + relative.as_posix())
            if not path.is_file() and not path.is_dir():
                raise ValueError("Materials contain a non-regular entry: " + relative.as_posix())
    elif case in ("supplier", "revision"):
        version = "v1" if case == "supplier" else "v2"
        entries = [(HERE.parent / "supplier" / ("gmv-caliber." + version + ".md"),
                    Path("gmv-caliber.md"))]
    else:
        entries = []
    destination.mkdir(parents=True, mode=0o755)
    manifest = []
    for path, relative in entries:
        target = destination / relative
        if path.is_dir():
            target.mkdir(exist_ok=True)
            continue
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, target)
        digest = hashlib.sha256()
        with target.open("rb") as material:
            for chunk in iter(lambda: material.read(1024 * 1024), b""):
                digest.update(chunk)
        manifest.append({"path": relative.as_posix(), "sha256": digest.hexdigest()})
    return manifest


def model_key():
    for name in ("ZK_MODEL_API_KEY", "VOLCENGINE_API_KEY", "ARK_API_KEY"):
        if os.environ.get(name):
            return os.environ[name]
    path = Path.home() / ".env"
    if path.is_file():
        match = re.search(r"(?m)^\s*(?:export\s+)?VOLCENGINE_API_KEY\s*=\s*(.*)$", path.read_text())
        if match:
            values = shlex.split(match.group(1), comments=True)
            if len(values) == 1 and values[0]:
                return values[0]
    raise ValueError("VOLCENGINE_API_KEY is missing (environment or user .env)")


def command(args, env, **kwargs):
    return subprocess.run(args, env=env, check=True, **kwargs)


def last_answer(path):
    final = ""
    errors = []
    ended = False
    settled = None
    for line in path.read_text().split("\n"):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except ValueError:
            errors.append("malformed JSON event")
            settled = None
            continue
        event_type = event.get("type")
        if event_type == "agent_start":
            ended = False
        elif event_type == "agent_end":
            ended = True
        # agent_end can precede recovery or queued work. Only a final settled
        # event with an explicit non-aborted outcome closes the invocation.
        settled = event if event_type == "agent_settled" else None
        message = event.get("message", {})
        if event_type == "message_end" and message.get("role") == "assistant":
            text = "\n".join(c.get("text", "") for c in message.get("content", []) if c.get("type") == "text")
            final = text or final
            if message.get("stopReason") in ("error", "aborted"):
                errors.append(message.get("errorMessage", message["stopReason"]))
    if not ended:
        errors.append("missing agent_end event")
    if settled is None:
        errors.append("missing final agent_settled event")
    elif settled.get("aborted") is True:
        errors.append("agent run was aborted")
    elif settled.get("aborted") is not False:
        errors.append("agent_settled lacks an explicit non-aborted outcome")
    return final, errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("case", choices=CASES)
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--materials", type=Path, help="Raw-material directory for supplier or revision")
    parser.add_argument("--message", help="An explicit human follow-up; recorded verbatim")
    parser.add_argument("--resume", type=Path, help="Resume the same case's recorded session")
    args = parser.parse_args()
    if args.timeout <= 0:
        parser.error("timeout must be positive")
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
    root = Path(os.environ.get("KC_ZK_RUNS", str(Path.home() / "kc-zk-runs"))).expanduser()
    run = root / (stamp + "-snapshot-" + args.case)
    run.mkdir(parents=True, mode=0o700)
    (run / "sessions").mkdir(mode=0o777)
    (run / "sessions").chmod(0o777)
    (run / "work").mkdir(mode=0o777)
    (run / "work").chmod(0o777)
    try:
        materials = stage_materials(args.case, run / "input", args.materials)
    except (ValueError, OSError) as error:
        parser.error(str(error))
    prompt = args.message or (HERE / "prompts" / ("snapshot-" + args.case + ".md")).read_text()
    (run / "prompt.md").write_text(prompt)
    env = os.environ.copy()
    env.update(ZK_PRINCIPAL=CASES[args.case], ZK_RUN_DIR=str(run),
               ZK_MODEL_API_KEY=model_key(),
               ZK_MODEL_BASE_URL=os.environ.get("ZK_MODEL_BASE_URL", "https://ark.cn-beijing.volces.com/api/plan/v3"),
               ZK_MODEL_ID=os.environ.get("ZK_MODEL_ID", "deepseek-v4.1-flash"))
    project = "kc-zk-agent-" + stamp.lower().replace(".", "-")
    compose = ["docker", "compose", "--project-directory", str(HERE), "-p", project,
               "-f", str(HERE / "compose.agent.yaml")]
    image = env.get("KC_ZK_AGENT_IMAGE", "kc-zk-agent:local")
    image_id = command(["docker", "image", "inspect", "--format", "{{.Id}}", image], env,
                       capture_output=True, text=True).stdout.strip()
    meta = {"case": args.case, "principal": CASES[args.case], "scope": ["snapshot", "object"],
            "model": env["ZK_MODEL_ID"], "model_endpoint": env["ZK_MODEL_BASE_URL"],
            "agent_image": image_id, "started": stamp, "timeout_seconds": args.timeout,
            "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            "retains_work_directory": True, "prompt_sha256": hashlib.sha256(prompt.encode()).hexdigest(),
            "materials": materials,
            "review": "unassessed", "resume": str(args.resume) if args.resume else None}
    (run / "run.json").write_text(json.dumps(meta, indent=2, ensure_ascii=False) + "\n")
    (run / "record.md").write_text("# Snapshot/Object 试跑评审\n\n判定分别记录结果、依据、权限、信息来源。\n\n"
                                   "运行结束只代表 Agent 停止；任务是否完成由独立状态核对和人工评审决定。\n\n"
                                   "## 结果\n\n待评审。\n\n## 介入\n\n无（如有后续回答，引用对应运行记录）。\n\n## 缺口\n\n待归因。\n")
    print("run_dir: " + str(run), flush=True)
    status = 1
    try:
        with (run / "runtime.log").open("w") as runtime:
            command(compose + ["up", "-d", "gateway"], env, stdout=runtime, stderr=runtime)
            # This checks actual request identity through the same sealed network, not just config text.
            probe = ('test -z "${ZK_MODEL_API_KEY:-}${VOLCENGINE_API_KEY:-}${ARK_API_KEY:-}" '
                     '&& test ! -e /var/run/docker.sock '
                     '&& test ! -r "$(command -v kc)" && KC_AS=admin kc whoami')
            result = None
            for attempt in range(15):
                result = subprocess.run(compose + ["run", "--rm", "-T", "--no-deps", "--entrypoint", "/bin/sh", "agent", "-c", probe],
                                        env=env, capture_output=True, text=True)
                if result.returncode == 0:
                    break
                time.sleep(1)
            if result.returncode:
                runtime.write(result.stderr)
                raise RuntimeError("isolation/identity preflight failed; see runtime.log")
            identity = json.loads(result.stdout)
            if identity.get("principal") != CASES[args.case]:
                raise RuntimeError("KC gateway did not enforce trial principal")
            (run / "preflight.json").write_text(json.dumps({"forged_admin_request_identity": identity,
                "agent_has_model_key": False, "agent_has_docker_socket": False}, indent=2) + "\n")
            agent_args = ["-p", "--mode", "json", "--no-extensions", "--no-skills", "--no-prompt-templates",
                          "--no-context-files", "--no-mcp", "--no-themes", "--no-approve", "--thinking", "low"]
            if args.resume:
                previous = json.loads((args.resume / "run.json").read_text())
                if previous["principal"] != CASES[args.case] or previous["case"] != args.case:
                    raise ValueError("resume must use the same case and principal")
                sessions = sorted((args.resume / "sessions").glob("*.jsonl"))
                if len(sessions) != 1:
                    raise ValueError("resume requires exactly one previous session")
                resumed = run / "sessions" / sessions[0].name
                shutil.copyfile(sessions[0], resumed)
                resumed.chmod(0o666)
                agent_args += ["--session", "/runs/sessions/" + sessions[0].name]
            with (run / "transcript.jsonl").open("w") as transcript:
                result = subprocess.run(compose + ["run", "--rm", "-T", "--no-deps", "agent"] + agent_args + [prompt],
                                        env=env, stdout=transcript, stderr=runtime, timeout=args.timeout)
                status = result.returncode
        answer, errors = last_answer(run / "transcript.jsonl")
        (run / "answer.md").write_text(answer + "\n")
        if errors or not answer:
            status = status or 1
        meta.update(process_exit=status, agent_errors=errors, has_final_answer=bool(answer))
    except (ValueError, RuntimeError, subprocess.SubprocessError, OSError) as error:
        meta.update(process_exit=1, execution_error=type(error).__name__ + ": " + str(error))
        print("execution_error: " + str(error), file=sys.stderr)
    finally:
        with (run / "runtime.log").open("a") as runtime:
            subprocess.run(compose + ["logs", "--no-color", "gateway"], env=env, stdout=runtime, stderr=runtime)
            subprocess.run(compose + ["down", "--remove-orphans"], env=env, stdout=runtime, stderr=runtime)
        meta["finished"] = datetime.now(timezone.utc).isoformat()
        (run / "run.json").write_text(json.dumps(meta, indent=2, ensure_ascii=False) + "\n")
    print("process_exit: " + str(meta.get("process_exit", status)), flush=True)
    print("review: unassessed", flush=True)
    return meta.get("process_exit", status)


if __name__ == "__main__":
    sys.exit(main())
