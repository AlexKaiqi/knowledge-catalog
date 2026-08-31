#!/usr/bin/env bash
# Clean-room Agent-first acceptance: six independent shell-only DSH role
# sessions share one KC Server. Linux/FUSE lifecycle is verified separately by
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
prepare_ephemeral_agent_home "$artifact_dir"
prepare_agent_profile "$plugin_dir" "$dsh_executable" "$profile_name"

export DSH_EXECUTABLE="$dsh_executable"
export DSH_MODEL_PATCH="$model_patch"
export KC_EXECUTABLE="$kc_bin"
export KC_HOME="$kc_home"
export KC_WORKSPACE="agent"
export DSH_PROFILE="$profile_name"
export KC_ROLE_ARTIFACTS="$artifact_dir"

admin_principal="service:agent-e2e"
server_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
server_url="http://127.0.0.1:${server_port}"
server_log="${artifact_dir}/kc-server.log"
"$kc_bin" local init --home "$kc_home" --catalog kr://acme/catalog >/dev/null
"$kc_bin" local repository attach --home "$kc_home" --repo kr://acme/public/core >/dev/null
"$kc_bin" local grant bootstrap --home "$kc_home" --principal "$admin_principal" >/dev/null
"$kc_bin" serve --home "$kc_home" --listen "127.0.0.1:${server_port}" >"$server_log" 2>&1 &
server_pid=$!
cleanup_server() {
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
}
trap cleanup_server EXIT
for _ in $(seq 1 100); do
  if curl --fail --silent "${server_url}/health" >/dev/null; then
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "kc serve exited during startup; inspect $server_log" >&2
    exit 1
  fi
  sleep 0.1
done
if ! curl --fail --silent "${server_url}/health" >/dev/null; then
  echo "kc serve did not become healthy; inspect $server_log" >&2
  exit 1
fi
export KC_SERVER_URL="$server_url"
export KC_AS="$admin_principal"

python3 "$plugin_dir/scripts/e2e_agent_roles.py"
printf 'evidence: %s\n' "$artifact_dir"
