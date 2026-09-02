import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCKERFILE = ROOT / "docker" / "Dockerfile"
COMPOSE = ROOT / "compose.e2e.yaml"
CLI_SH = ROOT / "docker" / "cli.sh"
SMOKE = ROOT / "docker" / "cli-smoke.sh"
PROFILE = ROOT / "docker" / "cli.profile.sh"
BOOTSTRAP = ROOT / "docker" / "bootstrap.sh"


class ComposeCLITest(unittest.TestCase):
    def test_dockerfile_builds_cli_stage_with_pinned_ttyd(self):
        text = DOCKERFILE.read_text(encoding="utf-8")
        self.assertIn("FROM kc AS cli", text)
        self.assertIn("ARG TTYD_VERSION=1.7.7", text)
        self.assertIn("kc-compose-cli", text)
        self.assertIn("kc-compose-cli-smoke", text)

    def test_compose_exposes_localhost_cli_and_keeps_dsh_optional(self):
        text = COMPOSE.read_text(encoding="utf-8")
        self.assertIn("kc-dw-e2e-cli:local", text)
        self.assertIn("KC_DW_CLI_PORT:-7681", text)
        self.assertIn("profiles: [dsh]", text)
        self.assertIn("target: cli", text)
        cli_block = text.split("  cli:", 1)[1].split("  prometheus:", 1)[0]
        self.assertNotIn("KC_AS:", cli_block)

    def test_cli_entrypoint_is_http_ttyd_with_kc_client_env(self):
        text = CLI_SH.read_text(encoding="utf-8")
        self.assertIn("exec ttyd", text)
        self.assertIn("--writable", text)
        self.assertIn("KC_SERVER_URL", text)
        self.assertIn("unset KC_AS", text)
        self.assertIn("bash --login", text)

    def test_cli_smoke_covers_consumer_provider_and_governor(self):
        text = SMOKE.read_text(encoding="utf-8")
        self.assertIn("kc login --mode local --as agent:dsh", text)
        self.assertIn("kc identity whoami", text)
        self.assertIn("kc catalog list", text)
        self.assertIn("kc catalog show", text)
        self.assertIn("kc knowledge schema browse --repo kr://dw/physical", text)
        self.assertIn("kc catalog workspace resolve", text)
        self.assertIn("kc catalog repository list", text)
        self.assertIn("kc knowledge search", text)
        self.assertIn("kc knowledge read", text)
        self.assertIn("kc knowledge relations", text)
        self.assertIn("kc resource access", text)
        self.assertIn("kcfs plan", text)
        self.assertIn("kc login --mode local --as service:bootstrap", text)
        self.assertIn("kc writer ingest", text)
        self.assertIn("kc writer head", text)
        self.assertIn("kc admin grant list", text)
        self.assertIn("kc operations projection describe", text)
        self.assertIn("kc operations access describe", text)
        self.assertIn("kc catalog audit", text)
        self.assertNotIn("dsh-plugin", text)
        self.assertNotIn("DSH_HOME", text)
        self.assertNotIn("kc local", text)

    def test_cli_profile_lists_three_roles_and_asks_for_login(self):
        text = PROFILE.read_text(encoding="utf-8")
        self.assertIn("kc login --mode local --as agent:dsh", text)
        self.assertIn("kc catalog list", text)
        self.assertIn("kc knowledge schema browse", text)
        self.assertIn("kcfs plan", text)
        self.assertIn("kc login --mode local --as service:bootstrap", text)
        self.assertIn("do not export KC_AS", text)
        self.assertNotIn("export KC_AS=", text)

    def test_bootstrap_grants_consumer_discovery_not_projection_manage(self):
        text = BOOTSTRAP.read_text(encoding="utf-8")
        self.assertIn("ensure_consumer_policy", text)
        self.assertIn("catalog.read", text)
        self.assertIn("knowledge.schema.read", text)
        self.assertIn("revoke_action agent:dsh projection.manage", text)
        self.assertIn("kc_bootstrap operations projection sync", text)
        self.assertNotIn("--as agent:dsh \\\n    --repo \"$physical\" --ref refs/heads/main", text)


if __name__ == "__main__":
    unittest.main()
