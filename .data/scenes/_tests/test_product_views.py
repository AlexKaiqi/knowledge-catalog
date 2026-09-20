"""Product promises are discovered from their owner, then linked by cases."""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("product_scene_tree", Path(__file__).resolve().parents[1] / "tree.py")
tree = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tree)


class ProductViewsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.root = self.repo / ".data/scenes"
        self.root.mkdir(parents=True)
        (self.repo / "docs/graph/documents").mkdir(parents=True)
        (self.repo / "docs/product.md").write_text(
            "# Product\n\n## 1. Introduction\n### U9: Outside section\n"
            "## 8. 用例\n### U1：发现已有知识\nThe consumer discovers knowledge.\n"
            "### U2：发表领域 Schema\nPublish through Writer.\n"
            "### U3：跨客户端消费\nA known missing journey.\n"
            "## 9. Invariants\n### U8: Outside section\n", encoding="utf-8")
        self.graph("product", "docs/product.md")
        self.config()
        self.node("root")
        self.node("root/attached", extra=(
            "construct_verifies:\n"
            "  - claim: 'product#U2'\n    detail: Writer receipt and read-back match the submitted schema.\n"
            "probes:\n"
            "  - file: discover.feature\n    source: discover\n    views: [engineering]\n"
            "    verifies:\n      - claim: 'product#U1'\n        detail: Consumer discovers the registered repository.\n"
            "  - file: unrelated.feature\n    source: recovery\n    views: [engineering]\n"))
        probes = self.root / "root/attached/_probes"
        probes.mkdir()
        for name in ("discover.feature", "unrelated.feature"):
            (probes / name).write_text("Feature: case\n", encoding="utf-8")

    def graph(self, document, path, *, filename="product.okf"):
        content = "---\nobject_id: documentation/" + document + "\n---\n" + json.dumps({"id": document, "path": path})
        (self.repo / "docs/graph/documents" / filename).write_text(content, encoding="utf-8")

    def config(self, suffix=""):
        (self.root / "_views.yaml").write_text(
            "views:\n  engineering:\n    title: Engineering\n"
            "product_documents:\n  product:\n    section: 8. 用例\n"
            "    gaps:\n      U3: The cross-client journey is not yet declared.\n" + suffix, encoding="utf-8")

    def node(self, relative, *, surface="feature", construct=True, extra=""):
        path = self.root / relative
        path.mkdir(parents=True, exist_ok=True)
        (path / "_meta.yaml").write_text("layer: app\nsurface: " + surface + "\nviews: [engineering]\n" + extra, encoding="utf-8")
        if construct:
            (path / "_build").mkdir(exist_ok=True)
            (path / "_build/construct.feature").write_text("Feature: construct\n", encoding="utf-8")

    def model(self):
        return tree.build_view(self.root)

    def errors(self):
        return "\n".join(tree.check_product(self.model())["errors"])

    def test_owner_headings_generate_views_and_gap_without_manual_members(self):
        model = self.model()
        views = model["views"]
        self.assertEqual([key for key in views if key.startswith("product/")],
                         ["product/product/U1", "product/product/U2", "product/product/U3"])
        self.assertEqual(views["product/product/U1"]["title"], "U1：发现已有知识")
        self.assertEqual(views["engineering"]["family"], "engineering")
        self.assertEqual(views["product/product/U1"]["family"], "product")
        claims = {claim["id"]: claim for claim in model["product"]["claims"]}
        self.assertEqual(claims["U1"]["status"], "linked")
        self.assertEqual(claims["U3"]["status"], "gap")
        self.assertEqual(claims["U1"]["execution_status"], "not_evaluated")
        self.assertEqual(tree.check_product(model)["errors"], [])

    def test_product_selects_case_links_not_host_tags_and_keeps_prerequisites(self):
        view = tree.project_view(self.model(), view_id="product/product/U1", from_id="attached")
        state, = view["states"]
        self.assertTrue(state["context"])
        self.assertEqual([proc["file"] for proc in state["processes"]], ["_probes/discover.feature"])
        self.assertFalse(state["construct_selected"])
        self.assertEqual(view["prerequisites"], ["root"])
        self.assertEqual(view["prerequisite_edges"], [{"parent": "root", "child": "attached", "kind": "build"}])

    def test_display_boundary_filters_case_links_but_retains_global_declaration_count(self):
        model = self.model()
        view = tree.project_view(model, view_id="product/product/U1", probes_at="root")
        self.assertEqual(view["states"], [])
        self.assertEqual(view["product_claim"]["links"], [])
        self.assertEqual(view["product_claim"]["declared_link_count"], 1)
        self.assertEqual(view["product_claim"]["selected_link_count"], 0)
        self.assertEqual(len(model["product"]["claims"][0]["links"]), 1)
        again = tree.project_view(model, view_id="product/product/U1", from_id="attached")
        self.assertEqual(again["product_claim"]["selected_link_count"], 1)

    def test_construct_evidence_does_not_change_legacy_process_inventory(self):
        model = self.model()
        attached = next(state for state in model["states"] if state["id"] == "attached")
        self.assertEqual(len(attached["processes"]), 2)
        view = tree.project_view(model, view_id="product/product/U2")
        attached = next(state for state in view["states"] if state["id"] == "attached")
        self.assertTrue(attached["construct_selected"])
        self.assertFalse(attached["context"])
        self.assertEqual(attached["processes"], [])
        self.assertEqual(tree.check_views(model)["totals"], {"nodes": 2, "edges": 1, "probes": 2, "evidence": 0})

    def test_new_owner_claim_is_automatically_reported_as_unlinked(self):
        path = self.repo / "docs/product.md"
        path.write_text(path.read_text().replace("## 9. Invariants", "### U11：New promise\n## 9. Invariants"), encoding="utf-8")
        self.assertIn("product#U11", self.errors())

    def test_stale_claim_reference_and_stale_gap_are_not_silently_ignored(self):
        path = self.root / "root/attached/_meta.yaml"
        path.write_text(path.read_text().replace("product#U1", "product#U404"), encoding="utf-8")
        self.config("      U405: Deleted claim.\n")
        errors = self.errors()
        self.assertIn("product#U404", errors)
        self.assertIn("U405", errors)

    def test_duplicate_document_and_claim_ids_are_rejected(self):
        self.graph("product", "docs/product.md", filename="duplicate.okf")
        self.assertIn("duplicate", self.errors())
        (self.repo / "docs/graph/documents/duplicate.okf").unlink()
        path = self.repo / "docs/product.md"
        path.write_text(path.read_text().replace("### U2", "### U1"), encoding="utf-8")
        self.assertIn("duplicate", self.errors())

    def test_missing_document_path_or_section_is_a_broken_source(self):
        for path, section in (("docs/missing.md", "8. 用例"), ("docs/product.md", "9. Missing")):
            with self.subTest(path=path, section=section):
                self.graph("product", path)
                config = self.root / "_views.yaml"
                self.config()
                config.write_text(config.read_text().replace("8. 用例", section), encoding="utf-8")
                self.assertTrue(self.errors())

    def test_unregistered_document_and_non_markdown_source_are_rejected(self):
        (self.repo / "docs/graph/documents/product.okf").unlink()
        self.assertIn("product", self.errors())
        (self.repo / "docs/product.html").write_text("derived", encoding="utf-8")
        self.graph("product", "docs/product.html")
        self.assertIn("Markdown", self.errors())

    def test_node_level_verifies_and_handwritten_product_membership_are_rejected(self):
        self.node("root", extra="verifies:\n  - claim: 'product#U1'\n    detail: Fake evidence on a state.\n")
        self.assertIn("node-level", self.errors())
        self.node("root")
        path = self.root / "root/_meta.yaml"
        path.write_text(path.read_text().replace("[engineering]", "[engineering, product/product/U1]"), encoding="utf-8")
        self.assertIn("handwritten", self.errors())

    def test_empty_detail_and_go_only_construct_do_not_satisfy_a_claim(self):
        path = self.root / "root/attached/_meta.yaml"
        path.write_text(path.read_text().replace("detail: Consumer discovers the registered repository.", "detail: ''"), encoding="utf-8")
        self.assertIn("detail", self.errors())
        self.node("root/attached", surface="go-test", extra=(
            "construct_verifies:\n  - claim: 'product#U2'\n    detail: Reference feature is not executable.\n"))
        for file in (self.root / "root/attached/_probes").glob("*.feature"):
            file.unlink()
        self.assertIn("construct", self.errors())

    def test_reference_only_probe_cannot_claim_actual_product_evidence(self):
        self.node("root/runtime", surface="go-test", extra=(
            "probes:\n  - file: reference.feature\n    source: runtime\n    views: [engineering]\n"
            "    verifies:\n      - claim: 'product#U3'\n        detail: This feature is not executed by any scene.\n"))
        directory = self.root / "root/runtime/_probes"
        directory.mkdir()
        (directory / "reference.feature").write_text("Feature: reference\n", encoding="utf-8")
        model = self.model()
        self.assertIn("executable", "\n".join(tree.check_product(model)["errors"]))
        claim = next(claim for claim in model["product"]["claims"] if claim["id"] == "U3")
        self.assertEqual(claim["links"], [])

    def test_go_evidence_requires_existing_named_tests_not_a_file_placeholder(self):
        case = "evidence:\n  - source: writer\n    views: [engineering]\n    tests:\n      - %s\n    verifies:\n      - claim: 'product#U1'\n        detail: Named test checks the discovery result.\n"
        self.node("root/checks", surface="go-test", construct=False, extra=case % "cli/evidence_test.go")
        self.assertIn("named", self.errors())
        self.node("root/checks", surface="go-test", construct=False, extra=case % "TestDiscovery")
        self.assertIn("TestDiscovery", self.errors())
        (self.repo / "cli").mkdir()
        (self.repo / "cli/evidence_test.go").write_text("package cli\nfunc TestDiscovery(t *testing.T) {}\n", encoding="utf-8")
        self.assertEqual(self.errors(), "")

    def test_links_and_explicit_remaining_gap_can_coexist(self):
        self.config("      U1: Only the CLI path is linked; browser navigation is still missing.\n")
        claim = self.model()["product"]["claims"][0]
        self.assertEqual(claim["status"], "linked_with_gap")
        self.assertTrue(claim["links"])
        self.assertTrue(claim["gap"])

    def test_cli_product_family_is_inventory_not_execution_and_product_lists_first(self):
        out = io.StringIO()
        with patch.object(tree, "ROOT", self.root), patch("sys.argv", ["tree.py", "--family", "product", "--json"]), contextlib.redirect_stdout(out):
            self.assertEqual(tree.main(), 0)
        data = json.loads(out.getvalue())
        self.assertEqual(data["family"], "product")
        self.assertEqual(data["execution_status"], "not_evaluated")
        self.assertEqual(len(data["claims"]), 3)
        out = io.StringIO()
        with patch.object(tree, "ROOT", self.root), patch("sys.argv", ["tree.py", "--list-views"]), contextlib.redirect_stdout(out):
            self.assertEqual(tree.main(), 0)
        self.assertLess(out.getvalue().index("Product"), out.getvalue().index("Engineering"))

    def test_headings_inside_fenced_examples_are_not_product_claims(self):
        path = self.repo / "docs/product.md"
        path.write_text(path.read_text().replace("### U2", "```markdown\n### U999: Example\n```\n### U2"), encoding="utf-8")
        self.assertEqual(self.errors(), "")
        self.assertEqual(len(self.model()["product"]["claims"]), 3)


if __name__ == "__main__":
    unittest.main()
