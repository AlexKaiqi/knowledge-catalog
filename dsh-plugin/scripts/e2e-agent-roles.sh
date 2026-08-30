#!/usr/bin/env bash
# Clean-room Agent-first acceptance: six independent shell-only DSH role
# sessions share one KC home. Linux/FUSE lifecycle is verified separately by
# the Docker kcfs suite; it is not a prerequisite for this paid model suite.
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/../.." && pwd)"
plugin_dir="${root_dir}/dsh-plugin"
source "${plugin_dir}/scripts/agent-env.sh"
load_agent_api_env
model_patch="$(select_agent_model_patch "$plugin_dir")"
require_agent_api_key_for_patch "$model_patch"
profile_name="${DSH_PROFILE:-loom-agent-roles}"
dsh_executable="${DSH_EXECUTABLE:-$(command -v dsh || true)}"
if [[ -z "$dsh_executable" || ! -x "$dsh_executable" ]]; then
  echo "dsh executable not found; put dsh on PATH or set DSH_EXECUTABLE=/absolute/path/to/dsh" >&2
  exit 1
fi
kc_home="${KC_ROLE_HOME:-$(mktemp -d /tmp/kc-agent-roles-home.XXXXXX)}"
kc_bin_dir="$(mktemp -d /tmp/kc-agent-roles-bin.XXXXXX)"
kc_bin="${KC_EXECUTABLE:-${KC_BIN:-$kc_bin_dir/kc}}"
artifact_dir="${KC_ROLE_ARTIFACTS:-$(mktemp -d /tmp/kc-agent-roles-evidence.XXXXXX)}"
export PATH="${HOME}/.local/go/bin:${HOME}/.local/bin:$(dirname "$kc_bin"):${PATH}"

go -C "$root_dir" build -o "$kc_bin" ./cmd/kc
prepare_agent_profile "$plugin_dir" "$dsh_executable" "$profile_name"

export DSH_EXECUTABLE="$dsh_executable"
export DSH_MODEL_PATCH="$model_patch"
export KC_EXECUTABLE="$kc_bin"
export KC_HOME="$kc_home"
export KC_WORKSPACE="agent"
export DSH_PROFILE="$profile_name"
export KC_ROLE_ARTIFACTS="$artifact_dir"

python3 "$plugin_dir/scripts/e2e_agent_roles.py"
printf 'evidence: %s\n' "$artifact_dir"
