"""Product promises are discovered from their owner, then linked by cases."""
from __future__ import annotations

import contextlib
import copy
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
        (self.repo / "doc/design").mkdir(parents=True)
        (self.repo / "doc/design/product.md").write_text(
            "# Product\n\n## 1. Introduction\n### U9: Outside section\n"
            "## 8. 用例\n### U1：发现已有对象\nThe consumer discovers object.\n"
            "### U2：发表领域 Schema\nPublish through Writer.\n"
            "### U3：跨客户端消费\nA known missing journey.\n"
            "## 9. Invariants\n### U8: Outside section\n", encoding="utf-8")
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

    def specification(self, *, case_id="liveaccess-read", claims=None):
        return {"id": case_id, "title": "Read the selected external value",
                "claims": claims or ["product#U3"], "given": ["An external value is declared."],
                "when": ["Read the object through the public entry."],
                "then": ["Observe value: running."], "risk": "The implementation returns a declaration instead of data."}

    def write_specifications(self, cases, *, relative="root/attached", filename="liveaccess.yaml"):
        directory = self.root / relative / "_specs"
        directory.mkdir(parents=True, exist_ok=True)
        lines = []
        for case in cases:
            for index, (key, value) in enumerate(case.items()):
                prefix = "- " if index == 0 else "  "
                if isinstance(value, list) and value:
                    lines.append(prefix + key + ":")
                    lines.extend("    - " + json.dumps(item, ensure_ascii=False) for item in value)
                else:
                    lines.append(prefix + key + ": " + json.dumps(value, ensure_ascii=False))
        path = directory / filename
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")
        return path

    def test_owner_headings_generate_views_and_gap_without_manual_members(self):
        model = self.model()
        views = model["views"]
        self.assertEqual([key for key in views if key.startswith("product/")],
                         ["product/product/U1", "product/product/U2", "product/product/U3"])
        self.assertEqual(views["product/product/U1"]["title"], "U1：发现已有对象")
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
        path = self.repo / "doc/design/product.md"
        path.write_text(path.read_text().replace("## 9. Invariants", "### U11：New promise\n## 9. Invariants"), encoding="utf-8")
        self.assertIn("product#U11", self.errors())

    def test_stale_claim_reference_and_stale_gap_are_not_silently_ignored(self):
        path = self.root / "root/attached/_meta.yaml"
        path.write_text(path.read_text().replace("product#U1", "product#U404"), encoding="utf-8")
        self.config("      U405: Deleted claim.\n")
        errors = self.errors()
        self.assertIn("product#U404", errors)
        self.assertIn("U405", errors)

    def test_duplicate_claim_ids_are_rejected(self):
        path = self.repo / "doc/design/product.md"
        path.write_text(path.read_text().replace("### U2", "### U1"), encoding="utf-8")
        self.assertIn("duplicate", self.errors())

    def test_missing_section_is_a_broken_source(self):
        config = self.root / "_views.yaml"
        config.write_text(config.read_text().replace("8. 用例", "9. Missing"), encoding="utf-8")
        self.assertIn("9. Missing", self.errors())

    def test_missing_design_document_is_rejected(self):
        (self.repo / "doc/design/product.md").unlink()
        self.assertIn("doc/design/product.md does not exist", self.errors())

    def test_document_id_cannot_escape_the_design_directory(self):
        for document_id in ("../product", "design/product", "Product"):
            with self.subTest(document_id=document_id):
                config = self.root / "_views.yaml"
                self.config()
                config.write_text(config.read_text().replace("  product:\n    section", "  '" + document_id + "':\n    section"), encoding="utf-8")
                self.assertIn("must name a chapter in doc/design/", self.errors())

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

    def test_specifications_are_discoverable_without_becoming_execution_evidence(self):
        before = self.model()
        case = self.specification()
        self.write_specifications([case])
        model = self.model()
        claim = next(item for item in model["product"]["claims"] if item["id"] == "U3")
        specification, = claim["specifications"]
        self.assertEqual(specification["id"], case["id"])
        self.assertEqual(specification["then"], case["then"])
        self.assertEqual(specification["state"], "attached")
        self.assertEqual(specification["file"], "root/attached/_specs/liveaccess.yaml")
        self.assertEqual(specification["execution_status"], "not_evaluated")
        self.assertEqual(claim["status"], "gap")
        self.assertEqual(claim["links"], [])
        self.assertEqual(tree.check_product(model)["errors"], [])
        self.assertEqual(tree.case_ids(model["states"]), tree.case_ids(before["states"]))
        self.assertEqual(tree.check_views(model)["totals"], tree.check_views(before)["totals"])
        text = tree.product_tools.inventory_text(model["product"])
        self.assertIn("specification liveaccess-read", text)
        self.assertIn(case["then"][0], text)
        self.assertIn("not_evaluated", text)

    def test_specifications_do_not_replace_explicit_missing_execution_gaps(self):
        self.write_specifications([self.specification()])
        self.config()
        path = self.root / "_views.yaml"
        path.write_text(path.read_text().replace("      U3: The cross-client journey is not yet declared.\n", ""), encoding="utf-8")
        self.assertIn("neither an actual case link nor an explicit gap", self.errors())

    def test_specification_shape_rejects_blank_steps_unknown_claims_and_runtime_fields(self):
        original = self.specification()
        invalid = [{"id": ""}, {"title": " "}, {"risk": ""}, {"claims": []},
                   {"claims": ["product#U404"]}, {"claims": ["product#U3", "product#U3"]},
                   {"given": []}, {"when": "Read the object."}, {"then": [""]},
                   {"then": [False]}, {"tests": ["TestFakePass"]},
                   {"verifies": []}, {"status": "PASS"}]
        for replacement in invalid:
            with self.subTest(replacement=replacement):
                case = copy.deepcopy(original)
                case.update(replacement)
                self.write_specifications([case])
                self.assertTrue(self.errors())

    def test_duplicate_specification_ids_across_hosts_are_rejected(self):
        self.write_specifications([self.specification()])
        self.write_specifications([self.specification()], relative="root")
        self.assertIn("duplicate specification id", self.errors())

    def test_specifications_require_existing_executable_hosts(self):
        path = self.write_specifications([self.specification()], relative="root/_materials/unhosted")
        self.assertIn("existing executable scene host", self.errors())
        path.unlink()
        self.node("root/runtime", surface="go-test", construct=False)
        self.write_specifications([self.specification()], relative="root/runtime")
        self.assertIn("existing executable scene host", self.errors())

    def test_specification_parse_errors_are_reported_by_product_check(self):
        path = self.write_specifications([self.specification()])
        path.write_text("- id: first\n  id: duplicate\n", encoding="utf-8")
        self.assertIn("duplicate key", self.errors())

    def test_product_selection_shows_specs_on_their_hosts_and_respects_boundaries(self):
        self.write_specifications([self.specification()])
        self.write_specifications([self.specification(case_id="root-read")], relative="root")
        model = self.model()
        view = tree.project_view(model, view_id="product/product/U3", from_id="attached")
        state, = view["states"]
        self.assertEqual(state["id"], "attached")
        self.assertTrue(state["context"])
        self.assertFalse(state["construct_selected"])
        self.assertEqual(state["processes"], [])
        self.assertEqual(view["product_claim"]["selected_link_count"], 0)
        self.assertEqual(view["product_claim"]["declared_specification_count"], 2)
        self.assertEqual(view["product_claim"]["selected_specification_count"], 1)
        self.assertEqual(view["product_claim"]["specifications"][0]["id"], "liveaccess-read")
        self.assertEqual(len(model["product"]["claims"][2]["specifications"]), 2)

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
        path = self.repo / "doc/design/product.md"
        path.write_text(path.read_text().replace("### U2", "```markdown\n### U999: Example\n```\n### U2"), encoding="utf-8")
        self.assertEqual(self.errors(), "")
        self.assertEqual(len(self.model()["product"]["claims"]), 3)

    def test_bold_u_items_in_an_ordered_list_are_stable_owner_claims(self):
        path = self.repo / "doc/design/product.md"
        path.write_text(
            "# Product\n1. **U9 Outside：**Not in the registered section.\n"
            "## 8. 用例\n1. **U1 字段：**Compare sets and returned values.\n"
            "```markdown\n2. **U99 Example：**A fenced example.\n```\n"
            "2. **U2 来源选用：**Select the source implementation.\n"
            "3. **U3 更新恢复：**Recover the selected source.\n"
            "## 9. Other\n1. **U8 Outside：**Not a registered claim.\n", encoding="utf-8")
        model = self.model()
        self.assertEqual(tree.check_product(model)["errors"], [])
        self.assertEqual([(item["id"], item["title"]) for item in model["product"]["claims"]],
                         [("U1", "U1：字段"), ("U2", "U2：来源选用"), ("U3", "U3：更新恢复")])
        self.assertEqual(model["product"]["claims"][0]["source"]["line"], 4)

    def test_ordered_list_claim_ids_share_the_heading_duplicate_guard(self):
        path = self.repo / "doc/design/product.md"
        for duplicate in ("2. **U1 第二项：**Duplicate ordered claim.", "### U1：Duplicate heading"):
            with self.subTest(duplicate=duplicate):
                path.write_text("# Product\n## 8. 用例\n1. **U1 字段：**First claim.\n" + duplicate + "\n", encoding="utf-8")
                self.assertIn("duplicate claim id U1", self.errors())


class LiveAccessLibrarySpecificationsTest(unittest.TestCase):
    def test_current_liveaccess_targets_keep_prepared_library_cases(self):
        root = Path(__file__).resolve().parents[1]
        claims = {claim["ref"]: claim for claim in tree.build_view(root)["product"]["claims"]}
        for number in range(13, 21):
            reference = f"liveaccess#U{number}"
            with self.subTest(reference=reference):
                self.assertIn(reference, claims)
                self.assertTrue(claims[reference]["specifications"],
                                f"{reference} must keep its prepared library specification, not only a gap")

    def test_current_liveaccess_targets_have_named_execution_assertions(self):
        root = Path(__file__).resolve().parents[1]
        claims = {claim["ref"]: claim for claim in tree.build_view(root)["product"]["claims"]}
        for number in range(13, 21):
            reference = f"liveaccess#U{number}"
            with self.subTest(reference=reference):
                self.assertTrue(any(link.get("tests") for link in claims[reference]["links"]),
                                f"{reference} needs executable assertions, not only prepared specifications")


if __name__ == "__main__":
    unittest.main()
