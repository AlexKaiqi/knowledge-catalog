"""Public runner request regressions using a client double; no services."""
import tempfile
import subprocess
import sys
import unittest
import urllib.parse
from pathlib import Path

import ladder
import smoke
from perf_tree import check_env


class ProtocolClient:
    def __init__(self):
        self.calls = []

    def repo_seg(self, repository):
        return urllib.parse.quote(repository, safe="")

    def call(self, method, path, body=None):
        self.calls.append((method, path, body))
        if method == "POST" and path == "/snapshot/v1/connections":
            name = body["name"]
            assert set(body) <= {"name", "store"}, f"creation has a selector: {body}"
            return 0.0, 200, {"repositoryId": name}
        if method == "POST" and path == "/snapshot/v1/repositories":
            assert set(body) == {"repository"}, f"admission has a selector: {body}"
            return 0.0, 200, {"repositoryId": body["repository"]}
        if method == "GET" and path.endswith("/head"):
            return 0.0, 200, {"commit": "c0"}
        raise AssertionError(f"unexpected public operation: {method} {path}")


class RunnerProtocolTest(unittest.TestCase):
    def test_scratch_creation_uses_connection_and_inventory_surfaces(self):
        env = {"target": {"serverURL": "http://unused.invalid", "principal": "perf"}}
        for run_type in (smoke.SmokeRun, ladder.LadderRun):
            with self.subTest(runner=run_type.__name__), tempfile.TemporaryDirectory() as temp:
                run = run_type(env, Path(temp))
                client = ProtocolClient()
                run.client = client
                run.create_repository()
                self.assertEqual([path for _, path, _ in client.calls[:2]],
                                 ["/snapshot/v1/connections", "/snapshot/v1/repositories"])
                self.assertEqual(client.calls[1][2], {"repository": run.repo})

    def test_environment_rejects_retired_catalog_selection(self):
        source = (Path(__file__).parents[1] / "scenes" / "env.example.yaml").read_text()
        for name in ("catalog", "Catalog", "CATALOGID", "catalogs"):
            with self.subTest(field=name), tempfile.TemporaryDirectory() as temp:
                path = Path(temp) / "env.yaml"
                path.write_text(source.replace("target:\n", f"target:\n  {name}: kr://retired/selection\n"))
                errors, _ = check_env(path)
                self.assertTrue(any("catalog" in error.lower() for error in errors), errors)
                for script in ("smoke.py", "ladder.py"):
                    out = Path(temp) / script / "evidence"
                    result = subprocess.run([sys.executable, str(Path(__file__).parent / script),
                                             "--env", str(path), "--out", str(out)],
                                            capture_output=True, text=True, timeout=10)
                    self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
                    self.assertFalse(out.exists(), "invalid selection must not start a run")

    def test_environment_rejects_scalar_target_before_binding(self):
        source = (Path(__file__).parents[1] / "scenes" / "env.example.yaml").read_text()
        start = source.index("target:")
        end = source.index("profile:", start)
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "env.yaml"
            path.write_text(source[:start] + "target: false\n" + source[end:])
            errors, _ = check_env(path)
            self.assertIn("target must be a mapping", errors)
            for script in ("smoke.py", "ladder.py"):
                out = Path(temp) / script / "evidence"
                result = subprocess.run([sys.executable, str(Path(__file__).parent / script),
                                         "--env", str(path), "--out", str(out)],
                                        capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
                self.assertFalse(out.exists(), "invalid target must not start a run")



if __name__ == "__main__":
    unittest.main()
