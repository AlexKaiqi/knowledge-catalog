import importlib.util
import hashlib
import json
import os
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("dsh_profile.py")
SPEC = importlib.util.spec_from_file_location("dsh_profile", MODULE_PATH)
dsh_profile = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(dsh_profile)


SOURCE_CLIENT = """api.settings.describe().then(responseValue)
export const inject = ['slots', 'locale', 'sessions', 'remote', 'remote.settings']
inject: () => ({ api: ctx.remote, sessions }),
"""

COMPILED_CLIENT = """api.settings.describe().then(responseValue)
\t\tconst inject = [
\t\t\t"slots",
\t\t\t"locale",
\t\t\t"sessions",
\t\t\t"remote",
\t\t\t"remote.settings"
\t\t];
api: ctx.remote,
\t\t\t\t\tsessions
"""


class DSHProfileTest(unittest.TestCase):
    def make_profile(self, root: Path) -> tuple[Path, Path]:
        source = root / "source"
        profile = root / "profile"
        installed = profile / "node_modules" / "dsh-loom"
        multi = profile / "node_modules" / "dsh-multi-model-provider"
        for base in (source, installed):
            (base / "skills/knowledge-catalog").mkdir(parents=True)
            (base / "dist").mkdir()
            (base / "package.json").write_text('{"version":"0.3.0"}')
            (base / "skills/knowledge-catalog/SKILL.md").write_text("skill")
            (base / "dist/index.js").write_text("dist")
        (multi / "src/client").mkdir(parents=True)
        (multi / "lib").mkdir()
        (multi / "src/client/index.jsx").write_text(SOURCE_CLIENT)
        (multi / "lib/client.js").write_text(COMPILED_CLIENT)
        (multi / "package.json").write_text(json.dumps({
            "version": "0.1.0-rc.19",
            "dsh": {"client": {"inject": sorted({
                "@deepseek-ai/dsh-client-locale",
                "@deepseek-ai/dsh-client-runtime",
                "@deepseek-ai/dsh-client-ui-settings",
                "@deepseek-ai/dsh-api-remotes",
            })}},
        }))
        (profile / "package.json").write_text(json.dumps({
            "dsh": {"profile": {"bundles": dsh_profile.EXPECTED_BUNDLES}},
        }))
        (profile / "pnpm-lock.yaml").write_text("locked dependency graph\n")
        return profile, source

    def prepare(self, profile: Path, source: Path) -> None:
        lock_sha256 = hashlib.sha256((profile / "pnpm-lock.yaml").read_bytes()).hexdigest()
        dsh_profile.prepare(profile, source, "0.1.0-rc.19", lock_sha256)

    def test_prepare_patches_current_multi_model_browser_contract(self):
        with tempfile.TemporaryDirectory() as temp:
            profile, source = self.make_profile(Path(temp))
            self.prepare(profile, source)
            compiled = (profile / "node_modules/dsh-multi-model-provider/lib/client.js").read_text()
            self.assertIn('"connection"', compiled)
            self.assertIn('ctx.get("connection").api', compiled)
            self.assertIn("settings.describe({})", compiled)
            self.assertNotIn("remote.settings", compiled)

    def test_prepare_fails_closed_when_upstream_client_drifts(self):
        with tempfile.TemporaryDirectory() as temp:
            profile, source = self.make_profile(Path(temp))
            client = profile / "node_modules/dsh-multi-model-provider/src/client/index.jsx"
            client.write_text(client.read_text().replace("remote.settings", "settingsScope"))
            with self.assertRaises(SystemExit):
                self.prepare(profile, source)

    def test_prepare_fails_closed_when_dependency_graph_drifts(self):
        with tempfile.TemporaryDirectory() as temp:
            profile, source = self.make_profile(Path(temp))
            with self.assertRaises(SystemExit):
                dsh_profile.prepare(profile, source, "0.1.0-rc.19", "0" * 64)

    def test_verify_config_checks_model_and_client_stack_without_secret(self):
        entries = {
            "settings": "@deepseek-ai/dsh-settings-file",
            "api-remotes": "@deepseek-ai/dsh-api-remotes",
            "client-runtime": "@deepseek-ai/dsh-client-runtime",
            "ui-settings": "@deepseek-ai/dsh-client-ui-settings",
            "multi-model-provider": "dsh-multi-model-provider",
            "loom-bundle": "dsh-loom",
            "loom-knowledge-catalog-skill": "dsh-loom/skill",
            "loom-web-runtime": "dsh-loom/web",
        }
        config = "".join(f"- id: {entry}\n  name: '{package}'\n" for entry, package in entries.items())
        config += """- id: agent-default-model
  name: '@deepseek-ai/dsh-agent-default-model'
  config:
    provider: lore-openai
    model: gpt-5.6-luna
    reasoningEffort: low
- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      lore-openai:
        apiKeyEnv: OPENAI_API_KEY
        api: openai-completions
        baseURL: !!js process.env.OPENAI_BASE_URL
        models:
          - id: gpt-5.6-luna
          - id: gpt-5.6-sol
"""
        with tempfile.TemporaryDirectory() as temp:
            config_path = Path(temp) / "config.yml"
            config_path.write_text(config)
            old = os.environ.get("OPENAI_API_KEY")
            os.environ["OPENAI_API_KEY"] = "must-not-be-written"
            try:
                dsh_profile.verify_config(config_path)
            finally:
                if old is None:
                    os.environ.pop("OPENAI_API_KEY", None)
                else:
                    os.environ["OPENAI_API_KEY"] = old


if __name__ == "__main__":
    unittest.main()
