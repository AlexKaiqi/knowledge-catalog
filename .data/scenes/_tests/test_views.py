"""Regression cases for projections of the one physical scene tree."""
from __future__ import annotations

import importlib.util
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("scene_tree", Path(__file__).resolve().parents[1] / "tree.py")
tree = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tree)


class SceneViewsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "_views.yaml").write_text(
            "views:\n"
            "  access-control:\n    title: Access\n    description: Permissions\n"
            "  publication:\n    title: Publish\n    description: Publishing\n    entry: attached\n"
            "  consumption:\n    title: Consume\n    description: Reading\n"
            "  lifecycle:\n    title: Life\n    description: Recovery\n", encoding="utf-8")
        self.node("root", ["access-control"])
        self.node("root/attached", ["access-control", "publication"], probes=[
            ("deny.feature", ["access-control"]),
            ("publish.feature", ["publication"]),
            ("shared.feature", ["access-control", "publication"]),
        ])
        self.node("root/attached/middle", ["consumption"])
        self.node("root/attached/middle/published", ["publication"])
        self.node("root/attached/schema-checks", ["consumption"], construct=False,
                  evidence=[("schema.browse", ["TestBrowse"], ["consumption"])])
        self.node("other-root", ["lifecycle"])

    def node(self, relative, views, *, construct=True, probes=(), evidence=()):
        path = self.root / relative
        path.mkdir(parents=True, exist_ok=True)
        # Inline lists are part of the documented metadata shape.
        lines = ["layer: app", "surface: " + ("feature" if construct else "go-test"),
                 "views: [" + ", ".join(views) + "]"]
        if probes:
            lines.append("probes:")
            (path / "_probes").mkdir(exist_ok=True)
            for file, tags in probes:
                lines += [f"  - file: {file}", "    source: writer.snapshot",
                          "    views: [" + ", ".join(tags) + "]"]
                (path / "_probes" / file).write_text("Feature: sample\n", encoding="utf-8")
        if evidence:
            lines.append("evidence:")
            for source, tests, tags in evidence:
                lines += [f"  - source: {source}", "    tests:"]
                lines += [f"      - {name}" for name in tests]
                lines += ["    views: [" + ", ".join(tags) + "]"]
        (path / "_meta.yaml").write_text("\n".join(lines) + "\n", encoding="utf-8")
        if construct:
            (path / "_build").mkdir(exist_ok=True)
            (path / "_build" / "construct.feature").write_text("Feature: build\n", encoding="utf-8")

    def model(self):
        return tree.build_view(self.root)

    def test_view_starts_at_entry_keeps_real_connections_and_selects_each_probe(self):
        view = tree.project_view(self.model(), view_id="publication")
        states = {state["id"]: state for state in view["states"]}
        self.assertEqual(set(states), {"attached", "middle", "published"})
        self.assertTrue(states["middle"]["context"])
        self.assertEqual([node["id"] for node in view["tree"]], ["attached"])
        self.assertEqual({proc["file"] for proc in states["attached"]["processes"]},
                         {"_probes/publish.feature", "_probes/shared.feature"})
        self.assertEqual({(edge["parent"], edge["child"]) for edge in view["edges"]},
                         {("attached", "middle"), ("middle", "published")})
        self.assertEqual(view["prerequisites"], ["root"])
        self.assertEqual(view["prerequisite_edges"], [
            {"parent": "root", "child": "attached", "kind": "build"}])

    def test_from_hides_ancestors_without_losing_construction_dependencies(self):
        view = tree.project_view(self.model(), view_id="publication", from_id="published")
        self.assertEqual([state["id"] for state in view["states"]], ["published"])
        self.assertEqual(view["prerequisites"], ["root", "attached", "middle"])
        self.assertEqual(view["edges"], [])
        self.assertEqual(len(view["prerequisite_edges"]), 3)

    def test_probes_at_returns_only_host_and_marks_go_only_groups(self):
        view = tree.project_view(self.model(), probes_at="schema-checks")
        state, = view["states"]
        self.assertEqual(state["kind"], "evidence-group")
        self.assertFalse(state["executable"])
        self.assertEqual(state["processes"][0]["evidence"], ["TestBrowse"])
        self.assertEqual(view["prerequisites"], [])
        self.assertEqual(view["prerequisite_edges"][-1]["kind"], "evidence")

    def test_case_membership_does_not_inherit_host_membership(self):
        self.node("root/attached", ["access-control"], probes=[("deny.feature", ["publication"])])
        # Remove fixture probes no longer declared, preserving the file/tag guard.
        for name in ("publish.feature", "shared.feature"):
            (self.root / "root/attached/_probes" / name).unlink()
        view = tree.project_view(self.model(), view_id="publication")
        attached = next(state for state in view["states"] if state["id"] == "attached")
        self.assertTrue(attached["context"])
        self.assertEqual([proc["file"] for proc in attached["processes"]], ["_probes/deny.feature"])

    def test_union_deduplicates_nodes_edges_and_shared_cases(self):
        result = tree.check_views(self.model())
        self.assertEqual(result["errors"], [])
        self.assertEqual(result["totals"], {"nodes": 6, "edges": 4, "probes": 3, "evidence": 1})
        self.assertEqual(result["covered"], result["totals"])

    def test_union_rejects_missing_case_tag_even_when_host_is_tagged(self):
        path = self.root / "root/attached/_meta.yaml"
        path.write_text(path.read_text().replace("views: [publication]", "views: []"), encoding="utf-8")
        result = tree.check_views(self.model())
        self.assertTrue(any("publish.feature" in error and "views" in error for error in result["errors"]), result)
        self.assertEqual(result["covered"]["probes"], 2)

    def test_union_reports_an_entire_missing_branch_and_its_edge(self):
        self.node("unclassified", [])
        self.node("unclassified/unclassified-child", [])
        result = tree.check_views(self.model())
        self.assertEqual(result["missing"]["nodes"], ["unclassified", "unclassified-child"])
        self.assertIn(("unclassified", "unclassified-child"), result["missing"]["edges"])

    def test_go_evidence_is_filtered_independently_and_deduplicated_across_views(self):
        self.node("root/attached/schema-checks", ["consumption"], construct=False, evidence=[
            ("schema.browse", ["TestBrowse"], ["consumption", "lifecycle"]),
            ("schema.auth", ["TestDenied"], ["access-control"]),
        ])
        view = tree.project_view(self.model(), view_id="access-control", probes_at="schema-checks")
        state, = view["states"]
        self.assertTrue(state["context"])
        self.assertEqual([p["evidence"] for p in state["processes"]], [["TestDenied"]])
        result = tree.check_views(self.model())
        self.assertEqual(result["errors"], [])
        self.assertEqual(result["covered"]["evidence"], 2)

    def test_check_cli_exits_nonzero_and_keeps_machine_readable_omissions(self):
        self.node("unclassified", [])
        out = io.StringIO()
        with patch.object(tree, "ROOT", self.root), patch("sys.argv", ["tree.py", "--check-views", "--json"]), contextlib.redirect_stdout(out):
            code = tree.main()
        self.assertEqual(code, 1)
        self.assertIn("unclassified", json.loads(out.getvalue())["missing"]["nodes"])

    def test_metadata_does_not_allow_a_second_membership_tree(self):
        path = self.root / "_views.yaml"
        path.write_text(path.read_text().replace("title: Access", "title: Access\n    members: [root]"), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "no member or edge lists"):
            self.model()

    def test_case_titles_and_declared_identity_survive_projection(self):
        path = self.root / "root/attached/schema-checks/_meta.yaml"
        path.write_text(path.read_text().replace("  - source: schema.browse",
            "  - id: bounded-schema-browse\n    title: Schema browse stays bounded\n    source: schema.browse"), encoding="utf-8")
        path = self.root / "root/attached/_meta.yaml"
        path.write_text(path.read_text().replace("  - file: deny.feature",
            "  - file: deny.feature\n    title: Unauthorized writer is denied"), encoding="utf-8")
        view = tree.project_view(self.model())
        state = next(state for state in view["states"] if state["id"] == "schema-checks")
        case, = state["processes"]
        self.assertEqual(case["case_id"], "bounded-schema-browse")
        self.assertEqual(case["title"], "Schema browse stays bounded")
        self.assertEqual(case["evidence"], ["TestBrowse"])
        text = tree.text_tree(view["tree"])
        self.assertIn("Schema browse stays bounded", text)
        self.assertIn("Unauthorized writer is denied", text)
        self.assertNotIn("TestBrowse", text)
        self.assertIn("TestBrowse", tree.text_tree(view["tree"], detailed=True))

    def test_unknown_tag_and_nodes_outside_entry_are_reported(self):
        self.node("other-root", ["typo", "publication"])
        result = tree.check_views(self.model())
        self.assertTrue(any("typo" in error for error in result["errors"]), result)
        self.assertIn("other-root", result["missing"]["nodes"])

    def test_no_root_required_and_full_view_is_a_forest(self):
        view = tree.project_view(self.model())
        self.assertEqual({node["id"] for node in view["tree"]}, {"root", "other-root"})
        self.assertEqual(view["prerequisites"], [])

    def test_unknown_selection_and_impossible_entry_are_errors(self):
        model = self.model()
        for selection in ({"view_id": "typo"}, {"from_id": "typo"}, {"probes_at": "typo"}):
            with self.subTest(selection=selection), self.assertRaises(ValueError):
                tree.project_view(model, **selection)
        model["views"]["publication"]["entry"] = "typo"
        self.assertTrue(tree.check_views(model)["errors"])

    def test_nonconstructable_ancestor_is_not_a_buildable_prerequisite(self):
        self.node("root/attached/schema-checks/unsafe-child", ["consumption"])
        view = tree.project_view(self.model(), from_id="unsafe-child")
        state, = view["states"]
        self.assertFalse(state["executable"])
        self.assertIn("schema-checks", state["unbuildable_ancestors"])

    def test_go_runtime_feature_is_reference_not_scene_construct(self):
        self.node("root/runtime", ["lifecycle"], evidence=[("runtime", ["TestRuntime"], ["lifecycle"])])
        path = self.root / "root/runtime/_meta.yaml"
        path.write_text(path.read_text().replace("surface: feature", "surface: go-test"), encoding="utf-8")
        self.node("root/runtime/runtime-child", ["lifecycle"])
        model = self.model()
        runtime = next(state for state in model["states"] if state["id"] == "runtime")
        self.assertEqual(runtime["construct"], "_build/construct.feature")
        self.assertEqual(runtime["kind"], "evidence-group")
        self.assertFalse(runtime["scene_construct"])
        self.assertFalse(runtime["executable"])
        self.assertIn({"parent": "root", "child": "runtime", "kind": "evidence"}, model["edges"])
        view = tree.project_view(model, from_id="runtime-child")
        child, = view["states"]
        self.assertFalse(child["executable"])
        self.assertEqual(child["unbuildable_ancestors"], ["runtime"])
        self.assertNotIn("runtime", view["prerequisites"])


if __name__ == "__main__":
    unittest.main()
