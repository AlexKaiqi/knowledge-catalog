#!/usr/bin/env bash
# Clean-room Agent-first acceptance: an empty directory, six independent DSH
# role sessions, one persisted kc home, and no manual kc command execution.
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/../.." && pwd)"
plugin_dir="${root_dir}/dsh-plugin"
source "${plugin_dir}/scripts/agent-env.sh"
load_agent_api_env
require_agent_api_key_for_patch "${DSH_MODEL_PATCH:-$plugin_dir/scripts/deepseek-official.patch.yml}"
profile_name="${DSH_PROFILE:-loom-agent-roles}"
port="${KC_ROLE_PORT:-18380}"
kc_home="${KC_ROLE_HOME:-$(mktemp -d /tmp/kc-agent-roles-home.XXXXXX)}"
kc_bin="${KC_BIN:-$(mktemp /tmp/kc-agent-roles-bin.XXXXXX)}"
artifact_dir="${KC_ROLE_ARTIFACTS:-$(mktemp -d /tmp/kc-agent-roles-evidence.XXXXXX)}"
base_url="http://127.0.0.1:${port}"
export PATH="${HOME}/.local/go/bin:${HOME}/.local/bin:${PATH}"

cleanup() {
  local pids
  pids="$(lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids 2>/dev/null || true
  fi
}
trap cleanup EXIT

go -C "$root_dir" build -o "$kc_bin" ./cmd/kc
(cd "$plugin_dir" && npm install --legacy-peer-deps && npm run build)

# pnpm caches directory dependencies by their file: locator. Removing the
# package first is the local-development cachebuster; `add --force` alone may
# leave an older packed copy in node_modules.
dsh plugin --profile "$profile_name" remove dsh-loom >/dev/null 2>&1 || true
dsh plugin --profile "$profile_name" add "file:${plugin_dir}"
profile_dir="${DSH_HOME:-${HOME}/.dsh}/profiles/${profile_name}"
python3 - "$profile_dir" "$plugin_dir" <<'PY'
import json, sys
path = sys.argv[1] + "/package.json"
plugin_dir = sys.argv[2]
with open(path) as handle:
    data = json.load(handle)
bundles = data.setdefault("dsh", {}).setdefault("profile", {}).setdefault("bundles", [])
for name in ("@deepseek-ai/dsh-base", "@deepseek-ai/dsh-headless", "dsh-loom"):
    if name not in bundles:
        bundles.append(name)
with open(path, "w") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")

with open(plugin_dir + "/package.json") as handle:
    expected_version = json.load(handle)["version"]
with open(sys.argv[1] + "/node_modules/dsh-loom/package.json") as handle:
    installed_version = json.load(handle)["version"]
if installed_version != expected_version:
    raise SystemExit(
        f"installed dsh-loom {installed_version}, expected {expected_version}"
    )
skill = open(
    sys.argv[1] + "/node_modules/dsh-loom/skills/knowledge-catalog/SKILL.md"
).read()
if 'cmd:"read-workspace"' not in skill:
    raise SystemExit("installed Knowledge Catalog Skill is stale")
PY

export KC_BIN="$kc_bin"
export KC_HOME="$kc_home"
export KC_SERVE="$base_url"
export KC_WORKSPACE="agent"
export DSH_PROFILE="$profile_name"
export KC_ROLE_ARTIFACTS="$artifact_dir"

python3 "$plugin_dir/scripts/e2e_agent_roles.py"
printf 'evidence: %s\n' "$artifact_dir"
