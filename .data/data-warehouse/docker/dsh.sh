#!/usr/bin/env bash
set -euo pipefail

export AGENT_ENV_FILE=/run/user.env
export DSH_HOME=/run-state/dsh-home
export DSH_AGENT_EPHEMERAL_HOME="$DSH_HOME"
source /opt/dsh-loom/scripts/agent-env.sh
load_agent_api_env
require_agent_credential OPENAI_API_KEY
[[ -n "${OPENAI_BASE_URL:-}" ]] || { echo "OPENAI_BASE_URL is required" >&2; exit 1; }

profile=kc-compose-web
profile_dir="$DSH_HOME/profiles/$profile"
source_digest="$({ sha256sum \
  /opt/dsh-loom/package.json \
  /opt/dsh-loom/cordis.patch.yml \
  /opt/dsh-loom/skills/knowledge-catalog/SKILL.md \
  /opt/dsh-loom/dist/index.js; } | sha256sum | cut -d' ' -f1)"
stamp="$DSH_HOME/.kc-compose-profile"

mkdir -p "$DSH_HOME" /run-state/kc-client /workspace
chmod 0700 "$DSH_HOME" /run-state/kc-client
if [[ ! -f "$stamp" || "$(<"$stamp")" != "$source_digest" ]]; then
  rm -rf "$profile_dir"
  dsh plugin --profile "$profile" add "dsh-multi-model-provider@${DSH_MULTI_MODEL_VERSION}"
  dsh plugin --profile "$profile" add file:/opt/dsh-loom
  npm --prefix "$profile_dir" pkg set \
    'dsh.profile.bundles[0]=@deepseek-ai/dsh-base' \
    'dsh.profile.bundles[1]=@deepseek-ai/dsh-web-app' \
    'dsh.profile.bundles[2]=dsh-multi-model-provider' \
    'dsh.profile.bundles[3]=dsh-loom'
  python3 - "$profile_dir" <<'PY'
import json
import sys
from pathlib import Path

profile = Path(sys.argv[1])
source = Path("/opt/dsh-loom")
installed = profile / "node_modules" / "dsh-loom"
if json.loads((installed / "package.json").read_text())["version"] != json.loads((source / "package.json").read_text())["version"]:
    raise SystemExit("installed dsh-loom version differs from the image source")
if (installed / "skills/knowledge-catalog/SKILL.md").read_text() != (source / "skills/knowledge-catalog/SKILL.md").read_text():
    raise SystemExit("installed Knowledge Catalog Skill differs from the image source")
multi = profile / "node_modules" / "dsh-multi-model-provider" / "package.json"
if json.loads(multi.read_text())["version"] != "0.1.0-rc.19":
    raise SystemExit("unexpected dsh-multi-model-provider version")
PY
  printf '%s\n' "$source_digest" >"$stamp"
fi

socat TCP-LISTEN:7400,fork,reuseaddr TCP:127.0.0.1:7401 &
exec dsh \
  --profile "$profile" \
  --patch /opt/dsh-config/gpt-web.patch.yml \
  --host 127.0.0.1 \
  --port 7401 \
  --no-open
