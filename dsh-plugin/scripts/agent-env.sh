#!/usr/bin/env bash

# Load Agent API settings without evaluating the dotenv file as shell code.
# Existing process values win. AGENT_ENV_FILE is the preferred override;
# LORE_ENV remains accepted for older local invocations.
load_agent_api_env() {
  local env_file="${AGENT_ENV_FILE:-${LORE_ENV:-${HOME}/.env}}"
  [[ -f "$env_file" ]] || return 0

  while IFS= read -r -d '' env_name && IFS= read -r -d '' env_value; do
    case "$env_name" in
      DEEPSEEK_API_KEY)
        [[ -n "${DEEPSEEK_API_KEY:-}" ]] || export DEEPSEEK_API_KEY="$env_value"
        ;;
      OPENAI_API_KEY)
        [[ -n "${OPENAI_API_KEY:-}" ]] || export OPENAI_API_KEY="$env_value"
        ;;
      OPENAI_BASE_URL)
        [[ -n "${OPENAI_BASE_URL:-}" ]] || export OPENAI_BASE_URL="$env_value"
        ;;
    esac
  done < <(python3 - "$env_file" <<'PY'
import sys
from pathlib import Path

names = {
    "deepseek_api_key": "DEEPSEEK_API_KEY",
    "openai_api_key": "OPENAI_API_KEY",
    "openai_base_url": "OPENAI_BASE_URL",
}
values: dict[str, str] = {}
for raw_line in Path(sys.argv[1]).read_text().splitlines():
    line = raw_line.strip()
    if not line or line.startswith("#"):
        continue
    if line.startswith("export "):
        line = line[7:].lstrip()
    key, separator, value = line.partition("=")
    canonical = names.get(key.strip().lower())
    if not separator or canonical is None or canonical in values:
        continue
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        value = value[1:-1]
    if value:
        values[canonical] = value

for name, value in values.items():
    sys.stdout.buffer.write(name.encode() + b"\0" + value.encode() + b"\0")
PY
  )
}

require_deepseek_api_key() {
  if [[ -z "${DEEPSEEK_API_KEY:-}" ]]; then
    local env_file="${AGENT_ENV_FILE:-${LORE_ENV:-${HOME}/.env}}"
    echo "DEEPSEEK_API_KEY is required; export it or set it in $env_file" >&2
    return 1
  fi
}

require_agent_api_key_for_patch() {
  local model_patch="$1"
  if [[ "${model_patch##*/}" == "deepseek-official.patch.yml" ]]; then
    require_deepseek_api_key
  fi
}

# Build the current plugin and install exactly that copy into an isolated DSH
# profile. Both real-model suites use this path so local package-manager caches
# cannot make one suite exercise stale Skill content.
prepare_agent_profile() {
  local plugin_dir="$1"
  local dsh_executable="$2"
  local profile_name="$3"
  local profile_dir="${DSH_HOME:-${HOME}/.dsh}/profiles/${profile_name}"

  (cd "$plugin_dir" && npm install --legacy-peer-deps && npm run build)
  "$dsh_executable" plugin --profile "$profile_name" remove dsh-loom >/dev/null 2>&1 || true
  "$dsh_executable" plugin --profile "$profile_name" add "file:${plugin_dir}"

  python3 - "$profile_dir" "$plugin_dir" <<'PY'
import json, sys
from pathlib import Path

profile_dir = Path(sys.argv[1])
plugin_dir = Path(sys.argv[2])
package_path = profile_dir / "package.json"
data = json.loads(package_path.read_text())
bundles = data.setdefault("dsh", {}).setdefault("profile", {}).setdefault("bundles", [])
for name in ("@deepseek-ai/dsh-base", "@deepseek-ai/dsh-headless", "dsh-loom"):
    if name not in bundles:
        bundles.append(name)
package_path.write_text(json.dumps(data, indent=2) + "\n")

expected_version = json.loads((plugin_dir / "package.json").read_text())["version"]
installed = profile_dir / "node_modules" / "dsh-loom"
installed_version = json.loads((installed / "package.json").read_text())["version"]
if installed_version != expected_version:
    raise SystemExit(f"installed dsh-loom {installed_version}, expected {expected_version}")
skill = (installed / "skills" / "knowledge-catalog" / "SKILL.md").read_text()
for phrase in ('cmd:"read-workspace"', "knowledge_list", "Mental model and common questions"):
    if phrase not in skill:
        raise SystemExit(f"installed Knowledge Catalog Skill is stale: missing {phrase}")
PY
}
