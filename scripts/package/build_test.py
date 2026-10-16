"""Local packaging contracts without compilation, installation, or services."""

import contextlib
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import stat
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).with_name("build.py")
SPEC = importlib.util.spec_from_file_location("kc_package_build", SCRIPT)
BUILDER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BUILDER)
VERSION = "1.2.3"
REVISION = "0123456789abcdef" * 2 + "01234567"


class FakeTools:
    """Model the tool boundary while leaving staging and archiving real."""

    def __init__(self, fail_target=None, fail_tool="go"):
        self.calls = []
        self.fail_target = fail_target
        self.fail_tool = fail_tool

    def __call__(self, command, **kwargs):
        self.calls.append((command, kwargs))
        if command == ["git", "rev-parse", "HEAD"]:
            result = REVISION + "\n"
        elif command == ["git", "status", "--porcelain", "--untracked-files=normal"]:
            result = " M example.go\n"
        elif command == ["fixture-go", "version"]:
            result = "go version go1.23.6 fixture/host\n"
        elif command[:2] == ["fixture-go", "build"]:
            env = kwargs["env"]
            target = env["GOOS"] + "/" + env["GOARCH"]
            if target == self.fail_target and self.fail_tool == "go":
                raise subprocess.CalledProcessError(71, command)
            pathlib.Path(command[command.index("-o") + 1]).write_bytes(b"fake kc: " + target.encode())
            result = ""
        else:
            raise AssertionError(f"unexpected external command: {command!r}")
        return subprocess.CompletedProcess(command, 0, stdout=result)


class PackageBuildTest(unittest.TestCase):
    def invoke(self, root, arguments, tools=None, system="Darwin"):
        tools = tools or FakeTools()
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.object(BUILDER.platform, "system", return_value=system), \
                mock.patch.object(BUILDER.platform, "machine", return_value="arm64"), \
                mock.patch.object(BUILDER.subprocess, "run", side_effect=tools), \
                mock.patch.dict(os.environ, {"GO": "fixture-go", "GOAMD64": "v4", "GOARM64": "v9.5"}), \
                contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            status = BUILDER.main([*arguments, "--output", str(root)])
        return status, stdout.getvalue(), stderr.getvalue(), tools

    def test_all_four_targets_have_expected_packages_manifests_and_checksums(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary) / "packages with spaces"
            status, stdout, stderr, tools = self.invoke(root, ["package", "--all", "--version", VERSION])
            self.assertEqual(status, 0, stdout + stderr)
            expected = {f"kc_client_{target}.tar.gz" for target in
                ("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64")}
            self.assertEqual({path.name for path in root.iterdir()}, expected | {"SHA256SUMS"})
            checksums = dict(line.split("  ", 1)[::-1] for line in (root / "SHA256SUMS").read_text().splitlines())
            self.assertEqual(set(checksums), expected)
            for name, checksum in checksums.items():
                self.assertEqual(checksum, hashlib.sha256((root / name).read_bytes()).hexdigest())
            for target in ("darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"):
                with tarfile.open(root / f"kc_client_{target.replace('/', '_')}.tar.gz") as tar:
                    self.assertEqual(set(tar.getnames()), {"kc", "build-info.json", "install.sh", "uninstall.sh", "README.txt"})
                    install = tar.extractfile("install.sh").read().decode()
                    self.assertNotIn("@KC_", install)
                    self.assert_manifest(json.load(tar.extractfile("build-info.json")),
                                         tar.extractfile("kc").read(), target)
            builds = [call for call in tools.calls if call[0][:2] == ["fixture-go", "build"]]
            self.assertEqual({kwargs["env"]["GOOS"] + "/" + kwargs["env"]["GOARCH"]
                              for _, kwargs in builds},
                             {"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"})
            for command, kwargs in builds:
                self.assertEqual(command[:5], ["fixture-go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false"])
                self.assertEqual(command[-1], "./cmd/kc")
                self.assertEqual(kwargs["cwd"], BUILDER.ROOT)
                self.assertTrue(kwargs["check"])
                self.assertEqual({key: kwargs["env"][key] for key in ("CGO_ENABLED", "GOAMD64", "GOARM64")},
                                 {"CGO_ENABLED": "0", "GOAMD64": "v1", "GOARM64": "v8.0"})

    def assert_manifest(self, manifest, binary, target):
        self.assertEqual(manifest, {"version": VERSION, "revision": REVISION, "dirty": True,
                                   "goVersion": "go version go1.23.6 fixture/host", "target": target,
                                   "binarySHA256": hashlib.sha256(binary).hexdigest()})

    def test_archive_has_safe_paths_normalized_modes_and_matching_manifest(self):
        # 统一 tar.gz 后任意宿主都能交叉打包，无平台打包工具依赖。
        for target in ("linux/amd64", "darwin/arm64"):
            with tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                status, stdout, stderr, _ = self.invoke(root, ["package", "--target", target, "--version", VERSION],
                                                        system="Linux")
                self.assertEqual(status, 0, stdout + stderr)
                archive = root / ("kc_client_" + target.replace("/", "_") + ".tar.gz")
                self.assertTrue(archive.exists(), archive)
                with tarfile.open(archive) as tar:
                    self.assertEqual(set(tar.getnames()), {"kc", "build-info.json", "install.sh", "uninstall.sh", "README.txt"})
                    for entry in tar.getmembers():
                        path = pathlib.PurePosixPath(entry.name)
                        self.assertFalse(path.is_absolute())
                        self.assertNotIn("..", path.parts)
                        self.assertTrue(entry.isfile())
                        self.assertEqual((entry.uid, entry.gid, entry.mtime, entry.uname, entry.gname), (0, 0, 0, "root", "root"))
                        self.assertEqual(entry.mode, 0o755 if entry.name in ("kc", "install.sh", "uninstall.sh") else 0o644)
                    binary = tar.extractfile("kc").read()
                    self.assert_manifest(json.load(tar.extractfile("build-info.json")), binary, target)
                    install = tar.extractfile("install.sh").read().decode()
                    self.assertNotIn("@KC_", install)
                self.assertEqual(archive.read_bytes()[:10], b"\x1f\x8b\x08\x00\x00\x00\x00\x00\x02\xff")

    def test_binary_build_has_manifest_and_cross_builds_on_any_host(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            status, stdout, stderr, tools = self.invoke(root, ["build", "--target", "darwin/amd64", "--version", VERSION],
                                                        system="Linux")
            self.assertEqual(status, 0, stdout + stderr)
            target = root / "bin/darwin_amd64"
            self.assertEqual({path.name for path in target.iterdir()}, {"kc", "build-info.json"})
            self.assertEqual(stat.S_IMODE((target / "kc").stat().st_mode), 0o755)
            self.assert_manifest(json.loads((target / "build-info.json").read_text()), (target / "kc").read_bytes(), "darwin/amd64")
            self.assertFalse((root / "SHA256SUMS").exists())

    def test_archive_permissions_are_independent_of_restrictive_umask(self):
        with tempfile.TemporaryDirectory() as temporary:
            previous = os.umask(0o077)
            try:
                status, stdout, stderr, _ = self.invoke(pathlib.Path(temporary) / "output",
                    ["package", "--target", "darwin/arm64", "--version", VERSION], system="Linux")
            finally:
                os.umask(previous)
            self.assertEqual(status, 0, stdout + stderr)
            archive = pathlib.Path(temporary) / "output" / "kc_client_darwin_arm64.tar.gz"
            self.assertTrue(archive.exists(), archive)
            with tarfile.open(archive) as tar:
                for entry in tar.getmembers():
                    self.assertEqual(entry.mode, 0o755 if entry.name in ("kc", "install.sh", "uninstall.sh") else 0o644)

    def test_default_target_uses_physical_mac_cpu_and_linux_architecture_aliases(self):
        cases = (("Darwin", "arm64", None, "darwin/arm64"),
                 ("Darwin", "x86_64", "0\n", "darwin/amd64"),
                 ("Darwin", "x86_64", "1\n", "darwin/arm64"),
                 ("Darwin", "AMD64", OSError("sysctl unavailable"), "darwin/amd64"),
                 ("Linux", "x86_64", None, "linux/amd64"),
                 ("Linux", "amd64", None, "linux/amd64"),
                 ("Linux", "aarch64", None, "linux/arm64"),
                 ("Linux", "arm64", None, "linux/arm64"))
        for system, machine, probe, target in cases:
            with self.subTest(system=system, machine=machine, probe=probe), \
                    mock.patch.object(BUILDER.platform, "system", return_value=system), \
                    mock.patch.object(BUILDER.platform, "machine", return_value=machine), \
                    mock.patch.object(BUILDER.subprocess, "run") as command:
                if isinstance(probe, OSError):
                    command.side_effect = probe
                else:
                    command.return_value = subprocess.CompletedProcess([], 0, stdout=probe)
                args = BUILDER.arguments(["package"])
                self.assertEqual(args.targets, (target,))
                if probe is None:
                    command.assert_not_called()
                else:
                    command.assert_called_once_with(["/usr/sbin/sysctl", "-n", "hw.optional.arm64"],
                                                    capture_output=True, text=True, check=False)

    def test_invalid_inputs_fail_before_running_tools_or_creating_output(self):
        invalid = [["package", "--version", value] for value in ("v1.2.3", "1.2", "01.2.3", "1.2.3-beta", "../1.2.3")]
        invalid += [["package", "--target", "windows/amd64"], ["build", "--all"],
                    ["package", "--all", "--target", "linux/amd64"]]
        for arguments in invalid:
            with self.subTest(arguments=arguments), tempfile.TemporaryDirectory() as temporary:
                root, tools = pathlib.Path(temporary) / "not-created", FakeTools()
                with self.assertRaises(SystemExit) as error:
                    self.invoke(root, arguments, tools)
                self.assertEqual(error.exception.code, 2)
                self.assertFalse(root.exists())
                self.assertFalse(tools.calls)

    def test_failed_target_keeps_all_preexisting_artifacts_and_checksum_table(self):
        for target, tool in (("darwin/amd64", "go"), ("linux/amd64", "go")):
            with self.subTest(target=target, tool=tool), tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                previous = {f"kc_client_{name}.tar.gz": b"previous " + name.encode()
                            for name in ("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64")}
                previous["SHA256SUMS"] = b"previous complete checksum table\n"
                for name, value in previous.items():
                    (root / name).write_bytes(value)
                status, _, stderr, tools = self.invoke(root, ["package", "--all", "--version", VERSION], FakeTools(target, tool))
                self.assertEqual(status, 1)
                self.assertIn("kc build failed:", stderr)
                self.assertEqual({path.name: path.read_bytes() for path in root.iterdir()}, previous)
                self.assertTrue(tools.calls, "failure must happen after an earlier package was staged")

if __name__ == "__main__":
    unittest.main()
