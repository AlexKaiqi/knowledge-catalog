#!/usr/bin/env python3
"""Prepare and verify the run-scoped DSH profile used by Compose."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
from pathlib import Path


EXPECTED_BUNDLES = [
    "@deepseek-ai/dsh-base",
    "@deepseek-ai/dsh-web-app",
    "dsh-multi-model-provider",
    "dsh-loom",
]


def fail(message: str) -> None:
    raise SystemExit(f"dsh profile verification failed: {message}")


def replace_once(path: Path, old: str, new: str) -> None:
    content = path.read_text(encoding="utf-8")
    count = content.count(old)
    if count != 1:
        fail(f"{path} expected one compatibility target, found {count}")
    path.write_text(content.replace(old, new), encoding="utf-8")


def prepare(profile_dir: Path, source: Path, multi_version: str, lock_sha256: str) -> None:
    lock_path = profile_dir / "pnpm-lock.yaml"
    actual_lock_sha256 = hashlib.sha256(lock_path.read_bytes()).hexdigest()
    if actual_lock_sha256 != lock_sha256:
        fail(f"profile dependency lock drifted: {actual_lock_sha256}")
    manifest_path = profile_dir / "package.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    bundles = manifest.get("dsh", {}).get("profile", {}).get("bundles")
    if bundles != EXPECTED_BUNDLES:
        fail(f"unexpected bundle order: {bundles!r}")

    installed = profile_dir / "node_modules" / "dsh-loom"
    source_manifest = json.loads((source / "package.json").read_text(encoding="utf-8"))
    installed_manifest = json.loads((installed / "package.json").read_text(encoding="utf-8"))
    if installed_manifest.get("version") != source_manifest.get("version"):
        fail("installed dsh-loom version differs from image source")
    for relative in ("skills/knowledge-catalog/SKILL.md", "dist/index.js"):
        if (installed / relative).read_bytes() != (source / relative).read_bytes():
            fail(f"installed dsh-loom {relative} differs from image source")

    multi = profile_dir / "node_modules" / "dsh-multi-model-provider"
    multi_manifest_path = multi / "package.json"
    multi_manifest = json.loads(multi_manifest_path.read_text(encoding="utf-8"))
    if multi_manifest.get("version") != multi_version:
        fail(f"unexpected dsh-multi-model-provider version {multi_manifest.get('version')!r}")
    client = multi_manifest.get("dsh", {}).get("client")
    required_injects = {
        "@deepseek-ai/dsh-client-locale",
        "@deepseek-ai/dsh-client-runtime",
        "@deepseek-ai/dsh-client-ui-settings",
        "@deepseek-ai/dsh-api-remotes",
    }
    if not isinstance(client, dict) or not required_injects.issubset(set(client.get("inject", []))):
        fail("multi-model browser manifest is missing its required DSH client packages")

    # rc.19 targets the pre-rc.2 remote.settings service. Patch both the shipped
    # source and compiled browser entry so the optional model-portrait surface
    # uses the same connection.api settings wire as DSH rc.2's stock UI.
    source_client = multi / "src" / "client" / "index.jsx"
    replace_once(source_client, "api.settings.describe().then(responseValue)", "api.settings.describe({}).then(responseValue)")
    replace_once(
        source_client,
        "export const inject = ['slots', 'locale', 'sessions', 'remote', 'remote.settings']",
        "export const inject = ['slots', 'locale', 'sessions', 'connection']",
    )
    replace_once(source_client, "inject: () => ({ api: ctx.remote, sessions }),", "inject: () => ({ api: ctx.get('connection').api, sessions }),")

    compiled_client = multi / "lib" / "client.js"
    replace_once(compiled_client, "api.settings.describe().then(responseValue)", "api.settings.describe({}).then(responseValue)")
    replace_once(
        compiled_client,
        '\t\tconst inject = [\n\t\t\t"slots",\n\t\t\t"locale",\n\t\t\t"sessions",\n\t\t\t"remote",\n\t\t\t"remote.settings"\n\t\t];',
        '\t\tconst inject = [\n\t\t\t"slots",\n\t\t\t"locale",\n\t\t\t"sessions",\n\t\t\t"connection"\n\t\t];',
    )
    replace_once(compiled_client, "api: ctx.remote,\n\t\t\t\t\tsessions", "api: ctx.get(\"connection\").api,\n\t\t\t\t\tsessions")


def entry_block(config: str, entry_id: str) -> str:
    match = re.search(
        rf"(?ms)^- id: {re.escape(entry_id)}\n(?P<body>.*?)(?=^# == |^- id: |\Z)",
        config,
    )
    if match is None:
        fail(f"composed config is missing {entry_id}")
    return match.group(0)


def verify_config(config_path: Path) -> None:
    config = config_path.read_text(encoding="utf-8")
    required_entries = {
        "settings": "@deepseek-ai/dsh-settings-file",
        "llm-pi-ai": "@deepseek-ai/dsh-llm-pi-ai",
        "agent-default-model": "@deepseek-ai/dsh-agent-default-model",
        "api-remotes": "@deepseek-ai/dsh-api-remotes",
        "client-runtime": "@deepseek-ai/dsh-client-runtime",
        "ui-settings": "@deepseek-ai/dsh-client-ui-settings",
        "multi-model-provider": "dsh-multi-model-provider",
        "loom-bundle": "dsh-loom",
        "loom-knowledge-catalog-skill": "dsh-loom/skill",
        "loom-web-runtime": "dsh-loom/web",
    }
    for entry_id, package in required_entries.items():
        block = entry_block(config, entry_id)
        if f"name: '{package}'" not in block and f"name: {package}" not in block:
            fail(f"{entry_id} does not resolve to {package}")

    llm = entry_block(config, "llm-pi-ai")
    for expected in (
        "lore-openai:",
        "apiKeyEnv: OPENAI_API_KEY",
        "baseURL: !!js process.env.OPENAI_BASE_URL",
        "api: openai-completions",
        "- id: gpt-5.6-luna",
        "- id: gpt-5.6-sol",
    ):
        if expected not in llm:
            fail(f"llm-pi-ai is missing {expected}")

    default_model = entry_block(config, "agent-default-model")
    for expected in ("provider: lore-openai", "model: gpt-5.6-luna", "reasoningEffort: low"):
        if expected not in default_model:
            fail(f"agent-default-model is missing {expected}")

    secret = os.environ.get("OPENAI_API_KEY", "")
    if secret and secret in config:
        fail("composed config contains the resolved OPENAI_API_KEY")


def main() -> None:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    prepare_parser = subparsers.add_parser("prepare")
    prepare_parser.add_argument("--profile-dir", type=Path, required=True)
    prepare_parser.add_argument("--source", type=Path, required=True)
    prepare_parser.add_argument("--multi-version", required=True)
    prepare_parser.add_argument("--lock-sha256", required=True)
    verify_parser = subparsers.add_parser("verify-config")
    verify_parser.add_argument("--config", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "prepare":
        prepare(args.profile_dir, args.source, args.multi_version, args.lock_sha256)
    else:
        verify_config(args.config)


if __name__ == "__main__":
    main()
