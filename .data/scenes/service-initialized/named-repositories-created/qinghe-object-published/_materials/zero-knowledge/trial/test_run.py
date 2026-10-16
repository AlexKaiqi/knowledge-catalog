"""Credential handling and incomplete-run evidence must fail closed."""
import importlib.util
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("trial_run", Path(__file__).with_name("run.py"))
run = importlib.util.module_from_spec(spec)
spec.loader.exec_module(run)


class MaterialInputTest(unittest.TestCase):
    def test_supplied_directory_keeps_relative_paths_and_records_only_hashes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, destination = root / "source", root / "input"
            (source / "queries").mkdir(parents=True)
            (source / "empty").mkdir()
            files = {"schema.sql": b"CREATE TABLE orders (id INT);\n",
                     "queries/gmv.sql": b"SELECT SUM(amount) FROM orders;\n"}
            for name, content in files.items():
                (source / name).write_bytes(content)
            manifest = run.stage_materials("supplier", destination, source)
            self.assertEqual(manifest, [
                {"path": name, "sha256": hashlib.sha256(files[name]).hexdigest()}
                for name in sorted(files)
            ])
            self.assertTrue((destination / "empty").is_dir())
            for name, content in files.items():
                self.assertEqual((destination / name).read_bytes(), content)
            self.assertNotIn(str(source), json.dumps(manifest))
            self.assertNotIn("CREATE TABLE", json.dumps(manifest))
            (source / "schema.sql").write_text("changed after staging")
            self.assertEqual((destination / "schema.sql").read_bytes(), files["schema.sql"])

    def test_default_material_and_revision_behavior_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "supplier").mkdir()
            for case, version in (("supplier", "v1"), ("revision", "v2")):
                content = (version + " original material").encode()
                (root / "supplier" / ("gmv-caliber." + version + ".md")).write_bytes(content)
                with patch.object(run, "HERE", root / "trial"):
                    manifest = run.stage_materials(case, root / case / "input")
                self.assertEqual(manifest, [{"path": "gmv-caliber.md",
                                            "sha256": hashlib.sha256(content).hexdigest()}])
            self.assertEqual(run.stage_materials("colleague", root / "consumer"), [])

    def test_custom_materials_are_rejected_for_roles_without_business_input(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for case in ("env", "colleague", "history"):
                with self.subTest(case=case), self.assertRaisesRegex(ValueError, "supplier or revision"):
                    run.stage_materials(case, root / "input", root)

    def test_missing_directory_and_regular_file_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "file").write_text("not a directory")
            for source in (root / "missing", root / "file"):
                with self.subTest(source=source.name), self.assertRaisesRegex(ValueError, "directory"):
                    run.stage_materials("supplier", root / "input", source)

    def test_symbolic_links_are_rejected_before_any_material_is_copied(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            outside = root / "outside"
            outside.mkdir()
            (outside / "private").write_text("must not enter agent input")
            for kind in ("file", "directory", "dangling", "root"):
                with self.subTest(kind=kind):
                    source, destination = root / kind, root / (kind + "-input")
                    if kind == "root":
                        source.symlink_to(outside, target_is_directory=True)
                    else:
                        source.mkdir()
                        (source / "a-safe.txt").write_text("safe")
                        target = outside if kind == "directory" else outside / (
                            "private" if kind == "file" else "missing")
                        (source / "z-link").symlink_to(target, target_is_directory=kind == "directory")
                    with self.assertRaisesRegex(ValueError, "symbolic link"):
                        run.stage_materials("supplier", destination, source)
                    self.assertFalse(destination.exists())

    def test_cli_records_the_materials_actually_given_to_the_agent(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "raw"
            source.mkdir()
            content = b"supplier raw input\n"
            (source / "schema.sql").write_bytes(content)

            def docker_result(arguments, **kwargs):
                if "--entrypoint" in arguments:
                    return run.subprocess.CompletedProcess(arguments, 0, stdout='{"principal":"zk-supplier"}')
                if "agent" in arguments:
                    events = [
                        {"type": "message_end", "message": {"role": "assistant", "content": [
                            {"type": "text", "text": "Fixture agent finished"}]}},
                        {"type": "agent_end"}, {"type": "agent_settled", "aborted": False},
                    ]
                    kwargs["stdout"].write("\n".join(map(json.dumps, events)))
                return run.subprocess.CompletedProcess(arguments, 0)

            environment = {"KC_ZK_RUNS": str(root / "runs"), "ZK_MODEL_API_KEY": "test-only-credential"}
            with patch.dict(os.environ, environment, clear=True), \
                    patch.object(run.sys, "argv", ["run.py", "supplier", "--materials", str(source)]), \
                    patch.object(run.sys, "stdout", io.StringIO()), \
                    patch.object(run, "command", return_value=run.subprocess.CompletedProcess([], 0, stdout="image-id")), \
                    patch.object(run.subprocess, "run", side_effect=docker_result):
                self.assertEqual(run.main(), 0)
            records = list((root / "runs").glob("*/run.json"))
            self.assertEqual(len(records), 1)
            metadata = json.loads(records[0].read_text())
            self.assertEqual(metadata["materials"], [
                {"path": "schema.sql", "sha256": hashlib.sha256(content).hexdigest()}
            ])
            self.assertEqual((records[0].parent / "input/schema.sql").read_bytes(), content)
            self.assertNotIn(environment["ZK_MODEL_API_KEY"], records[0].read_text())
            self.assertNotIn(content.decode().strip(), records[0].read_text())


class RunEvidenceTest(unittest.TestCase):
    def test_user_env_is_parsed_without_executing_shell(self):
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {}, clear=True):
            root = Path(directory)
            with patch.object(Path, "home", return_value=root):
                (root / ".env").write_text('OTHER=ignored\nexport VOLCENGINE_API_KEY="test-key" # comment\n')
                self.assertEqual(run.model_key(), "test-key")
                marker = root / "executed"
                (root / ".env").write_text('VOLCENGINE_API_KEY="$(touch ' + str(marker) + ')"\n')
                self.assertEqual(run.model_key(), "$(touch " + str(marker) + ")")
                self.assertFalse(marker.exists())

    def test_provider_error_is_not_a_successful_answer(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            events = [
                {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Working"}]}},
                {"type": "message_end", "message": {"role": "assistant", "content": [], "stopReason": "error", "errorMessage": "provider failed"}},
                {"type": "agent_end"},
                {"type": "agent_settled", "aborted": False},
            ]
            path.write_text("\n".join(map(json.dumps, events)))
            answer, errors = run.last_answer(path)
            self.assertEqual(answer, "Working")
            self.assertEqual(errors, ["provider failed"])

    def test_truncated_event_stream_is_not_complete(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            path.write_text(json.dumps({"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Done"}]}}))
            self.assertEqual(run.last_answer(path), ("Done", ["missing agent_end event", "missing final agent_settled event"]))
            with path.open("a") as output:
                output.write('\n{"type":"agent_end"}\n')
            self.assertEqual(run.last_answer(path), ("Done", ["missing final agent_settled event"]))
            with path.open("a") as output:
                output.write('{"type":"agent_settled","aborted":false}\n')
            self.assertEqual(run.last_answer(path), ("Done", []))

    def parse_events(self, events):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            path.write_text("\n".join(map(json.dumps, events)))
            return run.last_answer(path)

    def test_settled_abort_is_not_normal_completion(self):
        answer, errors = self.parse_events([
            {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Working"}]}},
            {"type": "agent_end"},
            {"type": "agent_settled", "aborted": True},
        ])
        self.assertEqual(answer, "Working")
        self.assertEqual(errors, ["agent run was aborted"])

    def test_settled_without_explicit_outcome_is_not_normal_completion(self):
        answer, errors = self.parse_events([
            {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Working"}]}},
            {"type": "agent_end"},
            {"type": "agent_settled"},
        ])
        self.assertEqual(answer, "Working")
        self.assertEqual(errors, ["agent_settled lacks an explicit non-aborted outcome"])

    def test_previous_settled_turn_does_not_hide_incomplete_later_work(self):
        answer, errors = self.parse_events([
            {"type": "agent_start"},
            {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "First answer"}]}},
            {"type": "agent_end"},
            {"type": "agent_settled", "aborted": False},
            {"type": "agent_start"},
            {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Still working"}]}},
            {"type": "agent_end"},
        ])
        self.assertEqual(answer, "Still working")
        self.assertEqual(errors, ["missing final agent_settled event"])

    def test_aborted_assistant_is_not_hidden_by_non_aborted_settled_event(self):
        answer, errors = self.parse_events([
            {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "Partial"}], "stopReason": "aborted"}},
            {"type": "agent_end"},
            {"type": "agent_settled", "aborted": False},
        ])
        self.assertEqual(answer, "Partial")
        self.assertEqual(errors, ["aborted"])

    def test_malformed_tail_cannot_reuse_a_previous_settled_event(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            path.write_text('{"type":"agent_end"}\n'
                            '{"type":"agent_settled","aborted":false}\n'
                            '{"type":"agent_start"')
            self.assertEqual(run.last_answer(path), ("", ["malformed JSON event", "missing final agent_settled event"]))

    def test_jsonl_uses_lf_not_unicode_line_separators(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            event = {"type": "message_end", "message": {"role": "assistant", "content": [{"type": "text", "text": "one\u2028two"}]}}
            path.write_text(json.dumps(event, ensure_ascii=False) + '\n{"type":"agent_end"}\n'
                            '{"type":"agent_settled","aborted":false}\n')
            self.assertEqual(run.last_answer(path), ("one\u2028two", []))


if __name__ == "__main__":
    unittest.main()
