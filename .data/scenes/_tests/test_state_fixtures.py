"""A scene directory represents a reused fixture, not a verification result."""
import importlib.util
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("fixture_tree", Path(__file__).resolve().parents[1] / "tree.py")
tree = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tree)


def state(name, *, parent=None, probes=(), executable=True, fixture="A persisted shared fixture"):
    return {"id": name, "fixture": fixture, "executable": executable,
            "scene_construct": executable, "depends_on": [parent] if parent else [],
            "processes": [{"id": name + "::probe:" + probe, "surface": "feature"} for probe in probes]}


class StateFixturesTest(unittest.TestCase):
    def check(self, *states):
        return tree.check_states({"states": list(states)})

    def test_independent_go_tests_do_not_create_or_consume_a_fixture(self):
        node = state("schema-browsed", executable=False)
        node["processes"] = [{"id": "browse", "surface": "go-test", "evidence": ["TestBrowse", "TestPage"]}]
        result = self.check(node)
        self.assertTrue(any("construct" in error for error in result["errors"]))
        self.assertEqual(result["fixtures"][0]["probe_consumers"], [])

    def test_one_off_transition_belongs_inside_its_only_probe(self):
        result = self.check(state("archived", probes=["deny-write"]))
        self.assertTrue(any("archived" in error and "reuse" in error for error in result["errors"]))

    def test_linear_prerequisites_are_reused_by_distinct_downstream_probes(self):
        result = self.check(state("root"), state("prepared", parent="root"),
                            state("authorized", parent="prepared", probes=["read", "write-denied"]))
        self.assertEqual(result["errors"], [])
        self.assertEqual(len(result["fixtures"][0]["probe_consumers"]), 2)

    def test_go_evidence_cannot_rescue_a_one_probe_state(self):
        node = state("archived", probes=["read"])
        node["processes"].append({"id": "oracle", "surface": "go-test"})
        self.assertTrue(self.check(node)["errors"])

    def test_empty_terminal_is_not_a_state_just_because_it_was_named(self):
        result = self.check(state("ready"))
        self.assertTrue(result["errors"])

    def test_construct_needs_an_explicit_fixture_postcondition(self):
        result = self.check(state("prepared", probes=["a", "b"], fixture=""))
        self.assertTrue(any("fixture" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
