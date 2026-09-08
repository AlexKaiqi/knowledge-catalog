#!/usr/bin/env python3
"""Read-only inventory and explicit, source-bound validation runs (stdlib only)."""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import platform
import signal
import subprocess
import sys
import time
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]
SELECTORS = ("KC_E2E_RUN", "KC_COVERAGE_MIN", "KC_REQUIRE_LIVE_ADAPTERS",
             "KC_LIVE_TAIHU", "KC_OPENSEARCH_IMAGE", "KC_DOLT_DOCKER_IMAGE",
             "KC_DOLT_FORCE_DOCKER", "GOFLAGS", "CGO_ENABLED", "GOOS", "GOARCH")


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def capture(command, root):
    try:
        result = subprocess.run(command, cwd=root, capture_output=True, text=True,
                                timeout=30, check=False)
        return result.stdout.strip() if result.returncode == 0 else None
    except (OSError, subprocess.TimeoutExpired):
        return None


def source_state(root):
    """Hash tracked + non-ignored untracked bytes, including staged/worktree edits.

    No source contents, diffs, credentials, or arbitrary environment are copied.
    Symlinks contribute their link text; ignored caches/results do not contribute.
    """
    result = subprocess.run(["git", "ls-files", "--cached", "--others",
                             "--exclude-standard", "-z"], cwd=root,
                            capture_output=True, check=True)
    digest = hashlib.sha256()
    count = 0
    for name in sorted(set(result.stdout.split(b"\0")) - {b""}):
        path = root / os.fsdecode(name)
        digest.update(name + b"\0")
        if path.is_symlink():
            payload = os.fsencode(os.readlink(path))
            kind = b"symlink"
        elif path.is_file():
            payload = path.read_bytes()
            kind = b"executable" if path.stat().st_mode & 0o111 else b"file"
        elif path.is_dir():
            # A submodule has a separate authority; bind its revision and dirty bit.
            payload = json.dumps({"head": capture(["git", "rev-parse", "HEAD"], path),
                                  "status": capture(["git", "status", "--porcelain"], path)},
                                 sort_keys=True).encode()
            kind = b"submodule"
        else:
            payload, kind = b"", b"deleted"
        digest.update(kind + b"\0" + hashlib.sha256(payload).digest())
        count += 1
    return {"gitRevision": capture(["git", "rev-parse", "HEAD"], root),
            "dirty": bool(capture(["git", "status", "--porcelain", "--untracked-files=all"], root)),
            "fingerprint": digest.hexdigest(), "fileCount": count,
            "fingerprintPolicy": "sha256(path,type,content); tracked and non-ignored untracked files"}


class GoResults:
    def __init__(self):
        self.tests, self.packages = [], []
        self.executions = {}

    def add(self, event):
        package, action = event.get("Package"), event.get("Action")
        if not package or not action:
            return
        if action == "start" and not event.get("Test"):
            self.executions[package] = self.executions.get(package, 0) + 1
        if action not in ("pass", "fail", "skip"):
            return
        row = {"package": package, "execution": self.executions.get(package, 1),
               "status": action, "time": event.get("Time"), "elapsedSeconds": event.get("Elapsed")}
        if event.get("Test"):
            row["test"] = event["Test"]
            self.tests.append(row)
        else:
            self.packages.append(row)

    def summary(self, declared):
        def counts(rows):
            return {status: sum(row["status"] == status for row in rows)
                    for status in ("pass", "fail", "skip")}
        observed = {(row["package"], row["test"].split("/")[0]) for row in self.tests}
        not_observed = []
        for entry in declared:
            directory = str(pathlib.PurePosixPath(entry["file"]).parent)
            package = "kc" if directory == "." else "kc/" + directory
            if (package, entry["name"]) not in observed:
                not_observed.append(entry)
        return {"testEvents": counts(self.tests), "packageEvents": counts(self.packages),
                "declaredCount": len(declared), "notObservedCount": len(not_observed),
                "notObservedDeclarations": not_observed,
                "completeness": "explicit-selection-only; not-observed is not pass or skip"}


class RunInterrupted(BaseException):
    def __init__(self, signum):
        self.signum = signum


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def inventory(root):
    print(json.dumps(inventory_data(root), ensure_ascii=False, indent=2))
    return 0


def inventory_data(root):
    result = subprocess.run([os.environ.get("GO", "go"), "run", "./scripts/validation-inventory"],
                            cwd=root, capture_output=True, text=True)
    if result.returncode:
        raise ValueError(f"inventory generation failed: {result.stderr.strip()}")
    value = json.loads(result.stdout)
    value.update({"generatedAt": now(), "source": source_state(root)})
    return value


def run(args):
    root = args.root.resolve()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        raise ValueError("run requires an explicit command after --")
    started, monotonic = now(), time.monotonic()
    run_id = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:12]
    source = source_state(root)
    output = (args.output or root / ".validation" / "runs").resolve() / run_id
    output.mkdir(parents=True, exist_ok=False)
    environment = {name: os.environ[name] for name in SELECTORS if name in os.environ}
    environment.update({"os": platform.platform(), "machine": platform.machine(),
                        "python": platform.python_version(),
                        "go": capture([os.environ.get("GO", "go"), "version"], root),
                        "node": capture(["node", "--version"], root)})
    report = {"schemaVersion": 1, "runId": run_id, "scope": args.scope,
              "command": command, "startedAt": started, "source": source,
              "environment": environment, "status": "running",
              "selectionPolicy": "Only the explicit command ran; inventory entries not observed are not passes or skips."}
    write_json(output / "manifest.json", report)
    env = dict(os.environ, KC_VALIDATION_RUN_ID=run_id, KC_VALIDATION_RUN_DIR=str(output),
               KC_VALIDATION_SOURCE_FINGERPRINT=source["fingerprint"], KC_VALIDATION_SCOPE=args.scope)
    for name, directory in (("KC_ROLE_ARTIFACTS", "agent-roles"),
                            ("KC_QUESTION_ARTIFACTS", "agent-questions"),
                            ("KC_METRIC_ARTIFACTS", "agent-metric"),
                            ("KC_DW_RUN_ROOT", "data-warehouse")):
        env.setdefault(name, str(output / directory))
    events, exit_code, child, declared, interrupted = GoResults(), 1, None, [], False
    def interrupt(signum, _frame):
        raise RunInterrupted(signum)
    prior_sigterm = signal.signal(signal.SIGTERM, interrupt)
    print(f"validation evidence: {output}", flush=True)
    try:
        if (root / "scripts" / "validation-inventory").is_dir():
            snapshot = inventory_data(root)
            declared = snapshot["goDeclarations"]
            write_json(output / "inventory.json", snapshot)
        with (output / "output.log").open("w") as log, (output / "go-test.jsonl").open("w") as raw_events:
            child = subprocess.Popen(command, cwd=root, env=env, stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT, text=True, errors="replace",
                                     start_new_session=True)
            for line in child.stdout:
                log.write(line)
                log.flush()
                try:
                    event = json.loads(line)
                except ValueError:
                    event = None
                if isinstance(event, dict) and event.get("Action") and event.get("Package"):
                    raw_events.write(line)
                    events.add(event)
                    if event.get("Output"):
                        print(event["Output"], end="", flush=True)
                else:
                    print(line, end="", flush=True)
            exit_code = child.wait()
    except (KeyboardInterrupt, RunInterrupted) as error:
        interrupted = True
        if child and child.poll() is None:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
        exit_code = 128 + (error.signum if isinstance(error, RunInterrupted) else signal.SIGINT)
    except (OSError, ValueError) as error:
        report["runnerError"] = str(error)
        print(f"validation runner: {error}", file=sys.stderr)
    finally:
        signal.signal(signal.SIGTERM, prior_sigterm)
        end_source = source_state(root)
        verification = events.summary(declared)
        execution_status = "interrupted" if interrupted else "passed" if exit_code == 0 else "failed"
        status = execution_status
        if status == "passed" and (verification["testEvents"]["fail"] or verification["packageEvents"]["fail"]):
            status = "failed-events"
        elif status == "passed" and (verification["testEvents"]["skip"] or verification["packageEvents"]["skip"]):
            status = "passed-with-skips"
        report.update({"finishedAt": now(), "elapsedSeconds": round(time.monotonic() - monotonic, 3),
                       "exitCode": exit_code, "status": status, "executionStatus": execution_status,
                       "sourceAfter": end_source, "sourceChanged": source != end_source,
                       "tests": events.tests, "packages": events.packages,
                       "verification": verification,
                       "resultDetail": "go-test-events" if events.packages else "command-exit-only"})
        if report["sourceChanged"]:
            report["evidenceWarning"] = "Source changed during execution; this run does not validate one source state."
            if report["executionStatus"] == "passed":
                report["status"] = "source-changed"
        runner_exit_code = exit_code if exit_code >= 0 else 128 - exit_code
        if runner_exit_code == 0 and report["status"] in ("failed-events", "source-changed"):
            runner_exit_code = 1
        report["runnerExitCode"] = runner_exit_code
        report["artifacts"] = [{"path": str(path.relative_to(output)),
                                 "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
                                for path in sorted(output.rglob("*")) if path.is_file()]
        write_json(output / "report.json", report)
    return report["runnerExitCode"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="mode", required=True)
    inv = sub.add_parser("inventory", help="print declared inventory JSON; execute no tests")
    inv.add_argument("--root", type=pathlib.Path, default=ROOT)
    execute = sub.add_parser("run", help="run only the supplied command and retain source-bound evidence")
    execute.add_argument("--root", type=pathlib.Path, default=ROOT)
    execute.add_argument("--scope", required=True)
    execute.add_argument("--output", type=pathlib.Path)
    execute.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    try:
        return inventory(args.root) if args.mode == "inventory" else run(args)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"validation: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
