"""A one-off walkthrough remains runnable without inventing a state node."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("scene_goto", Path(__file__).resolve().parents[1] / "goto.py")
goto = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(goto)


class GotoProbeTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.node = {"id": "published", "dir": "published", "construct": "_build/construct.feature",
                     "scene_construct": True, "processes": [
                         {"surface": "feature", "file": "_probes/publish-dataset.feature"},
                         {"surface": "feature", "file": "_probes/sync-projection.feature"}]}
        (self.root / "published/_probes").mkdir(parents=True)
        (self.root / "published/_build").mkdir()
        (self.root / "published/_build/construct.feature").write_text("When I run `kc show`\n")
        (self.root / "published/_probes/publish-dataset.feature").write_text(
            "Feature: Publish\nScenario: Verify\nWhen I run `kc show`\nThen the output has:\n| id | catalog |\n")

    def test_explicit_probe_runs_only_after_its_shared_construct(self):
        calls = []
        with patch.object(goto, "ROOT", self.root), patch.object(goto, "walk_states", return_value=[self.node]), \
                patch.object(goto, "start_runtime"), \
                patch.object(goto, "apply_construct", side_effect=lambda s: calls.append("construct")), \
                patch.object(goto, "apply_feature", side_effect=lambda s, f: calls.append(f.name)):
            goto.apply_target("published", probe="publish-dataset.feature")
        self.assertEqual(calls, ["construct", "publish-dataset.feature"])

    def test_undeclared_probe_cannot_start_constructs(self):
        with patch.object(goto, "ROOT", self.root), patch.object(goto, "walk_states", return_value=[self.node]), \
                patch.object(goto, "start_runtime") as runtime:
            with self.assertRaises(goto.GotoError):
                goto.apply_target("published", probe="../other.feature")
            runtime.assert_not_called()

    def test_unsupported_probe_fails_before_mutating_live_fixture(self):
        (self.root / "published/_probes/publish-dataset.feature").write_text("Then error FORBIDDEN\n")
        with patch.object(goto, "ROOT", self.root), patch.object(goto, "walk_states", return_value=[self.node]), \
                patch.object(goto, "start_runtime") as runtime:
            with self.assertRaises(goto.GotoError):
                goto.apply_target("published", probe="publish-dataset.feature")
            runtime.assert_not_called()

    def test_unresolved_variables_fail_before_any_live_mutation(self):
        for relative in ["_build/construct.feature", "_probes/publish-dataset.feature"]:
            with self.subTest(feature=relative):
                feature = self.root / "published" / relative
                original = feature.read_text()
                feature.write_text("When I run `kc gate add example`\nWhen I run `kc gate remove $last.id`\n")
                try:
                    with patch.object(goto, "ROOT", self.root), \
                            patch.object(goto, "walk_states", return_value=[self.node]), \
                            patch.object(goto, "start_runtime") as runtime, \
                            patch.object(goto, "apply_construct") as construct, \
                            patch.object(goto, "run_kc", return_value={"id": "catalog"}) as command:
                        with self.assertRaisesRegex(goto.GotoError, r"unsupported variable.*\$last"):
                            goto.apply_target("published", probe="publish-dataset.feature")
                        runtime.assert_not_called()
                        construct.assert_not_called()
                        command.assert_not_called()
                finally:
                    feature.write_text(original)


if __name__ == "__main__":
    unittest.main()
