"""Evidence integrity tests; no external services or product test suite."""
import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest

SCRIPT = pathlib.Path(__file__).with_name("validation.py")


class ValidationEvidenceTest(unittest.TestCase):
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


if __name__ == "__main__":
    unittest.main()
