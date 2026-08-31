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

# Prefer an already-installed runtime matching the repository's declared Node
# version. This keeps local acceptance reproducible without mutating a user's
# global Node selection or installing software behind their back.
activate_agent_node_runtime() {
  local plugin_dir="$1"
  local repo_root
  repo_root="$(cd "$plugin_dir/.." && pwd)"
  local requested=""
  [[ ! -f "$repo_root/.node-version" ]] || requested="$(tr -d '[:space:]' <"$repo_root/.node-version")"
  local major="${requested%%.*}"
  [[ -n "$major" ]] || return 0
  if command -v node >/dev/null 2>&1 && [[ "$(node -p 'process.versions.node.split(".")[0]')" == "$major" ]]; then
    return 0
  fi
  local candidate
  for candidate in \
    "/opt/homebrew/opt/node@${major}/bin" \
    "/usr/local/opt/node@${major}/bin" \
    "${HOME}/.nvm/versions/node/v${requested}/bin"; do
    if [[ -x "$candidate/node" ]] && [[ "$($candidate/node -p 'process.versions.node.split(".")[0]')" == "$major" ]]; then
      export PATH="$candidate:$PATH"
      return 0
    fi
  done
  while IFS= read -r candidate; do
    if [[ -x "$candidate/node" ]] && [[ "$($candidate/node -p 'process.versions.node.split(".")[0]')" == "$major" ]]; then
      export PATH="$candidate:$PATH"
      return 0
    fi
  done < <(compgen -G "${HOME}/.nvm/versions/node/v${major}*/bin" || true)
}

# Fail before expensive deterministic gates when the local DSH/plugin runtime
# cannot satisfy the version contract. The dsh executable uses /usr/bin/env
# node, so the Node on PATH is the runtime that matters here.
require_agent_runtime() {
  local plugin_dir="$1"
  local dsh_executable="$2"
  command -v "$dsh_executable" >/dev/null 2>&1 || {
    echo "DSH executable not found: $dsh_executable" >&2
    return 1
  }
  command -v node >/dev/null 2>&1 || { echo "Node.js is required" >&2; return 1; }
  command -v npm >/dev/null 2>&1 || { echo "npm is required" >&2; return 1; }
  local required_major actual_major actual_version
  required_major="$(python3 - "$plugin_dir/package.json" <<'PY'
import json, re, sys
from pathlib import Path

engine = json.loads((Path(sys.argv[1])).read_text())["engines"]["node"]
match = re.search(r">=([0-9]+)", engine)
if not match:
    raise SystemExit(f"unsupported Node engine declaration: {engine}")
print(match.group(1))
PY
  )"
  actual_major="$(node -p 'process.versions.node.split(".")[0]')"
  actual_version="$(node --version)"
  if [[ "$actual_major" != "$required_major" ]]; then
    echo "Node $required_major.x is required by dsh-loom; found $actual_version on PATH" >&2
    return 1
  fi
  echo "[preflight] DSH: $dsh_executable; Node: $actual_version"
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

# Create a run-scoped DSH home without copying credentials into its evidence
# directory. The home-level patch points the ephemeral credentials provider at
# the caller's existing store; plugin profiles and sessions stay under the run.
# Call this only after model selection has inspected the caller's DSH home.
prepare_ephemeral_agent_home() {
  local artifact_root="$1"
  local source_dsh_home="${DSH_HOME:-${HOME}/.dsh}"
  local credentials_file="${DSH_CREDENTIALS_FILE:-${source_dsh_home}/.credentials.yaml}"

  mkdir -p "$artifact_root"
  chmod 700 "$artifact_root"
  local ephemeral_home
  ephemeral_home="$(mktemp -d "${artifact_root%/}/dsh-home.XXXXXX")"
  chmod 700 "$ephemeral_home"
  export DSH_HOME="$ephemeral_home"
  export DSH_AGENT_EPHEMERAL_HOME="$ephemeral_home"

  if [[ -f "$credentials_file" ]]; then
    export DSH_CREDENTIALS_FILE="$credentials_file"
    python3 - "$ephemeral_home/cordis.patch.yml" <<'PY'
import sys
from pathlib import Path

Path(sys.argv[1]).write_text("""- id: credentials
  config:
    path: !!js process.env.DSH_CREDENTIALS_FILE
    watch: false
""")
PY
  fi
}

# Build the current plugin and install exactly that copy into a run-scoped DSH
# profile. Refuse a normal user home: acceptance plugins are task fixtures, not
# general DSH extensions, and must never persist in ~/.dsh/profiles.
prepare_agent_profile() {
  local plugin_dir="$1"
  local dsh_executable="$2"
  local profile_name="$3"
  if [[ -z "${DSH_HOME:-}" || "${DSH_AGENT_EPHEMERAL_HOME:-}" != "$DSH_HOME" ]]; then
    echo "prepare_agent_profile requires prepare_ephemeral_agent_home; refusing persistent DSH profile installation" >&2
    return 1
  fi
  local profile_dir="${DSH_HOME}/profiles/${profile_name}"

  (cd "$plugin_dir" && npm ci --ignore-scripts --legacy-peer-deps && npm run build)
  "$dsh_executable" plugin --profile "$profile_name" remove dsh-loom >/dev/null 2>&1 || true
  npm_config_ignore_scripts=true "$dsh_executable" plugin --profile "$profile_name" add "file:${plugin_dir}"

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
