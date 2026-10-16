"""All Go evidence names are checked even without a product claim link."""
from __future__ import annotations

import contextlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("all_meta_tree", Path(__file__).resolve().parents[1] / "tree.py")
tree = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tree)


class MetaGoTestNames(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.repo = Path(temp.name)
        self.root = self.repo / ".data/scenes"
        (self.root / "state/_build").mkdir(parents=True)
        (self.root / "state/_build/construct.feature").write_text("Feature: fixture\n", encoding="utf-8")
        (self.repo / "doc/design").mkdir(parents=True)
        (self.repo / "doc/design/example.md").write_text("# Example\n## Cases\n### U1: A future journey\n", encoding="utf-8")
        (self.root / "_views.yaml").write_text(
            "views:\n  engineering:\n    title: Engineering\nproduct_documents:\n  example:\n    section: Cases\n    gaps:\n      U1: No journey claims this evidence.\n", encoding="utf-8")
        (self.repo / "contract").mkdir()
        self.go = self.repo / "contract/oracle_test.go"
        self.go.write_text("package contract\nimport \"testing\"\nfunc TestPresent(t *testing.T) {}\n", encoding="utf-8")

    def evidence(self, name, extra=""):
        (self.root / "state/_meta.yaml").write_text(
            "layer: app\nsurface: feature\nviews: [engineering]\nfixture: A shared persisted fixture.\n"
            "evidence:\n  - id: independent-oracle\n    source: regression\n    views: [engineering]\n"
            "    tests:\n      - " + name + "\n" + extra, encoding="utf-8")

    def errors(self):
        return "\n".join(tree.build_view(self.root)["product"]["errors"])

    def test_missing_test_without_verifies_is_rejected(self):
        self.evidence("TestMissingWithoutProductLink")
        self.assertIn("TestMissingWithoutProductLink", self.errors())
        self.assertIn("independent-oracle", self.errors())

    def test_missing_test_with_empty_verifies_is_rejected(self):
        self.evidence("TestMissingWithEmptyLinks", "    verifies: []\n")
        self.assertIn("TestMissingWithEmptyLinks", self.errors())

    def test_validation_does_not_require_registered_product_documents(self):
        self.evidence("TestMissingWithoutDocuments")
        (self.root / "_views.yaml").write_text("views:\n  engineering:\n    title: Engineering\n", encoding="utf-8")
        self.assertIn("TestMissingWithoutDocuments", self.errors())

    def test_whole_file_reference_without_verifies_is_rejected(self):
        self.evidence("contract/oracle_test.go")
        self.assertIn("named Go tests", self.errors())
        self.assertIn("contract/oracle_test.go", self.errors())

    def test_existing_test_without_verifies_is_valid_and_not_product_coverage(self):
        self.evidence("TestPresent")
        model = tree.build_view(self.root)
        self.assertEqual(model["product"]["errors"], [])
        claim, = model["product"]["claims"]
        self.assertEqual(claim["links"], [])
        self.assertEqual(claim["status"], "gap")
        self.go.write_text(self.go.read_text().replace("TestPresent", "TestRenamed"), encoding="utf-8")
        self.assertIn("TestPresent", self.errors())

    def test_existing_integrated_check_reports_unlinked_missing_name(self):
        self.evidence("TestMissingInCheckCommand")
        out = io.StringIO()
        with patch.object(tree, "ROOT", self.root), patch("sys.argv", ["tree.py", "--check-product"]), contextlib.redirect_stdout(out):
            self.assertEqual(tree.main(), 1)
        self.assertIn("TestMissingInCheckCommand", out.getvalue())


if __name__ == "__main__":
    unittest.main()
