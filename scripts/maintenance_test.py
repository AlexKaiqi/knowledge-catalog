"""Script entry-point regressions using isolated fixtures, without services."""
import json
import os
import pathlib
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = pathlib.Path(__file__).resolve().parent


class MaintenanceEntryTest(unittest.TestCase):
    def isolated_testsuite(self, root, group, mode="declared", evidence=False):
        scripts, bins = root / "scripts", root / "bin"
        scripts.mkdir()
        bins.mkdir()
        shutil.copyfile(SCRIPTS / "testsuite.sh", scripts / "testsuite.sh")
        log = root / "commands.jsonl"
        go = bins / "go"
        go.write_text(f"#!{sys.executable}\n"
                      f"log_path = {str(log)!r}\n" + '''import json, os, sys
args = sys.argv[1:]
mode = os.environ["KC_MAINTENANCE_TEST_MODE"]
with open(log_path, "a") as log:
    log.write(json.dumps({"args": args, "live": os.getenv("KC_LIVE_TAIHU"),
                         "report": os.getenv("KC_COMMAND_COVERAGE_REPORT")}) + "\\n")
if "-list" in args:
    if mode == "list-failed":
        print("listing failed", file=sys.stderr)
        sys.exit(71)
    declared = mode != "empty" and args[-1] == "./server"
    print("TestLiveTaihuAuthentication" if declared else "TestLiveTaihuAuthenticationExtra")
    print("ok kc/server")
elif mode == "test-failed":
    print("test failed", file=sys.stderr)
    sys.exit(73)
elif args[-1] != "./server":
    print("ok kc/cli [no tests to run]")
else:
    print(json.dumps({"Action": "pass", "Package": "kc/server",
                      "Test": "TestLiveTaihuAuthentication"}))
''')
        go.chmod(0o755)
        for name in ("npm", "node", "docker"):
            stub = bins / name
            stub.write_text("#!/bin/sh\nexit 75\n")
            stub.chmod(0o755)
        env = dict(os.environ, GO=str(go), KC_VALIDATION_RUN_ID="isolated-fixture",
                   KC_VALIDATION_RUN_DIR=str(root / "evidence") if evidence else "",
                   KC_TEST_OPENSEARCH_URL="", KC_LIVE_TAIHU="0",
                   KC_MAINTENANCE_TEST_MODE=mode)
        env.pop("KC_COMMAND_COVERAGE_REPORT", None)
        env["PATH"] = str(bins) + os.pathsep + env.get("PATH", "")
        result = subprocess.run(["/bin/bash", str(scripts / "testsuite.sh"), group],
                                env=env, capture_output=True, text=True, timeout=10)
        return result, log

    def test_retired_gitea_entry_rejects_without_side_effects(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            result, log = self.isolated_testsuite(root, "gitea")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertFalse(log.exists(), "retired driver must not run a provider or install dependencies")

    def test_taihu_suite_dispatch_requires_declared_owner_and_preserves_failures(self):
        name, package = "TestLiveTaihuAuthentication", "./server"
        selection = f"^{name}$"
        for mode, evidence, status in (("declared", False, 0), ("declared", True, 0),
                                       ("empty", True, 1), ("list-failed", True, 71),
                                       ("test-failed", True, 73)):
            with self.subTest(mode=mode, evidence=evidence), tempfile.TemporaryDirectory() as temp:
                root = pathlib.Path(temp)
                result, log = self.isolated_testsuite(root, "taihu-live", mode, evidence)
                self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                calls = [json.loads(line) for line in log.read_text().splitlines()]
                self.assertEqual(calls[0]["args"], ["test", "-list", selection, package])
                self.assertTrue(all(call["live"] == "1" for call in calls),
                                "Taihu suite must explicitly activate its existing live gate")
                if mode in ("empty", "list-failed"):
                    self.assertEqual(len(calls), 1, "failed declaration lookup must prevent test execution")
                    self.assertIn("not declared" if mode == "empty" else "listing failed", result.stderr)
                    continue
                self.assertEqual(len(calls), 2)
                expected = ["test"] + (["-json"] if evidence else [])
                expected += ["-count=1", "-timeout=2m", "-run", selection, package]
                self.assertEqual(calls[1]["args"], expected)
                report = str(root / "evidence" / "command-coverage-1.json") if evidence else None
                self.assertEqual(calls[1]["report"], report)
                if mode == "test-failed":
                    self.assertIn("test failed", result.stderr)

    def test_native_daemon_acceptance_preserves_failures_and_cleanup(self):
        source = (SCRIPTS / "e2e-kcfs-linux.sh").read_text()
        block = source[source.index('daemon_project="$run_root/daemon-project"'):source.index('remote_project="$run_root/remote-project"')]
        functions = "\n".join(re.search(r"(?ms)^" + name + r"\(\) \{.*?^\}", source).group(0)
                              for name in ("cleanup", "assert_file_content"))
        for mode in (None, "bad-manifest", "writable", "wrong-write-error", "stop-failed", "leftover-log"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temp:
                root = pathlib.Path(temp)
                run_root, bins = root / "run", root / "bin"
                run_root.mkdir()
                bins.mkdir()
                log = root / "commands.jsonl"
                (root / "fixture.json").write_text(json.dumps({"mode": mode, "log": str(log),
                    "readOnlyTarget": str(run_root / "daemon-project/docs/team/README.md")}))
                kcfs = run_root / "kcfs"
                kcfs.write_text(f"#!{sys.executable}\n" + '''import json, os, pathlib, shutil, sys
fixture = json.loads((pathlib.Path(__file__).parent.parent / "fixture.json").read_text())
args = sys.argv[1:]
with open(fixture["log"], "a") as log:
    log.write(json.dumps(args) + "\\n")
project = pathlib.Path(__file__).parent / "daemon-project"
log_file = pathlib.Path(os.environ["TMPDIR"]) / "kcfs-daemon-424242.log"
mounts = ("docs/team", "docs/runbooks", "object/policy")
if args[0] == "daemon-mount":
    for name, value in (("docs/team/README.md", "advanced\\n"), ("object/policy/rules.md", "policy\\n"), ("docs/runbooks/incident.md", "incident\\n")):
        path = project / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value)
    log_file.write_text("daemon output")
    json.dump({"pid": 424242, "setId": "agent", "root": str(project), "readOnly": fixture["mode"] != "bad-manifest",
               "pinId": "fixed-pin", "mounts": [{"path": path} for path in mounts]}, sys.stdout)
elif args[0] == "stop":
    if fixture["mode"] == "stop-failed":
        sys.exit(73)
    for mount in mounts:
        shutil.rmtree(project / mount)
    if fixture["mode"] != "leftover-log":
        log_file.unlink()
else:
    sys.exit(72)
''')
                kcfs.chmod(0o755)
                python = bins / "python3"
                python.write_text(f"#!{sys.executable}\n" + '''import builtins, errno, json, pathlib, sys
fixture = json.loads((pathlib.Path(__file__).parent.parent / "fixture.json").read_text())
original_open = builtins.open
def boundary_open(path, mode="r", *args, **kwargs):
    if str(path) == fixture["readOnlyTarget"] and mode == "w" and fixture["mode"] != "writable":
        code = errno.EACCES if fixture["mode"] == "wrong-write-error" else errno.EROFS
        raise OSError(code, "fixture mount write boundary")
    return original_open(path, mode, *args, **kwargs)
builtins.open = boundary_open
sys.argv = sys.argv[1:]
exec(sys.stdin.read(), {"__name__": "__main__"})
''')
                python.chmod(0o755)
                shell = ("set -euo pipefail\nrun_root=" + shlex.quote(str(run_root)) +
                         '\nkc_pid=""\nremote_pid=""\nserver_pid=""\ndaemon_pid=""\n' +
                         'daemon_log_dir="$run_root/daemon-tmp"\nserver_url=http://server.invalid\n' +
                         functions + "\ntrap cleanup EXIT\n" + block)
                env = dict(os.environ, PATH=str(bins) + os.pathsep + os.environ.get("PATH", ""))
                result = subprocess.run(["/bin/bash", "-c", shell], env=env,
                                        capture_output=True, text=True, timeout=10)
                calls = [json.loads(line)[0] for line in log.read_text().splitlines()]
                self.assertEqual(calls[0], "daemon-mount")
                self.assertIn("stop", calls, "failed acceptance must still stop the daemon")
                self.assertFalse(run_root.exists(), "acceptance cleanup must remove its isolated run directory")
                if mode is None:
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn("PASS: native kcfs daemon lifecycle", result.stdout)
                    self.assertEqual(calls, ["daemon-mount", "stop"])
                else:
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertNotIn("PASS: native kcfs daemon lifecycle", result.stdout)
                    if mode == "stop-failed":
                        self.assertEqual(result.returncode, 73)
                        self.assertEqual(calls, ["daemon-mount", "stop", "stop"])
                    if mode == "leftover-log":
                        self.assertIn("kcfs stop left a daemon log behind", result.stderr)

    def test_kcfs_rejects_unsupported_platform_with_a_complete_binding(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            log = root / "commands.log"
            stub = root / "uname"
            stub.write_text(f'#!/bin/sh\nprintf "uname\\n" >> "{log}"\nprintf "Darwin\\n"\n')
            stub.chmod(0o755)
            env = dict(os.environ, KC_LAKEFS_URL="http://lakefs.invalid",
                       KC_LAKEFS_CREDENTIAL="fixture:fixture")
            env["PATH"] = str(root) + os.pathsep + env.get("PATH", "")
            result = subprocess.run(["/bin/bash", str(SCRIPTS / "e2e-kcfs-linux.sh")],
                                    env=env, capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            self.assertIn("FAIL: kcfs host mount smoke test requires Linux", result.stderr)
            self.assertEqual(log.read_text(), "uname\n")

    def test_kcfs_container_setup_stops_on_install_failure_without_plugin_dependencies(self):
        for failed_step in ("update", "install", None):
            with self.subTest(failed_step=failed_step), tempfile.TemporaryDirectory() as temp:
                root = pathlib.Path(temp)
                scripts = root / "scripts"
                bins = root / "bin"
                scripts.mkdir()
                bins.mkdir()
                for name in ("e2e-kcfs-docker.sh", "kcfs-env.sh"):
                    shutil.copyfile(SCRIPTS / name, scripts / name)
                log = root / "commands.log"
                captured = root / "docker-command.sh"
                stub = scripts / "e2e-kcfs-linux.sh"
                stub.write_text(f'#!/bin/sh\nprintf "linux\\n" >> "{log}"\n')
                stub.chmod(0o755)

                forbidden = ("npm", "node", "curl", "tar", "uname")
                for name in ("apt-get",) + forbidden:
                    lines = ["#!/bin/sh", f'printf "{name} %s\\n" "$*" >> "{log}"']
                    if name == "apt-get" and failed_step in ("update", "install"):
                        lines.append(f'[ "$1" != "{failed_step}" ] || exit 73')
                    if name in forbidden:
                        lines.append("exit 75")
                    lines.append("exit 0")
                    stub = bins / name
                    stub.write_text("\n".join(lines) + "\n")
                    stub.chmod(0o755)
                docker = bins / "docker"
                docker.write_text(f"#!{sys.executable}\n"
                                  "import os, pathlib, shlex, subprocess, sys\n"
                                  "assert sys.argv[-3:-1] == ['bash', '-c']\n"
                                  f"pathlib.Path({str(captured)!r}).write_text(sys.argv[-1])\n"
                                  # Preserve the captured production command; relocate only its
                                  # output sink before running the container setup on the host.
                                  "script = sys.argv[-1]\n"
                                  "assert script.count('>/tmp/kcfs-apt.log') == 1\n"
                                  f"script = script.replace('>/tmp/kcfs-apt.log', '>' + shlex.quote({str(root / 'kcfs-apt.log')!r}))\n"
                                  "env = dict(os.environ)\n"
                                  "args = sys.argv[1:-3]\n"
                                  "for index, arg in enumerate(args):\n"
                                  "    if arg == '-e':\n"
                                  "        key, separator, value = args[index + 1].partition('=')\n"
                                  "        if separator:\n"
                                  "            env[key] = value\n"
                                  f"sys.exit(subprocess.run(['/bin/bash', '-c', script], cwd={str(root)!r}, env=env).returncode)\n")
                docker.chmod(0o755)
                env = dict(os.environ, KC_LAKEFS_URL="http://lakefs.invalid",
                           KC_LAKEFS_CREDENTIAL="fixture:fixture")
                env["PATH"] = str(bins) + os.pathsep + env.get("PATH", "")
                result = subprocess.run(["/bin/bash", str(scripts / "e2e-kcfs-docker.sh")],
                                        env=env, capture_output=True, text=True, timeout=10)
                self.assertTrue(captured.exists(), result.stdout + result.stderr)
                self.assertEqual((root / "kcfs-apt.log").exists(), failed_step != "update",
                                 "container setup logs must stay inside the test fixture")
                invoked = log.read_text().splitlines()
                self.assertFalse(any(line.split()[0] in forbidden for line in invoked),
                                 "native FUSE setup must not build a plugin or download Node")
                if failed_step is None:
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertEqual(invoked.count("linux"), 1)
                else:
                    self.assertEqual(result.returncode, 73,
                                     result.stdout + result.stderr)
                    self.assertNotIn("linux", invoked)

    def test_surface_guard_rejects_retired_implementations_and_cli_examples(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            for directory in ("scripts", "cli", "client", "liveaccess", "object",
                              "snapshot/cli", "snapshot/client",
                              "snapshot/dataset", "snapshot/access", "snapshot/service", "snapshot/console", "doc", "server", "cmd", ".github"):
                (root / directory).mkdir(parents=True, exist_ok=True)
            for name in ("README.md", "cli/README.md", "snapshot/README.md", "snapshot/dataset/README.md", "snapshot/access/README.md", "snapshot/service/README.md", "snapshot/console/README.md",
                         "snapshot/cli/README.md", "snapshot/client/README.md",
                         "liveaccess/README.md", "Makefile", ".dockerignore"):
                (root / name).write_text("")
            (root / "snapshot/service/contract.go").write_text("package service\n")
            (root / "snapshot/service/file_http.go").write_text('mux.HandleFunc("POST /snapshot/v1/files/file:read", handler)\n')
            routes = root / "server/service_routes.go"
            routes.write_text("\n".join(f'"/{namespace}/v1/resources"'
                                        for namespace in ("snapshot", "object",
                                                          "writer", "identity",
                                                          "admin", "operations")) + "\n")
            script = root / "scripts/check-surface.sh"
            shutil.copyfile(SCRIPTS / script.name, script)
            command = ["/bin/bash", str(script)]
            prefixes = ("kc ", "go run ./cmd/kc -- ")
            current_commands = ("registry audit", "show", "attach --repo kr://acme/core",
                                "create --name core", "detach --repo kr://acme/core",
                                "create --name catalog", "read --object catalog/list")
            current_examples = "\n".join(prefix + path for prefix in prefixes for path in current_commands)
            current_examples += "\ncatalog show\ncatalog repo attach\ncatalog repo create\ncatalog repo archive\n"
            readme = root / "README.md"
            readme.write_text(current_examples)
            (root / "cli/HTTP_REFACTOR.md").write_text(
                "Historical dsh-plugin bridge: object_read / registerTool\n")
            baseline = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(baseline.returncode, 0, baseline.stdout + baseline.stderr)

            restored_sources = (("dsh-plugin", None, "retired DSH plugin"),
                                ("Makefile", "test-plugin:\n\tnpm --prefix dsh-plugin run build\n", "retired DSH plugin"),
                                ("cli/plugin_contract_test.go", "//go:build dsh_contract\n\npackage cli_test\n", "retired DSH plugin"),
                                ("cli/context.go", "package cli\nfunc inheritTaskContext() {}\n", "retired private task context"),
                                ("client/model.go", 'package client\nvar tool = "object_read"\n', "retired model tools"),
                                ("server/agent.go", 'package server\nfunc agent() { registerTool("read") }\n', "model tool registration"))
            for name, content, label in restored_sources:
                with self.subTest(restored=name):
                    restored = root / name
                    if content is None:
                        restored.mkdir()
                    else:
                        restored.write_text(content)
                    result = subprocess.run(command, capture_output=True, text=True)
                    if content is None:
                        restored.rmdir()
                    elif name == "Makefile":
                        restored.write_text("")
                    else:
                        restored.unlink()
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn(label, result.stdout + result.stderr)

            (root / "client").rmdir()
            result = subprocess.run(command, capture_output=True, text=True)
            (root / "client").mkdir()
            self.assertNotEqual(result.returncode, 0, "missing required scan paths must fail closed")

            for prefix in prefixes:
                for path in ("catalog show", "catalog repo attach", "catalog repo create", "catalog repo archive", "catalog repo detach", "catalog repo connect", "catalog repo share", "catalog", "catalog future-subcommand", "catalog list", "catalog use", "catalog audit", "catalog archive"):
                    with self.subTest(example=prefix + path):
                        readme.write_text(prefix + path + "\n")
                        result = subprocess.run(command, capture_output=True, text=True)
                        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                        self.assertIn("retired Catalog CLI examples", result.stdout + result.stderr)
                        self.assertIn("README.md", result.stdout + result.stderr)
            readme.write_text(current_examples)

            (root / "server/legacy.go").write_text(
                'package server\nfunc legacyRoute() { mux.HandleFunc("POST /v1/write", handler) }\n')
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("legacy verb HTTP implementation", result.stdout + result.stderr)
            self.assertIn("server/legacy.go", result.stdout + result.stderr)
            (root / "server/legacy.go").unlink()

    def test_kcfs_requires_lakefs_binding_before_running_commands(self):
        cases = ((script, supplied)
                 for script in ("e2e-kcfs-linux.sh", "e2e-kcfs-docker.sh")
                 for supplied in ({}, {"KC_LAKEFS_URL": "http://lakefs.invalid"},
                                  {"KC_LAKEFS_CREDENTIAL": "fixture:fixture"}))
        for script, supplied in cases:
            with self.subTest(script=script, supplied=tuple(supplied)), tempfile.TemporaryDirectory() as temp:
                root = pathlib.Path(temp)
                log = root / "commands.log"
                for name in ("dirname", "uname", "go", "mktemp", "curl", "docker", "npm", "apt-get"):
                    stub = root / name
                    stub.write_text('#!/bin/sh\nprintf "invoked\\n" >> "$KC_MAINTENANCE_COMMAND_LOG"\nexit 73\n')
                    stub.chmod(0o755)
                env = dict(os.environ)
                for name in ("KC_LAKEFS_URL", "KC_LAKEFS_CREDENTIAL"):
                    env.pop(name, None)
                env.update(supplied)
                env.update(PATH=str(root) + os.pathsep + env.get("PATH", ""),
                           KC_MAINTENANCE_COMMAND_LOG=str(log))
                result = subprocess.run(["/bin/bash", script], cwd=SCRIPTS,
                                        env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("FAIL:", result.stderr)
                self.assertIn("KC_LAKEFS_URL", result.stderr)
                self.assertIn("KC_LAKEFS_CREDENTIAL", result.stderr)
                self.assertIn("scripts/system-lakefs.sh up", result.stderr)
                self.assertNotIn("system-lakefs.sh local up", result.stderr)
                self.assertFalse(log.exists(), "missing binding must fail before invoking build or platform commands")


if __name__ == "__main__":
    unittest.main()
