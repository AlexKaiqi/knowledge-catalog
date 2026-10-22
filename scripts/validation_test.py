"""Evidence integrity tests; no external services or product test suite."""
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest

SCRIPT = pathlib.Path(__file__).with_name("validation.py")


class ValidationEvidenceTest(unittest.TestCase):
    def test_run_records_active_selectors_without_retired_dolt_configuration(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("results/\n")
            output = root / "results"
            env = dict(os.environ, KC_E2E_RUN="TestSelected", KC_DOLT_DOCKER_IMAGE="retired:fixture",
                       KC_DOLT_FORCE_DOCKER="1")
            result = subprocess.run([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                     "--scope", "selector-fixture", "--output", str(output),
                                     "--", sys.executable, "-c", "print('selected')"],
                                    env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            report = json.loads(next(output.glob("*/report.json")).read_text())
            self.assertEqual(report["environment"]["KC_E2E_RUN"], "TestSelected")
            self.assertNotIn("KC_DOLT_DOCKER_IMAGE", report["environment"])
            self.assertNotIn("KC_DOLT_FORCE_DOCKER", report["environment"])

    def assert_invalid_evidence_fails_runner(self, source, status):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("results/\n")
            (root / "source.go").write_text("package example\n")
            output = root / "results"
            result = subprocess.run([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                     "--scope", "invalid-evidence-fixture", "--output", str(output),
                                     "--", sys.executable, "-c", source], capture_output=True, text=True)
            report = json.loads(next(output.glob("*/report.json")).read_text())
            self.assertEqual(report["status"], status)
            self.assertEqual(report["exitCode"], 0)
            self.assertEqual(report["executionStatus"], "passed")
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(report["runnerExitCode"], result.returncode)

    def test_failed_go_events_reject_successful_child_exit(self):
        events = [{"Action": "start", "Package": "kc/example"},
                  {"Action": "fail", "Package": "kc/example", "Test": "TestBroken"},
                  {"Action": "fail", "Package": "kc/example"}]
        source = "print(" + repr("\n".join(json.dumps(event) for event in events)) + ")"
        self.assert_invalid_evidence_fails_runner(source, "failed-events")

    def test_source_change_rejects_successful_child_exit(self):
        source = "from pathlib import Path; Path('source.go').write_text('package changed\\n')"
        self.assert_invalid_evidence_fails_runner(source, "source-changed")

    def test_go_events_keep_failed_skipped_and_repeated_executions(self):
        spec = importlib.util.spec_from_file_location("validation", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        events = module.GoResults()
        for action, name in [("start", None), ("pass", "TestOK"),
                             ("skip", "TestLive"), ("fail", "TestBroken"),
                             ("fail", None), ("start", None),
                             ("pass", "TestOK"), ("pass", None)]:
            events.add({"Action": action, "Package": "kc/example", "Test": name})
        self.assertEqual([r["status"] for r in events.tests], ["pass", "skip", "fail", "pass"])
        self.assertNotEqual(events.tests[0]["execution"], events.tests[-1]["execution"])
        self.assertEqual([r["status"] for r in events.packages], ["fail", "pass"])
        summary = events.summary([{"file": "example/example_test.go", "name": "TestNeverSelected"}])
        self.assertEqual(summary["testEvents"], {"pass": 2, "fail": 1, "skip": 1})
        self.assertEqual(summary["notObservedCount"], 1)

    def test_no_test_files_is_neither_test_success_nor_unexplained_package_skip(self):
        spec = importlib.util.spec_from_file_location("validation", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        events = module.GoResults()
        events.add({"Action": "start", "Package": "kc/fixture"})
        events.add({"Action": "output", "Package": "kc/fixture", "Output": "?   \tkc/fixture\t[no test files]\n"})
        events.add({"Action": "skip", "Package": "kc/fixture"})
        # Markers belong only to their execution. Unknown package skips and
        # actual test skips must retain their original conservative meaning.
        events.add({"Action": "start", "Package": "kc/fixture"})
        events.add({"Action": "skip", "Package": "kc/fixture"})
        events.add({"Action": "skip", "Package": "kc/other"})
        events.add({"Action": "skip", "Package": "kc/live", "Test": "TestExternal"})
        self.assertEqual([r["status"] for r in events.packages], ["no-tests", "skip", "skip"])
        self.assertEqual(events.packages[0]["goAction"], "skip")
        self.assertEqual(events.tests[0]["status"], "skip")
        summary = events.summary([])
        self.assertEqual(summary["testEvents"], {"pass": 0, "fail": 0, "skip": 1})
        self.assertEqual(summary["packageEvents"], {"pass": 0, "fail": 0, "skip": 2, "no-tests": 1})

    def test_sigterm_retains_interrupted_result(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("results/\n")
            output = root / "results"
            child = subprocess.Popen([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                      "--scope", "cancel-fixture", "--output", str(output), "--",
                                      sys.executable, "-u", "-c", "import time; print('started'); time.sleep(30)"],
                                     stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                for _ in range(100):
                    logs = list(output.glob("*/output.log"))
                    if logs and "started" in logs[0].read_text():
                        break
                    time.sleep(0.05)
                else:
                    self.fail("child never started")
                child.terminate()
                stdout, stderr = child.communicate(timeout=15)
                self.assertEqual(child.returncode, 143, stdout + stderr)
                report = json.loads(next(output.glob("*/report.json")).read_text())
                self.assertEqual(report["status"], "interrupted")
                self.assertEqual(report["executionStatus"], "interrupted")
            finally:
                if child.poll() is None:
                    child.kill()
                    child.communicate()

    def test_run_preserves_failure_and_binds_source_without_exposing_env(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / "source.go").write_text("package example\n")
            output = root / "ignored-results"
            (root / ".gitignore").write_text("ignored-results/\n")
            command = [sys.executable, "-c", "print('original output'); raise SystemExit(7)"]
            result = subprocess.run([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                     "--scope", "failure-fixture", "--output", str(output),
                                     "--", *command], capture_output=True, text=True)
            self.assertEqual(result.returncode, 7, result.stderr)
            reports = list(output.glob("*/report.json"))
            self.assertEqual(len(reports), 1)
            report = json.loads(reports[0].read_text())
            self.assertEqual(report["status"], "failed")
            self.assertEqual(report["scope"], "failure-fixture")
            self.assertEqual(report["exitCode"], 7)
            self.assertTrue(report["source"]["dirty"])
            self.assertEqual(len(report["source"]["fingerprint"]), 64)
            self.assertFalse(report["sourceChanged"])
            self.assertTrue(report["startedAt"] and report["finishedAt"] and report["runId"])
            self.assertEqual(report["tests"], [])
            self.assertEqual(report["resultDetail"], "command-exit-only")
            self.assertIn("original output", (reports[0].parent / "output.log").read_text())
            self.assertNotIn("KC_AUTH_TOKEN", report["environment"])


    def test_inventory_artifact_is_a_digest_not_a_copy(self):
        spec = importlib.util.spec_from_file_location("validation", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        snapshot = {"kind": "declared-inventory", "counts": {"goDeclarations": 3},
                    "limitations": ["declarations are not results"],
                    "goDeclarations": [{"name": f"Test{index}"} for index in range(500)]}
        summary = module.inventory_summary(snapshot)
        # The rebuildable payload must not be copied into per-run evidence.
        self.assertNotIn("goDeclarations", summary)
        self.assertEqual(summary["counts"], snapshot["counts"])
        self.assertEqual(len(summary["digest"]), 64)
        # The same inventory must digest identically, and any change must not.
        self.assertEqual(summary["digest"], module.inventory_summary(dict(snapshot))["digest"])
        changed = dict(snapshot, counts={"goDeclarations": 4})
        self.assertNotEqual(summary["digest"], module.inventory_summary(changed)["digest"])

    def assert_run_writes_only_stratified_artifacts(self, root, output, command):
        result = subprocess.run([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                 "--scope", "artifact-fixture", "--output", str(output),
                                 "--", *command], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        run = next(output.glob("*/"))
        written = sorted(path.name for path in run.iterdir() if path.is_file())
        # go-test.jsonl duplicated output.log line for line, so it must be gone.
        self.assertNotIn("go-test.jsonl", written)
        self.assertIn("output.log", written)
        self.assertIn("report.json", written)
        self.assertIn("manifest.json", written)
        return run

    def test_run_does_not_duplicate_raw_events_in_a_second_file(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("results/\n")
            (root / "source.go").write_text("package example\n")
            events = [{"Action": "start", "Package": "kc/example"},
                      {"Action": "pass", "Package": "kc/example", "Test": "TestOK"}]
            source = "print(" + repr("\n".join(json.dumps(event) for event in events)) + ")"
            run = self.assert_run_writes_only_stratified_artifacts(
                root, root / "results", [sys.executable, "-c", source])
            report = json.loads((run / "report.json").read_text())
            # Parsed events survive in report.json even without the raw copy.
            self.assertEqual([row["status"] for row in report["tests"]], ["pass"])

    def test_prune_keeps_recent_and_cited_runs_and_ignores_foreign_content(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text(".validation/\n")
            runs = root / ".validation" / "runs"
            runs.mkdir(parents=True)
            # Newest first by name; the name carries the UTC timestamp.
            names = ["20260101T000000Z-aaaaaaaaaaaa", "20260102T000000Z-bbbbbbbbbbbb",
                     "20260103T000000Z-cccccccccccc"]
            for name in names:
                (runs / name).mkdir()
                (runs / name / "manifest.json").write_text(json.dumps({"scope": "s"}))
                (runs / name / "report.json").write_text("{}\n")
            # A cited run must survive even when it falls outside the window.
            (root / "TASK.md").write_text(f"evidence: {names[0]}\n")
            (root / "source.go").write_text("package example\n")
            # Content the runner did not create must never be pruned here.
            (runs / "hand-written").mkdir()
            (runs / "hand-written" / "notes.txt").write_text("keep me\n")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            result = subprocess.run([sys.executable, str(SCRIPT), "prune", "--root", str(root),
                                     "--keep", "1", "--keep-global", "0", "--apply"],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue((runs / names[0]).is_dir(), "cited run must be kept")
            self.assertTrue((runs / names[2]).is_dir(), "newest run must be kept")
            self.assertFalse((runs / names[1]).is_dir(), "uncited older run must go")
            self.assertTrue((runs / "hand-written" / "notes.txt").is_file())
            # Not pruning foreign content is right, but staying silent about it
            # is not: this repository reached 2.4G of hand-written evidence that
            # no window could ever reclaim, because the summary counted only
            # what the plan could see. The leftover must be reported.
            self.assertIn("have no manifest and are outside every retention window",
                          result.stdout)
            self.assertIn("1 director", result.stdout)

    def test_run_writes_inventory_summary_not_the_rebuildable_payload(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("results/\n")
            (root / "source.go").write_text("package example\n")
            # The runner shells out to `go run ./scripts/validation-inventory`;
            # a stub keeps this test offline and fast while still exercising the
            # write path, so a regression that stores the full inventory fails.
            (root / "scripts" / "validation-inventory").mkdir(parents=True)
            payload = {"kind": "declared-inventory", "counts": {"goDeclarations": 2},
                       "limitations": [],
                       "goDeclarations": [{"name": "TestA", "file": "example/a_test.go"},
                                          {"name": "TestB", "file": "example/b_test.go"}]}
            stub = root / "fake-go"
            stub.write_text("#!" + sys.executable + "\nimport json\nprint(json.dumps("
                            + repr(payload) + "))\n")
            stub.chmod(0o700)
            output = root / "results"
            env = dict(os.environ, GO=str(stub))
            result = subprocess.run([sys.executable, str(SCRIPT), "run", "--root", str(root),
                                     "--scope", "inventory-fixture", "--output", str(output),
                                     "--", sys.executable, "-c", "print('ok')"],
                                    env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            written = json.loads(next(output.glob("*/inventory.json")).read_text())
            self.assertEqual(written["kind"], "declared-inventory-summary")
            self.assertNotIn("goDeclarations", written)
            self.assertEqual(written["counts"], payload["counts"])
            self.assertEqual(len(written["digest"]), 64)

    def test_prune_global_window_bounds_one_off_scopes(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text(".validation/\n")
            runs = root / ".validation" / "runs"
            # Each run has its own scope, which is how ad-hoc experiments are
            # actually recorded; per-scope retention alone would keep all of them.
            names = []
            for day in range(1, 11):
                name = f"202601{day:02d}T000000Z-{day:012d}"
                names.append(name)
                (runs / name).mkdir(parents=True)
                (runs / name / "manifest.json").write_text(json.dumps({"scope": f"one-off-{day}"}))
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            result = subprocess.run([sys.executable, str(SCRIPT), "prune", "--root", str(root),
                                     "--keep", "20", "--keep-global", "3", "--apply"],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            remaining = sorted(path.name for path in runs.iterdir() if path.is_dir())
            self.assertEqual(remaining, sorted(names[-3:]))

    def test_prune_is_a_dry_run_without_apply(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text(".validation/\n")
            runs = root / ".validation" / "runs"
            for name in ["20260101T000000Z-aaaaaaaaaaaa", "20260102T000000Z-bbbbbbbbbbbb"]:
                (runs / name).mkdir(parents=True)
                (runs / name / "manifest.json").write_text(json.dumps({"scope": "s"}))
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            result = subprocess.run([sys.executable, str(SCRIPT), "prune", "--root", str(root),
                                     "--keep", "1"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("would delete", result.stdout)
            self.assertTrue((runs / "20260101T000000Z-aaaaaaaaaaaa").is_dir())


if __name__ == "__main__":
    unittest.main()
