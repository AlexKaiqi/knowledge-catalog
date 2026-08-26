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
dsh_executable="${DSH_EXECUTABLE:-$(command -v dsh || true)}"
if [[ -z "$dsh_executable" || ! -x "$dsh_executable" ]]; then
  echo "dsh executable not found; put dsh on PATH or set DSH_EXECUTABLE=/absolute/path/to/dsh" >&2
  exit 1
fi
export DSH_EXECUTABLE="$dsh_executable"
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
prepare_agent_profile "$plugin_dir" "$dsh_executable" "$profile_name"

export KC_BIN="$kc_bin"
export KC_HOME="$kc_home"
export KC_SERVE="$base_url"
export KC_WORKSPACE="agent"
export DSH_PROFILE="$profile_name"
export KC_ROLE_ARTIFACTS="$artifact_dir"

python3 "$plugin_dir/scripts/e2e_agent_roles.py"
printf 'evidence: %s\n' "$artifact_dir"
