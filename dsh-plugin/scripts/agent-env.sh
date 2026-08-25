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
