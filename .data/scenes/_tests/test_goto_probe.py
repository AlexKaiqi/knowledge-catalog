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
    def test_feature_comments_preserve_fragments_and_quoted_command_content(self):
        command = 'kc read urn:kc:object:semantic-model/shop.sales#/properties'
        source = ('# before feature\nFeature: Fragment read # actual comment\n'
                  'Scenario: Verify\nWhen I run `'+command+'` # after command\n'
                  'Then the output has: # after assertion\n'
                  '| address | urn:kc:object:semantic-model/shop.sales#/properties | # after row\n')
        steps = goto.parse_feature(source)
        self.assertEqual(steps, [{'kind': 'run', 'command': command},
                                {'kind': 'has', 'rows': [('address', 'urn:kc:object:semantic-model/shop.sales#/properties')]}])
        for command in [
            'kc search --dataset example --query "sales # tag"',
            "kc invoke --repo example --input '{\"text\": \"value # tag\"}'",
            r'kc search --dataset example --query "quoted \\" # tag"',
        ]:
            with self.subTest(command=command):
                self.assertEqual(goto.parse_feature('When I run `'+command+'` # comment'),
                                 [{'kind': 'run', 'command': command}])

    def test_feature_comment_quoting_and_escape_boundaries(self):
        for raw, wanted in [
            ('# full comment', ''),
            ('value # comment', 'value '),
            ('urn:kc:object:item#/properties # comment', 'urn:kc:object:item#/properties '),
            ('"quoted # value" # comment', '"quoted # value" '),
            ("'quoted # value' # comment", "'quoted # value' "),
            (r'"escaped \" # value" # comment', r'"escaped \" # value" '),
            ('`quoted # value` # comment', '`quoted # value` '),
        ]:
            with self.subTest(raw=raw):
                self.assertEqual(goto.strip_feature_comment(raw), wanted)

    def test_catalog_read_construct_remains_live_compatible(self):
        feature = (Path(__file__).resolve().parents[1] / "service-initialized" /
                   "inventory-read-granted" / "_build" / "construct.feature")
        goto.validate_live_feature(feature)

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
                feature.write_text("When I run `kc hook add example`\nWhen I run `kc hook remove $last.id`\n")
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

    def test_retired_per_repository_selection_is_rejected_before_setup(self):
        given = "Given existing repository kr://scene/object with LiveAccess instance main\n"
        with self.assertRaisesRegex(goto.GotoError, "plugin instances come from deployment"):
            goto.parse_feature(given)
        self.assertEqual(goto.parse_feature("Given existing repository kr://scene/object\n"), [{"kind": "skip"}])
        feature = self.root / "published/_build/construct.feature"
        feature.write_text(given + "When I run `kc attach --repo kr://scene/object`\n")
        with patch.object(goto, "ROOT", self.root), \
                patch.object(goto, "walk_states", return_value=[self.node]), \
                patch.object(goto, "start_runtime") as runtime, \
                patch.object(goto, "apply_construct") as construct, \
                patch.object(goto, "run_kc") as command:
            with self.assertRaisesRegex(goto.GotoError, "plugin instances come from deployment"):
                goto.apply_target("published")
            runtime.assert_not_called()
            construct.assert_not_called()
            command.assert_not_called()


if __name__ == "__main__":
    unittest.main()
