from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import time
import urllib.request
from pathlib import Path


FIXTURE = Path(__file__).resolve().parents[1]
REPO = FIXTURE.parents[1]


def _run(command: list[str], *, cwd: Path = REPO) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(command, cwd=cwd, capture_output=True, text=True)
    if result.returncode:
        detail = result.stderr.strip() or result.stdout.strip()
        raise RuntimeError(f"{' '.join(command)} failed ({result.returncode}):\n{detail}")
    return result


def _build(context) -> None:
    context.bin_dir.mkdir(parents=True, exist_ok=True)
    configured_kc = os.environ.get("KC_BIN", "").strip()
    configured_preview = os.environ.get("KC_CONNECTOR_PREVIEW_BIN", "").strip()
    context.kc = Path(configured_kc) if configured_kc else context.bin_dir / "kc"
    context.preview = Path(configured_preview) if configured_preview else context.bin_dir / "connector-preview"
    if not configured_kc:
        _run(["go", "build", "-o", str(context.kc), "./cmd/kc"])
    if not configured_preview:
        _run([
            "go", "build", "-o", str(context.preview),
            "./.data/data-warehouse/connector/preview",
        ])


def before_all(context) -> None:
    context.fixture = FIXTURE
    context.repo = REPO
    context.python = REPO / ".venv" / "bin" / "python"
    context.run_root = Path(os.environ.get("KC_DW_RUN_ROOT", str(FIXTURE / "runs" / "current")))
    context.bin_dir = context.run_root / "bin"
    context.scenario_root = context.run_root / "scenarios"
    if not context.config.dry_run:
        for generated in (context.scenario_root, context.run_root / "kc-home", context.run_root / "evidence", context.run_root / "state"):
            if generated.exists():
                shutil.rmtree(generated)
    context.scenario_root.mkdir(parents=True, exist_ok=True)
    _build(context)


def _slug(scenario) -> str:
    ids = sorted(tag for tag in scenario.effective_tags if tag.startswith("DW-"))
    return (ids[0] if ids else scenario.name).replace("/", "-").replace(" ", "-")


def _start_mysql(context) -> None:
    context.compose = [
        "docker", "compose", "--project-name", "kc-dw-acceptance",
        "--file", str(FIXTURE / "mysql" / "compose.yaml"),
    ]
    last_failure = ""
    for attempt in range(3):
        _run([*context.compose, "down", "--volumes", "--remove-orphans"])
        started = subprocess.run(
            [*context.compose, "up", "--detach", "--wait"],
            cwd=REPO,
            capture_output=True,
            text=True,
        )
        if started.returncode == 0:
            break
        last_failure = started.stderr.strip() or started.stdout.strip()
        if attempt < 2:
            # Docker Desktop can briefly report a just-recreated container as
            # missing while `compose up --wait` races its own cleanup. Retry a
            # fresh project lifecycle, but keep configuration/health failures
            # visible after the bounded attempts.
            time.sleep(0.5 * (attempt + 1))
    else:
        raise RuntimeError(f"{' '.join([*context.compose, 'up', '--detach', '--wait'])} failed after 3 attempts:\n{last_failure}")
    context.mysql_container = _run([*context.compose, "ps", "--quiet", "mysql"]).stdout.strip()
    if not context.mysql_container:
        raise RuntimeError("MySQL Compose did not create the mysql container")


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _start_kc_service(context) -> None:
    port = _free_port()
    context.kc_serve = f"http://127.0.0.1:{port}"
    log = (context.run / "kc-serve.log").open("w", encoding="utf-8")
    context.kc_service_log = log
    command = [str(context.kc), "serve", "--home", str(context.home), "--listen", f"127.0.0.1:{port}"]
    if context.resource_access:
        command.extend(["--resource-access-url", context.resource_access])
    context.kc_service = subprocess.Popen(
        command,
        cwd=REPO,
        stdout=log,
        stderr=subprocess.STDOUT,
        text=True,
    )
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if context.kc_service.poll() is not None:
            raise RuntimeError(f"kc serve exited early; see {context.run / 'kc-serve.log'}")
        for path in ("/livez", "/health"):
            try:
                with urllib.request.urlopen(context.kc_serve + path, timeout=1) as response:
                    if response.status == 200:
                        return
            except Exception:
                pass
        time.sleep(0.1)
    raise RuntimeError("kc serve did not become ready")


def _start_resource_access(context) -> None:
    port = _free_port()
    context.resource_access = f"http://127.0.0.1:{port}"
    log = (context.run / "resource-access.log").open("w", encoding="utf-8")
    context.resource_access_log = log
    env = os.environ.copy()
    env.update({
        "KC_MYSQL_CONTAINER": context.mysql_container,
        "KC_MYSQL_PASSWORD": "dw-test-root",
        "KC_MYSQL_DATABASE": "tpch",
        "PYTHONDONTWRITEBYTECODE": "1",
    })
    context.resource_access_service = subprocess.Popen(
        [str(context.python), str(FIXTURE / "connector" / "access.py"), "--listen", f"127.0.0.1:{port}"],
        cwd=FIXTURE / "connector",
        env=env,
        stdout=log,
        stderr=subprocess.STDOUT,
        text=True,
    )
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if context.resource_access_service.poll() is not None:
            raise RuntimeError(f"resource access exited early; see {context.run / 'resource-access.log'}")
        try:
            with urllib.request.urlopen(context.resource_access + "/health", timeout=1) as response:
                if response.status == 200:
                    return
        except Exception:
            pass
        time.sleep(0.1)
    raise RuntimeError("resource access did not become ready")


def before_scenario(context, scenario) -> None:
    context.run = context.scenario_root / _slug(scenario)
    if context.run.exists():
        shutil.rmtree(context.run)
    context.run.mkdir(parents=True)
    context.home = context.run / "kc-home"
    context.commands = []
    context.command = None
    context.command_index = 0
    context.mysql_container = ""
    context.compose = None
    context.kc_service = None
    context.kc_service_log = None
    context.resource_access = ""
    context.resource_access_service = None
    context.resource_access_log = None
    context.agent_runs = []
    if "mysql" in scenario.effective_tags or "agent" in scenario.effective_tags:
        _start_mysql(context)
    if "resource" in scenario.effective_tags or "agent" in scenario.effective_tags:
        _start_resource_access(context)
    if "agent" in scenario.effective_tags:
        _start_kc_service(context)


def after_step(context, step) -> None:
    if step.status.name == "failed" and context.command is not None:
        print("\nlast command:", context.command["command"])
        print("stdout:\n", context.command["stdout"][-4000:])
        print("stderr:\n", context.command["stderr"][-4000:])


def after_scenario(context, scenario) -> None:
    (context.run / "commands.json").write_text(
        json.dumps(context.commands, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    if context.kc_service is not None:
        context.kc_service.terminate()
        try:
            context.kc_service.wait(timeout=5)
        except subprocess.TimeoutExpired:
            context.kc_service.kill()
        context.kc_service_log.close()
    if context.resource_access_service is not None:
        context.resource_access_service.terminate()
        try:
            context.resource_access_service.wait(timeout=5)
        except subprocess.TimeoutExpired:
            context.resource_access_service.kill()
        context.resource_access_log.close()
    if context.compose is not None:
        subprocess.run(
            [*context.compose, "down", "--volumes", "--remove-orphans"],
            cwd=REPO,
            capture_output=True,
            text=True,
        )
