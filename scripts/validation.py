#!/usr/bin/env python3
"""Read-only inventory and explicit, source-bound validation runs (stdlib only)."""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import platform
import re
import shutil
import signal
import subprocess
import sys
import time
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]
SELECTORS = ("KC_E2E_RUN", "KC_COVERAGE_MIN", "KC_REQUIRE_LIVE_ADAPTERS",
             "KC_LIVE_TAIHU", "KC_OPENSEARCH_IMAGE", "GOFLAGS", "CGO_ENABLED", "GOOS", "GOARCH")


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
        self.no_test_files = set()

    def add(self, event):
        package, action = event.get("Package"), event.get("Action")
        if not package or not action:
            return
        if action == "start" and not event.get("Test"):
            self.executions[package] = self.executions.get(package, 0) + 1
        execution = self.executions.get(package, 1)
        if action == "output" and not event.get("Test") and re.fullmatch(
                r"\?\s+" + re.escape(package) + r"\s+\[no test files\]", event.get("Output", "").strip()):
            self.no_test_files.add((package, execution))
        if action not in ("pass", "fail", "skip"):
            return
        row = {"package": package, "execution": execution,
               "status": action, "time": event.get("Time"), "elapsedSeconds": event.get("Elapsed")}
        if event.get("Test"):
            row["test"] = event["Test"]
            self.tests.append(row)
        else:
            if action == "skip" and (package, execution) in self.no_test_files:
                row.update(status="no-tests", goAction=action)
            self.packages.append(row)

    def summary(self, declared):
        def counts(rows, statuses=("pass", "fail", "skip")):
            return {status: sum(row["status"] == status for row in rows)
                    for status in statuses}
        observed = {(row["package"], row["test"].split("/")[0]) for row in self.tests}
        not_observed = []
        for entry in declared:
            directory = str(pathlib.PurePosixPath(entry["file"]).parent)
            package = "kc" if directory == "." else "kc/" + directory
            if (package, entry["name"]) not in observed:
                not_observed.append(entry)
        return {"testEvents": counts(self.tests), "packageEvents": counts(self.packages, ("pass", "fail", "skip", "no-tests")),
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


def inventory_summary(snapshot):
    """Bind a run to its declared inventory without copying the rebuildable payload.

    The full inventory is a deterministic function of the source fingerprint
    already recorded in manifest.json, so it is regenerated on demand instead of
    stored per run. The digest is what makes a later regeneration comparable.
    """
    canonical = json.dumps(snapshot, ensure_ascii=False, sort_keys=True,
                           separators=(",", ":")).encode()
    return {"kind": "declared-inventory-summary", "schemaVersion": 1,
            "counts": snapshot.get("counts", {}),
            "limitations": snapshot.get("limitations", []),
            "generatedAt": snapshot.get("generatedAt"),
            "digest": hashlib.sha256(canonical).hexdigest(),
            "digestPolicy": "sha256 of canonical JSON of the full declared inventory",
            "regenerateWith": "make validation-inventory"}



def directory_bytes(path):
    total = 0
    for entry in path.rglob("*"):
        try:
            if entry.is_file():
                total += entry.stat().st_size
        except OSError:
            continue
    return total


def referenced_run_ids(root, candidates):
    """Return the subset of run ids mentioned by tracked or newly added files.

    Runs cited from tasks or docs are the ones a later reader is expected to
    open, so retention must not delete them. Ignored evidence is not searched:
    `.validation/` is ignored, so `--untracked` does not re-find the runs
    themselves. One grep covers every candidate, because a per-run grep would
    spawn hundreds of processes on a large history.
    """
    if not candidates:
        return set()
    pattern = "|".join(re.escape(name) for name in candidates)
    result = subprocess.run(["git", "grep", "--untracked", "-h", "-o", "-E", "-e", pattern],
                            cwd=root, capture_output=True, text=True)
    if result.returncode not in (0, 1):
        # A non-match exit is normal; anything else means the citation check
        # cannot be answered, so keep everything rather than guess.
        raise ValueError(f"citation check failed: {result.stderr.strip()}")
    return set(result.stdout.split())


def prune(args):
    """Apply the evidence retention policy; deletion requires --apply."""
    root = args.root.resolve()
    runs = root / ".validation" / "runs"
    if not runs.is_dir():
        print(f"validation prune: {runs} does not exist; nothing to prune")
        return 0
    if args.keep < 1:
        raise ValueError("--keep must be at least 1")
    by_scope = {}
    unmanaged = []
    for path in sorted(runs.iterdir()):
        # Only directories the runner created carry a manifest. Other content
        # under runs/ belongs to whoever wrote it and is never pruned here.
        if not path.is_dir():
            continue
        if not (path / "manifest.json").is_file():
            unmanaged.append(path)
            continue
        try:
            scope = json.loads((path / "manifest.json").read_text()).get("scope")
        except (OSError, ValueError):
            scope = None
        by_scope.setdefault(scope or "(unknown)", []).append(path)
    # Per-scope retention keeps recent history comparable inside one scope. It
    # does not bound disk: a history of one-off scopes keeps every run forever,
    # which is how this repository reached hundreds of scopes with one run each.
    # So the global window is applied to the union of per-scope survivors rather
    # than to the leftovers, and it is what actually caps accumulation.
    considered = [path for scope in sorted(by_scope) if not args.scope or scope in args.scope
                  for path in sorted(by_scope[scope], reverse=True)]
    # Per-scope retention keeps recent history comparable inside one scope. It
    # does not bound disk: a history of one-off scopes keeps every run forever,
    # which is how this repository reached hundreds of scopes with one run each.
    # So the global window is applied to the union of per-scope survivors rather
    # than to the leftovers, and it is what actually caps accumulation.
    per_scope_kept = {}
    for scope in sorted(by_scope):
        if args.scope and scope not in args.scope:
            continue
        # Run ids start with a UTC timestamp, so reverse sorting is recency.
        for index, path in enumerate(sorted(by_scope[scope], reverse=True)):
            if index < args.keep:
                per_scope_kept[path] = f"within the last {args.keep} for scope {scope}"
    everything = sorted(per_scope_kept, key=lambda path: path.name, reverse=True)
    cited = referenced_run_ids(root, [path.name for path in considered])
    # A citation is the only reason a run outlives both windows: it is what a
    # later reader is told to open. Everything else is bounded by the windows.
    kept = {path: "cited by a tracked file" for path in considered if path.name in cited}
    windowed = [path for path in everything if path.name not in cited]
    if args.keep_global > 0:
        windowed = windowed[:args.keep_global]
    for path in windowed:
        kept[path] = per_scope_kept[path]
    # Everything considered but not kept must be deleted, including runs the
    # per-scope window already rejected.
    dropped = [path for path in considered if path not in kept]
    dropped_bytes = sum(directory_bytes(path) for path in dropped)
    for path in considered:
        if not args.quiet and path in kept:
            print(f"keep   {path.name}  ({kept[path]})")
    for path in dropped:
        print(f"{'delete' if args.apply else 'would delete'} {path.name}  ({directory_bytes(path) // 1024}K)")
    print(f"validation prune: kept {len(kept)}, "
          f"{'deleted' if args.apply else 'would delete'} {len(dropped)} run(s), "
          f"{dropped_bytes // 1048576}M reclaimed")
    # Directories without a manifest were not created by the runner, so no
    # retention window describes them and none of them are pruned. Reporting the
    # count keeps them from silently owning most of the disk: the plan wins only
    # over what it can see.
    if unmanaged:
        unmanaged_bytes = sum(directory_bytes(path) for path in unmanaged)
        print(f"validation prune: {len(unmanaged)} director(y/ies) under runs/ have no "
              f"manifest and are outside every retention window "
              f"({unmanaged_bytes // 1048576}M)");
    if not args.apply:
        if dropped:
            print("validation prune: this was a dry run; pass --apply to delete")
        return 0
    for path in dropped:
        shutil.rmtree(path, ignore_errors=True)
    return 0


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
    events, exit_code, child, declared, interrupted = GoResults(), 1, None, [], False
    def interrupt(signum, _frame):
        raise RunInterrupted(signum)
    prior_sigterm = signal.signal(signal.SIGTERM, interrupt)
    print(f"validation evidence: {output}", flush=True)
    try:
        if (root / "scripts" / "validation-inventory").is_dir():
            snapshot = inventory_data(root)
            declared = snapshot["goDeclarations"]
            write_json(output / "inventory.json", inventory_summary(snapshot))
        with (output / "output.log").open("w") as log:
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
    trim = sub.add_parser("prune", help="apply the evidence retention policy; dry run unless --apply")
    trim.add_argument("--root", type=pathlib.Path, default=ROOT)
    trim.add_argument("--keep", type=int, default=20, help="runs retained per scope (default 20)")
    trim.add_argument("--keep-global", type=int, default=50,
                      help="runs retained overall regardless of scope (default 50; 0 disables)")
    trim.add_argument("--scope", action="append", help="limit to a scope; repeatable")
    trim.add_argument("--apply", action="store_true", help="delete instead of reporting")
    trim.add_argument("--quiet", action="store_true", help="omit the per-run keep lines")
    args = parser.parse_args()
    try:
        if args.mode == "inventory":
            return inventory(args.root)
        if args.mode == "prune":
            return prune(args)
        return run(args)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"validation: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
