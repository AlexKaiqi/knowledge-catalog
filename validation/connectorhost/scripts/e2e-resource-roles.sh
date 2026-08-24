#!/usr/bin/env bash
set -euo pipefail

scene_root="$(cd "$(dirname "$0")/../../.." && pwd)"
main_root="$(cd "${scene_root}/../.." && pwd)"
plugin_dir="${main_root}/dsh-plugin"
profile_name="${DSH_PROFILE:-loom-resource-roles}"
evidence_dir="${RESOURCE_E2E_EVIDENCE:-$(mktemp -d /tmp/kc-resource-e2e.XXXXXX)}"
integration_repo="${evidence_dir}/payments-integration"
remote_repo="${evidence_dir}/payments-integration.git"
source_dir="${evidence_dir}/external-source"
kc_home="${evidence_dir}/kc-home"
host_home="${evidence_dir}/host-home"
kc_bin="${evidence_dir}/kc"
host_bin="${evidence_dir}/integration-host"

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

kc_port="${RESOURCE_E2E_KC_PORT:-$(free_port)}"
host_port="${RESOURCE_E2E_HOST_PORT:-$(free_port)}"
supervisor_port="${RESOURCE_E2E_SUPERVISOR_PORT:-$(free_port)}"
kc_url="http://127.0.0.1:${kc_port}"
host_url="http://127.0.0.1:${host_port}"
supervisor_url="http://127.0.0.1:${supervisor_port}"

cleanup() {
  if [[ "${RESOURCE_E2E_KEEP_SERVICES:-0}" == "1" ]]; then
    return
  fi
  [[ -z "${supervisor_pid:-}" ]] || kill "${supervisor_pid}" 2>/dev/null || true
  local pids
  pids="$(lsof -tiTCP:"${host_port}" -sTCP:LISTEN 2>/dev/null || true)"
  [[ -z "${pids}" ]] || kill ${pids} 2>/dev/null || true
  pids="$(lsof -tiTCP:"${kc_port}" -sTCP:LISTEN 2>/dev/null || true)"
  [[ -z "${pids}" ]] || kill ${pids} 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "${integration_repo}/connectors" "${source_dir}" "${kc_home}" "${host_home}"

python3 - "${source_dir}/payment-ops.json" <<'PY'
import json, sys
from pathlib import Path
Path(sys.argv[1]).write_text(json.dumps({
    "sourceRef": "payments://operations/payment-api",
    "observedAt": "2026-08-24T09:00:00Z",
    "revision": "ops-r1",
    "service": {
        "key": "payment-api",
        "name": "Payment API",
        "owner": "payments-platform",
        "oncall": "pay-primary",
        "status": "healthy"
    },
    "traces": [{
        "traceId": "trace-001",
        "timestamp": "2026-08-24T08:55:00Z",
        "status": "OK",
        "summary": "authorization completed"
    }]
}, indent=2) + "\n")
PY

python3 - "${integration_repo}/README.md" <<'PY'
from pathlib import Path
import sys
Path(sys.argv[1]).write_text("""# Payments integration repository

This business-owned repository is the delivery and maintenance unit for the
Payment API Collector and live resource access implementation. Develop only
under `connectors/`; source data is provided at runtime through
`PAYMENT_OPS_SOURCE` and must not be copied into this repository.
""")
PY

git init --bare "${remote_repo}" >/dev/null
git -C "${integration_repo}" init -b main >/dev/null
git -C "${integration_repo}" add README.md connectors
git -C "${integration_repo}" -c user.name=resource-e2e -c user.email=resource-e2e@example.invalid \
  commit -m 'initialize payments integration repository' >/dev/null
git -C "${integration_repo}" remote add origin "${remote_repo}"
git -C "${integration_repo}" push -u origin main >/dev/null

export PATH="${HOME}/.local/go/bin:${HOME}/.local/bin:${PATH}"
go -C "${main_root}" build -o "${kc_bin}" ./cmd/kc
go -C "${scene_root}" build -o "${host_bin}" ./validation/connectorhost/cmd/connector-host
(cd "${plugin_dir}" && npm run build >/dev/null)

dsh plugin --profile "${profile_name}" remove dsh-loom >/dev/null 2>&1 || true
dsh plugin --profile "${profile_name}" add "file:${plugin_dir}" >/dev/null
profile_dir="${DSH_HOME:-${HOME}/.dsh}/profiles/${profile_name}"
python3 - "${profile_dir}" <<'PY'
import json, sys
from pathlib import Path
path = sys.argv[1] + "/package.json"
with open(path) as handle:
    data = json.load(handle)
bundles = data.setdefault("dsh", {}).setdefault("profile", {}).setdefault("bundles", [])
for name in ("@deepseek-ai/dsh-base", "@deepseek-ai/dsh-headless", "dsh-loom"):
    if name not in bundles:
        bundles.append(name)
with open(path, "w") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
Path(sys.argv[1], "cordis.patch.yml").write_text("- id: loom-web\n  disabled: true\n")
PY

export DSH_PROFILE="${profile_name}"
export DSH_MODEL_PATCH="${plugin_dir}/scripts/deepseek-official.patch.yml"
export RESOURCE_E2E_EVIDENCE="${evidence_dir}"
export INTEGRATION_REPO="${integration_repo}"
export INTEGRATION_REMOTE="${remote_repo}"
export PAYMENT_OPS_SOURCE="${source_dir}/payment-ops.json"
export KC_BIN="${kc_bin}"
export KC_HOME="${kc_home}"
export KC_SERVE="${kc_url}"
export KC_CATALOG="kr://demo/catalog"
export KC_WORKSPACE="payments-agent"
export KC_RESOURCE_ACCESS_URL="${host_url}"
export INTEGRATION_HOST_BIN="${host_bin}"
export INTEGRATION_HOST_HOME="${host_home}"
export INTEGRATION_HOST_LISTEN="127.0.0.1:${host_port}"
export INTEGRATION_HOST_LOG="${evidence_dir}/integration-host.log"
export INTEGRATION_SUPERVISOR_LISTEN="127.0.0.1:${supervisor_port}"
export INTEGRATION_SUPERVISOR_URL="${supervisor_url}"

python3 "${scene_root}/validation/connectorhost/scripts/runtime_supervisor.py" \
  >"${evidence_dir}/supervisor.log" 2>&1 &
supervisor_pid=$!
for _ in $(seq 1 100); do
  curl -fsS "${supervisor_url}/health" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "${supervisor_url}/health" >"${evidence_dir}/supervisor-health.json"

python3 "${scene_root}/validation/connectorhost/scripts/e2e_resource_roles.py"

printf 'EVIDENCE=%s\nKC_URL=%s\nRESOURCE_URL=%s\n' "${evidence_dir}" "${kc_url}" "${host_url}"
if [[ "${RESOURCE_E2E_KEEP_SERVICES:-0}" == "1" ]]; then
  trap - EXIT
fi
