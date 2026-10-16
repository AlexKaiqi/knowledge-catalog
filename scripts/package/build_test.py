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
        self.packages = []
        self.package_directories = []
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
        elif command[0] == "pkgbuild":
            root = pathlib.Path(command[command.index("--root") + 1])
            metadata = root / "usr/local/share/kc/build-info.json"
            manifest = json.loads(metadata.read_text())
            if manifest["target"] == self.fail_target and self.fail_tool == "pkgbuild":
                raise subprocess.CalledProcessError(73, command)
            files = {str(path.relative_to(root)): (path.read_bytes(), stat.S_IMODE(path.stat().st_mode))
                     for path in root.rglob("*") if path.is_file()}
            self.package_directories.append({".": stat.S_IMODE(root.stat().st_mode)} |
                                            {str(path.relative_to(root)): stat.S_IMODE(path.stat().st_mode)
                                             for path in root.rglob("*") if path.is_dir()})
            scripts = pathlib.Path(command[command.index("--scripts") + 1])
            self.packages.append((command, manifest, files, (scripts / "preinstall").read_text(),
                                  stat.S_IMODE((scripts / "preinstall").stat().st_mode)))
            pathlib.Path(command[-1]).write_bytes(b"fake pkg: " + manifest["target"].encode())
            result = ""
        else:
            raise AssertionError(f"unexpected external command: {command!r}")
        return subprocess.CompletedProcess(command, 0, stdout=result)


class PackageBuildTest(unittest.TestCase):
    def invoke(self, root, arguments, tools=None, system="Darwin", pkgbuild="/fixture/pkgbuild"):
        tools = tools or FakeTools()
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.object(BUILDER.platform, "system", return_value=system), \
                mock.patch.object(BUILDER.platform, "machine", return_value="arm64"), \
                mock.patch.object(BUILDER.shutil, "which", return_value=pkgbuild), \
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
            expected = {f"kc_{VERSION}_{target}{suffix}" for target, suffix in (
                ("darwin_arm64", ".pkg"), ("darwin_amd64", ".pkg"),
                ("linux_arm64", ".tar.gz"), ("linux_amd64", ".tar.gz"))}
            self.assertEqual({path.name for path in root.iterdir()}, expected | {"SHA256SUMS"})
            checksums = dict(line.split("  ", 1)[::-1] for line in (root / "SHA256SUMS").read_text().splitlines())
            self.assertEqual(set(checksums), expected)
            for name, checksum in checksums.items():
                self.assertEqual(checksum, hashlib.sha256((root / name).read_bytes()).hexdigest())
            self.assertEqual(len(tools.packages), 2)
            for command, manifest, files, preinstall, script_mode in tools.packages:
                self.assertEqual(command[command.index("--identifier") + 1], "io.knowledge-catalog.kc")
                self.assertEqual(command[command.index("--version") + 1], VERSION)
                self.assertEqual(command[command.index("--install-location") + 1], "/")
                self.assertEqual(command[command.index("--ownership") + 1], "recommended")
                self.assertEqual(set(files), {"usr/local/bin/kc", "usr/local/share/kc/build-info.json",
                                             "usr/local/share/kc/uninstall.sh"})
                self.assertEqual(files["usr/local/bin/kc"][1], 0o755)
                self.assertEqual(files["usr/local/share/kc/build-info.json"][1], 0o644)
                self.assertEqual(files["usr/local/share/kc/uninstall.sh"][1], 0o755)
                self.assertEqual(script_mode, 0o755)
                self.assertNotIn("@KC_", preinstall)
                self.assert_manifest(manifest, files["usr/local/bin/kc"][0], manifest["target"])
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

    def test_linux_archive_has_safe_paths_normalized_modes_and_matching_manifest(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            status, stdout, stderr, _ = self.invoke(root, ["package", "--target", "linux/amd64", "--version", VERSION],
                                                    system="Linux", pkgbuild=None)
            self.assertEqual(status, 0, stdout + stderr)
            archive = root / f"kc_{VERSION}_linux_amd64.tar.gz"
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
                self.assert_manifest(json.load(tar.extractfile("build-info.json")), binary, "linux/amd64")
                install = tar.extractfile("install.sh").read().decode()
                self.assertNotIn("@KC_", install)
                self.assertIn("linux", install)
                self.assertIn("amd64", install)
            self.assertEqual(archive.read_bytes()[:10], b"\x1f\x8b\x08\x00\x00\x00\x00\x00\x02\xff")

    def test_binary_build_has_manifest_and_can_cross_build_without_pkgbuild(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            status, stdout, stderr, tools = self.invoke(root, ["build", "--target", "darwin/amd64", "--version", VERSION],
                                                        system="Linux", pkgbuild=None)
            self.assertEqual(status, 0, stdout + stderr)
            target = root / "bin/darwin_amd64"
            self.assertEqual({path.name for path in target.iterdir()}, {"kc", "build-info.json"})
            self.assertEqual(stat.S_IMODE((target / "kc").stat().st_mode), 0o755)
            self.assert_manifest(json.loads((target / "build-info.json").read_text()), (target / "kc").read_bytes(), "darwin/amd64")
            self.assertFalse(tools.packages)
            self.assertFalse((root / "SHA256SUMS").exists())

    def test_macos_staging_permissions_are_independent_of_restrictive_umask(self):
        with tempfile.TemporaryDirectory() as temporary:
            previous = os.umask(0o077)
            try:
                status, stdout, stderr, tools = self.invoke(pathlib.Path(temporary) / "output",
                    ["package", "--target", "darwin/arm64", "--version", VERSION])
            finally:
                os.umask(previous)
            self.assertEqual(status, 0, stdout + stderr)
            self.assertEqual(tools.package_directories, [{name: 0o755 for name in
                (".", "usr", "usr/local", "usr/local/bin", "usr/local/share", "usr/local/share/kc")}])
            _, _, files, _, script_mode = tools.packages[0]
            self.assertEqual({name: mode for name, (_, mode) in files.items()},
                {"usr/local/bin/kc": 0o755, "usr/local/share/kc/build-info.json": 0o644,
                 "usr/local/share/kc/uninstall.sh": 0o755})
            self.assertEqual(script_mode, 0o755)

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
                    mock.patch.object(BUILDER.shutil, "which", return_value="/fixture/pkgbuild"), \
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

    def test_macos_packages_require_macos_host_and_pkgbuild(self):
        for system, tool, selection in (("Linux", "/fixture/pkgbuild", ["--target", "darwin/arm64"]),
                                        ("Linux", None, ["--all"]), ("Darwin", None, ["--all"])):
            with self.subTest(system=system, tool=tool, selection=selection), tempfile.TemporaryDirectory() as temporary:
                root, tools = pathlib.Path(temporary) / "not-created", FakeTools()
                stderr = io.StringIO()
                with contextlib.redirect_stderr(stderr), mock.patch.object(BUILDER.platform, "system", return_value=system), \
                        mock.patch.object(BUILDER.shutil, "which", return_value=tool), \
                        mock.patch.object(BUILDER.subprocess, "run", side_effect=tools), self.assertRaises(SystemExit) as error:
                    BUILDER.main(["package", *selection, "--output", str(root)])
                self.assertEqual(error.exception.code, 2)
                self.assertIn("macOS .pkg creation requires a macOS host with pkgbuild", stderr.getvalue())
                self.assertFalse(root.exists())
                self.assertFalse(tools.calls)

    def test_failed_target_keeps_all_preexisting_artifacts_and_checksum_table(self):
        for target, tool in (("darwin/amd64", "pkgbuild"), ("linux/amd64", "go")):
            with self.subTest(target=target, tool=tool), tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                previous = {f"kc_{VERSION}_{name}{suffix}": b"previous " + name.encode()
                            for name, suffix in (("darwin_arm64", ".pkg"), ("darwin_amd64", ".pkg"),
                                                 ("linux_arm64", ".tar.gz"), ("linux_amd64", ".tar.gz"))}
                previous["SHA256SUMS"] = b"previous complete checksum table\n"
                for name, value in previous.items():
                    (root / name).write_bytes(value)
                status, _, stderr, tools = self.invoke(root, ["package", "--all", "--version", VERSION], FakeTools(target, tool))
                self.assertEqual(status, 1)
                self.assertIn("kc build failed:", stderr)
                self.assertEqual({path.name: path.read_bytes() for path in root.iterdir()}, previous)
                self.assertTrue(tools.packages, "failure must happen after an earlier package was staged")

    def test_preinstall_rejects_wrong_architecture_and_noncurrent_volume(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            uname = root / "uname"
            uname.write_text('#!/bin/sh\ncase "$1" in -s) echo Darwin;; -m) echo arm64;; *) exit 1;; esac\n')
            uname.chmod(0o755)
            script = root / "preinstall"
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ.get("PATH", ""))
            for target, volume, status, message in (("darwin/amd64", "/", 1, "matching CPU"),
                                                     ("darwin/arm64", "/Volumes/other", 1, "current system volume"),
                                                     ("darwin/arm64", "/", 0, "")):
                with self.subTest(target=target, volume=volume):
                    script.write_text(BUILDER.template("preinstall.sh", target))
                    result = subprocess.run(["/bin/sh", str(script), "fixture.pkg", "/", volume],
                                            env=env, capture_output=True, text=True, timeout=30)
                    self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                    if message:
                        self.assertIn(message, result.stderr)


if __name__ == "__main__":
    unittest.main()
