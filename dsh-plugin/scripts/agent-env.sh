#!/usr/bin/env bash

# Load Agent API settings without evaluating the dotenv file as shell code.
# Existing process values win. AGENT_ENV_FILE is the preferred override;
# LORE_ENV remains accepted for older local invocations.
load_agent_api_env() {
  local env_file="${AGENT_ENV_FILE:-${LORE_ENV:-${HOME}/.env}}"
  if [[ -f "$env_file" ]]; then
    while IFS= read -r -d '' env_name && IFS= read -r -d '' env_value; do
      case "$env_name" in
        NPM_TOKEN)
          [[ -n "${NPM_TOKEN:-}" ]] || export NPM_TOKEN="$env_value"
          ;;
        DEEPSEEK_API_KEY)
          [[ -n "${DEEPSEEK_API_KEY:-}" ]] || export DEEPSEEK_API_KEY="$env_value"
          ;;
        OPENAI_API_KEY)
          [[ -n "${OPENAI_API_KEY:-}" ]] || export OPENAI_API_KEY="$env_value"
          ;;
        OPENAI_BASE_URL)
          [[ -n "${OPENAI_BASE_URL:-}" ]] || export OPENAI_BASE_URL="$env_value"
          ;;
        OPENROUTER_API_KEY)
          [[ -n "${OPENROUTER_API_KEY:-}" ]] || export OPENROUTER_API_KEY="$env_value"
          ;;
      esac
    done < <(python3 - "$env_file" <<'PY'
import sys
from pathlib import Path

names = {
    "npm_token": "NPM_TOKEN",
    "deepseek_api_key": "DEEPSEEK_API_KEY",
    "openai_api_key": "OPENAI_API_KEY",
    "openai_base_url": "OPENAI_BASE_URL",
    "openrouter_api_key": "OPENROUTER_API_KEY",
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
  fi

  # DSH users commonly keep an OpenAI-compatible endpoint in settings.yaml
  # and the corresponding OPENAI_API_KEY ref in .credentials.yaml. Reuse that
  # endpoint without evaluating YAML or copying credentials into this repo.
  if [[ -z "${OPENAI_BASE_URL:-}" ]]; then
    local settings_file="${DSH_SETTINGS_FILE:-${DSH_HOME:-${HOME}/.dsh}/settings.yaml}"
    if [[ -f "$settings_file" ]]; then
      local configured_base_url
      configured_base_url="$(python3 - "$settings_file" <<'PY'
import re
import sys
from pathlib import Path

for raw_line in Path(sys.argv[1]).read_text().splitlines():
    match = re.match(r"^\s*baseURL\s*:\s*(.*?)\s*$", raw_line)
    if not match:
        continue
    value = match.group(1).strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        value = value[1:-1]
    if value.startswith(("http://", "https://")):
        print(value)
        break
PY
      )"
      [[ -z "$configured_base_url" ]] || export OPENAI_BASE_URL="$configured_base_url"
    fi
  fi
}

agent_credential_ref_available() {
  local name="$1"
  local credentials_file="${DSH_CREDENTIALS_FILE:-${DSH_HOME:-${HOME}/.dsh}/.credentials.yaml}"
  [[ -n "${!name:-}" ]] && return 0
  [[ -f "$credentials_file" ]] || return 1
  grep -Eq "^[[:space:]]+${name}:[[:space:]]*" "$credentials_file"
}

require_agent_credential() {
  local name="$1"
  if ! agent_credential_ref_available "$name"; then
    local env_file="${AGENT_ENV_FILE:-${LORE_ENV:-${HOME}/.env}}"
    local credentials_file="${DSH_CREDENTIALS_FILE:-${DSH_HOME:-${HOME}/.dsh}/.credentials.yaml}"
    echo "$name is required; export it, set it in $env_file, or register its ref in $credentials_file" >&2
    return 1
  fi
}

require_agent_api_key_for_patch() {
  local model_patch="$1"
  case "${model_patch##*/}" in
    deepseek-official.patch.yml)
      require_agent_credential DEEPSEEK_API_KEY
      ;;
    openai-official.patch.yml)
      require_agent_credential OPENAI_API_KEY
      ;;
    lore-openai.patch.yml)
      require_agent_credential OPENAI_API_KEY
      [[ -n "${OPENAI_BASE_URL:-}" ]] || {
        echo "OPENAI_BASE_URL is required by lore-openai.patch.yml" >&2
        return 1
      }
      ;;
    openrouter.patch.yml)
      require_agent_credential OPENROUTER_API_KEY
      ;;
    volcengine.patch.yml)
      require_agent_credential ARK_API_KEY
      ;;
  esac
}

select_agent_model_patch() {
  local plugin_dir="$1"
  if [[ -n "${DSH_MODEL_PATCH:-}" ]]; then
    printf '%s\n' "$DSH_MODEL_PATCH"
  elif [[ -n "${OPENAI_BASE_URL:-}" ]] && agent_credential_ref_available OPENAI_API_KEY; then
    printf '%s\n' "$plugin_dir/scripts/lore-openai.patch.yml"
  elif [[ -n "${OPENAI_API_KEY:-}" ]]; then
    printf '%s\n' "$plugin_dir/scripts/openai-official.patch.yml"
  elif agent_credential_ref_available OPENAI_API_KEY; then
    printf '%s\n' "$plugin_dir/scripts/openai-official.patch.yml"
  elif [[ -n "${OPENROUTER_API_KEY:-}" ]]; then
    printf '%s\n' "$plugin_dir/scripts/openrouter.patch.yml"
  elif agent_credential_ref_available OPENROUTER_API_KEY; then
    printf '%s\n' "$plugin_dir/scripts/openrouter.patch.yml"
  elif [[ -n "${DEEPSEEK_API_KEY:-}" ]]; then
    printf '%s\n' "$plugin_dir/scripts/deepseek-official.patch.yml"
  elif agent_credential_ref_available DEEPSEEK_API_KEY; then
    printf '%s\n' "$plugin_dir/scripts/deepseek-official.patch.yml"
  elif agent_credential_ref_available ARK_API_KEY; then
    printf '%s\n' "$plugin_dir/scripts/volcengine.patch.yml"
  else
    echo "no supported Agent model credential is configured" >&2
    return 1
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
expected_skill = (plugin_dir / "skills" / "knowledge-catalog" / "SKILL.md").read_text()
if skill != expected_skill:
    raise SystemExit("installed Knowledge Catalog Skill differs from the current plugin source")
if len(skill.encode()) >= 5_000:
    raise SystemExit("Knowledge Catalog Skill exceeds the 5KB prompt budget")
PY
}
