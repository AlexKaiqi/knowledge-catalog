#!/usr/bin/env python3
"""Build local KC binaries and installers using Go and standard library tools."""

import argparse
import gzip
import hashlib
import json
import os
import pathlib
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile


ROOT = pathlib.Path(__file__).resolve().parents[2]
TEMPLATES = pathlib.Path(__file__).resolve().parent
TARGETS = ("darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64")
IDENTIFIER = "io.knowledge-catalog.kc"


def run(command, **kwargs):
    return subprocess.run(command, cwd=ROOT, check=True, **kwargs)


def output(command):
    return run(command, capture_output=True, text=True).stdout.strip()


def host_target():
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64",
            "amd64": "amd64"}.get(platform.machine().lower())
    if system == "darwin" and arch == "amd64":
        try:
            hardware = subprocess.run(["/usr/sbin/sysctl", "-n", "hw.optional.arm64"],
                                      capture_output=True, text=True, check=False)
            if hardware.stdout.strip() == "1":
                arch = "arm64"
        except OSError:
            pass  # Intel macOS versions need not expose the Apple Silicon probe.
    return f"{system}/{arch}"


def arguments(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "package"))
    targets = parser.add_mutually_exclusive_group()
    targets.add_argument("--target", choices=TARGETS)
    targets.add_argument("--all", action="store_true", help="package all four targets (macOS host required)")
    parser.add_argument("--version", default="0.0.0", help="three numeric components, e.g. 0.1.0")
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / "dist")
    args = parser.parse_args(argv)
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", args.version):
        parser.error("--version must contain three numeric components, e.g. 0.1.0")
    if args.all and args.action != "package":
        parser.error("--all is only supported for package")
    args.targets = TARGETS if args.all else (args.target or host_target(),)
    if any(target not in TARGETS for target in args.targets):
        parser.error("unsupported host; select --target explicitly")
    if args.action == "package" and any(t.startswith("darwin/") for t in args.targets):
        if platform.system() != "Darwin" or not shutil.which("pkgbuild"):
            parser.error("macOS .pkg creation requires a macOS host with pkgbuild; Linux packages can be cross-built")
    args.output = args.output.resolve()
    return args


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def build_info(version, go):
    # Ignore generated output and other ignored caches when describing the source.
    return {"version": version, "revision": output(["git", "rev-parse", "HEAD"]),
            "dirty": bool(output(["git", "status", "--porcelain", "--untracked-files=normal"])),
            "goVersion": output([go, "version"])}


def build_binary(directory, target, go, info):
    directory.mkdir(parents=True)
    system, arch = target.split("/")
    env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0")
    # Keep baseline ISA requirements independent of the build host's environment.
    env.update(GOAMD64="v1", GOARM64="v8.0")
    run([go, "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", str(directory / "kc"),
         "./cmd/kc"], env=env)
    (directory / "kc").chmod(0o755)
    manifest = dict(info, target=target, binarySHA256=sha256(directory / "kc"))
    metadata = directory / "build-info.json"
    metadata.write_text(json.dumps(manifest, indent=2) + "\n")
    metadata.chmod(0o644)


def template(name, target):
    system, arch = target.split("/")
    return (TEMPLATES / name).read_text().replace("@KC_OS@", system).replace("@KC_ARCH@", arch)


def linux_package(payload, archive, target):
    for name in ("install.sh", "uninstall.sh"):
        (payload / name).write_text(template(name, target))
        (payload / name).chmod(0o755)
    (payload / "README.txt").write_text(
        "KC client installer\n\n"
        "Install:   sudo sh ./install.sh\n"
        "Per-user:  PREFIX=\"$HOME/.local\" sh ./install.sh\n"
        "Uninstall: sudo sh /usr/local/share/kc/uninstall.sh\n"
        "Per-user:  PREFIX=\"$HOME/.local\" sh \"$HOME/.local/share/kc/uninstall.sh\"\n\n"
        "Add the selected prefix's bin directory to PATH. Run kc help, then kc login.\n"
        "No Go runtime is needed. Configuration and login sessions are preserved.\n"
        "build-info.json records the version, source revision and target.\n")
    # A fixed header and normalized ownership make identical payload archives reproducible.
    with archive.open("wb") as stream, gzip.GzipFile(filename="", mode="wb", fileobj=stream, mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode="w") as tar:
            for path in sorted(payload.iterdir()):
                entry = tar.gettarinfo(str(path), arcname=path.name)
                entry.uid = entry.gid = entry.mtime = 0
                entry.uname = entry.gname = "root"
                entry.mode = 0o755 if path.name in ("kc", "install.sh", "uninstall.sh") else 0o644
                with path.open("rb") as content:
                    tar.addfile(entry, content)


def macos_package(payload, package, target, version, stage):
    root = stage / "root"
    binary_dir = root / "usr/local/bin"
    metadata_dir = root / "usr/local/share/kc"
    binary_dir.mkdir(parents=True)
    metadata_dir.mkdir(parents=True)
    root.chmod(0o755)
    for directory in root.rglob("*"):
        directory.chmod(0o755)
    shutil.copy2(payload / "kc", binary_dir / "kc")
    shutil.copy2(payload / "build-info.json", metadata_dir / "build-info.json")
    (metadata_dir / "uninstall.sh").write_text(template("uninstall.sh", target))
    (metadata_dir / "uninstall.sh").chmod(0o755)
    scripts = stage / "scripts"
    scripts.mkdir()
    (scripts / "preinstall").write_text(template("preinstall.sh", target))
    (scripts / "preinstall").chmod(0o755)
    run(["pkgbuild", "--root", str(root), "--identifier", IDENTIFIER, "--version", version,
         "--install-location", "/", "--ownership", "recommended", "--scripts", str(scripts), str(package)])


def build(args):
    go = os.environ.get("GO", "go")
    info = build_info(args.version, go)
    args.output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".kc-build-", dir=args.output) as temporary:
        work = pathlib.Path(temporary)
        artifacts = []
        for target in args.targets:
            slug = target.replace("/", "_")
            stage = work / slug
            payload = stage / "payload"
            print(f"Building kc {args.version} for {target}", flush=True)
            build_binary(payload, target, go, info)
            if args.action == "build":
                destination = args.output / "bin" / slug
                destination.mkdir(parents=True, exist_ok=True)
                for name in ("kc", "build-info.json"):
                    os.replace(payload / name, destination / name)
                print(destination / "kc")
                continue
            suffix = ".pkg" if target.startswith("darwin/") else ".tar.gz"
            package = work / f"kc_client_{slug}{suffix}"
            if suffix == ".pkg":
                macos_package(payload, package, target, args.version, stage)
            else:
                linux_package(payload, package, target)
            artifacts.append(package)
        # Publish local files only after every requested target was built and packaged.
        if artifacts:
            checksums = work / "SHA256SUMS"
            checksums.write_text("".join(f"{sha256(path)}  {path.name}\n" for path in artifacts))
            for path in [*artifacts, checksums]:
                os.replace(path, args.output / path.name)
                print(args.output / path.name)


def main(argv=None):
    args = arguments(argv)
    try:
        build(args)
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"kc build failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
