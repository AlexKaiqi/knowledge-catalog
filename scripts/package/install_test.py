"""Installer contracts use a disposable prefix and never write to the host PATH."""

import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest


SCRIPTS = Path(__file__).resolve().parent


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="kc installer ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.package = self.root / "package contents"
        self.package.mkdir()
        self.prefix = self.root / "install prefix"
        self.cwd = self.root / "unrelated working directory"
        self.cwd.mkdir()
        self.home = self.root / "home"
        self.home.mkdir()
        self.fake_bin = self.root / "test commands"
        self.fake_bin.mkdir()
        self.write_command(
            "uname",
            'case "$1" in -s) printf "%s\\n" "$KC_TEST_UNAME_OS";; '
            '-m) printf "%s\\n" "$KC_TEST_UNAME_ARCH";; *) exit 2;; esac\n',
        )
        self.env = {
            **os.environ,
            "HOME": str(self.home),
            "PREFIX": str(self.prefix),
            "PATH": str(self.fake_bin) + os.pathsep + os.environ["PATH"],
            "KC_TEST_UNAME_OS": "Linux",
            "KC_TEST_UNAME_ARCH": "x86_64",
        }
        self.write_package("first")

    def write_command(self, name, body):
        path = self.fake_bin / name
        path.write_text("#!/bin/sh\n" + body)
        path.chmod(0o755)

    def write_package(self, version, target_os="linux", target_arch="amd64"):
        template = (SCRIPTS / "install.sh").read_text()
        (self.package / "install.sh").write_text(
            template.replace("@KC_OS@", target_os).replace("@KC_ARCH@", target_arch)
        )
        shutil.copyfile(SCRIPTS / "uninstall.sh", self.package / "uninstall.sh")
        (self.package / "kc").write_text(f"#!/bin/sh\nprintf '{version}\\n'\n")
        (self.package / "build-info.json").write_text(
            json.dumps({"version": version, "goos": target_os, "goarch": target_arch})
        )

    def run_script(self, script="install.sh", args=(), env=None, path=None):
        return subprocess.run(
            ["sh", str(path or self.package / script), *args],
            cwd=self.cwd,
            env=self.env | (env or {}),
            text=True,
            capture_output=True,
            timeout=30,
        )

    def assert_success(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def assert_failure(self, result, message):
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(message, result.stderr)

    def installed_files(self):
        return (
            self.prefix / "bin/kc",
            self.prefix / "share/kc/build-info.json",
            self.prefix / "share/kc/uninstall.sh",
        )

    def install(self):
        self.assert_success(self.run_script())

    def installed_content(self):
        return [path.read_bytes() for path in self.installed_files()]

    def test_install_upgrade_and_installed_uninstaller_preserve_user_files(self):
        session = self.home / ".config/kc/session.json"
        session.parent.mkdir(parents=True)
        session.write_text('{"token":"keep-local-session"}\n')
        profile = self.home / ".profile"
        profile.write_text("# existing shell configuration\n")
        unrelated = self.prefix / "share/kc/user-file"
        unrelated.parent.mkdir(parents=True)
        unrelated.write_text("keep package neighbour\n")
        original_session = session.read_bytes()
        original_profile = profile.read_bytes()

        self.install()
        binary, metadata, uninstaller = self.installed_files()
        self.assertEqual(stat.S_IMODE(binary.stat().st_mode), 0o755)
        self.assertEqual(stat.S_IMODE(metadata.stat().st_mode), 0o644)
        self.assertEqual(stat.S_IMODE(uninstaller.stat().st_mode), 0o755)
        self.assertEqual(json.loads(metadata.read_text())["version"], "first")
        self.assertEqual(subprocess.check_output([binary], text=True), "first\n")

        self.write_package("second")
        self.install()
        self.assertEqual(subprocess.check_output([binary], text=True), "second\n")
        self.assertEqual(json.loads(metadata.read_text())["version"], "second")
        self.assertEqual(sorted(p.name for p in binary.parent.iterdir()), ["kc"])
        self.assertEqual(
            sorted(p.name for p in metadata.parent.iterdir()),
            ["build-info.json", "uninstall.sh", "user-file"],
        )

        self.assert_success(self.run_script(path=uninstaller))
        for path in self.installed_files():
            self.assertFalse(path.exists(), path)
        self.assertTrue(binary.parent.is_dir())
        self.assertTrue(metadata.parent.is_dir())
        self.assertEqual(unrelated.read_text(), "keep package neighbour\n")
        self.assertEqual(session.read_bytes(), original_session)
        self.assertEqual(profile.read_bytes(), original_profile)
        self.assert_success(self.run_script("uninstall.sh"))

    def test_platform_and_architecture_must_match_before_any_write(self):
        self.install()
        before = self.installed_content()
        self.write_package("second")
        for key, value, expected in (
            ("KC_TEST_UNAME_OS", "Darwin", "this machine is darwin/amd64"),
            ("KC_TEST_UNAME_ARCH", "arm64", "this machine is linux/arm64"),
            ("KC_TEST_UNAME_OS", "Windows_NT", "unsupported operating system"),
            ("KC_TEST_UNAME_ARCH", "riscv64", "unsupported CPU architecture"),
        ):
            with self.subTest(key=key, value=value):
                self.assert_failure(self.run_script(env={key: value}), expected)
                self.assertEqual(self.installed_content(), before)

    def test_arm64_and_darwin_uname_aliases(self):
        for target_os, uname_os in (("linux", "Linux"), ("darwin", "Darwin")):
            for uname_arch in ("aarch64", "arm64"):
                with self.subTest(target_os=target_os, uname_arch=uname_arch):
                    self.write_package("arm", target_os, "arm64")
                    self.assert_success(
                        self.run_script(
                            env={"KC_TEST_UNAME_OS": uname_os, "KC_TEST_UNAME_ARCH": uname_arch}
                        )
                    )
        self.write_package("amd", "linux", "amd64")
        self.assert_success(self.run_script(env={"KC_TEST_UNAME_ARCH": "amd64"}))

    def test_invalid_arguments_and_prefix_never_install(self):
        for script in ("install.sh", "uninstall.sh"):
            for prefix in ("", "relative/path"):
                with self.subTest(script=script, prefix=prefix):
                    self.assert_failure(
                        self.run_script(script, env={"PREFIX": prefix}),
                        "PREFIX must be a nonempty absolute path",
                    )
            self.assert_failure(self.run_script(script, args=("--prefix",)), "unexpected arguments")
            self.assert_success(self.run_script(script, args=("--help",)))
        self.assertFalse(self.prefix.exists())

    def test_missing_payload_fails_before_installation(self):
        for name in ("kc", "build-info.json", "uninstall.sh"):
            with self.subTest(name=name):
                original = (self.package / name).read_bytes()
                (self.package / name).unlink()
                self.assert_failure(self.run_script(), "missing regular package file: " + name)
                self.assertFalse(self.prefix.exists())
                (self.package / name).write_bytes(original)

    def test_directory_and_symlink_targets_are_rejected_before_changes(self):
        self.install()
        for target in self.installed_files():
            original = target.read_bytes()
            for kind in ("directory", "symlink", "dangling symlink"):
                with self.subTest(target=target.name, kind=kind):
                    target.unlink()
                    if kind == "directory":
                        target.mkdir()
                        message = "refusing non-regular destination"
                    else:
                        referent = self.root / "symlink referent"
                        if kind == "symlink":
                            referent.write_text("do not touch\n")
                        else:
                            referent.unlink()
                        target.symlink_to(referent)
                        message = "refusing symbolic link"
                    for script in ("install.sh", "uninstall.sh"):
                        self.assert_failure(self.run_script(script), message)
                    for other in self.installed_files():
                        if other != target:
                            self.assertTrue(other.is_file(), other)
                    if kind == "directory":
                        target.rmdir()
                    else:
                        self.assertTrue(target.is_symlink())
                        if kind == "symlink":
                            self.assertEqual(referent.read_text(), "do not touch\n")
                        target.unlink()
                    target.write_bytes(original)

    def test_staging_failure_keeps_previous_installation_and_cleans_temporary_files(self):
        self.install()
        before = self.installed_content()
        self.write_package("second")
        self.write_command(
            "cp",
            'case "$1" in */build-info.json) printf "permission denied\\n" >&2; exit 1;; esac\n'
            + 'exec "' + shutil.which("cp") + '" "$@"\n',
        )
        self.assert_failure(self.run_script(), "cannot stage metadata")
        self.assertEqual(self.installed_content(), before)
        for parent in (self.prefix / "bin", self.prefix / "share/kc"):
            self.assertEqual(list(parent.glob(".*")), [])

    def test_directory_creation_failure_explains_permissions(self):
        self.write_command("mkdir", 'printf "permission denied\\n" >&2\nexit 1\n')
        self.assert_failure(self.run_script(), "check ownership and permissions")
        self.assertFalse(self.prefix.exists())

    def test_uninstall_failure_explains_permissions(self):
        self.install()
        before = self.installed_content()
        self.write_command("rm", 'printf "permission denied\\n" >&2\nexit 1\n')
        self.assert_failure(self.run_script("uninstall.sh"), "check ownership and permissions")
        self.assertEqual(self.installed_content(), before)


if __name__ == "__main__":
    unittest.main()
