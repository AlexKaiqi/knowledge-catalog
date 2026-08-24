#!/usr/bin/env bash
set -euo pipefail

scene_root="$(cd "$(dirname "$0")/../../.." && pwd)"
host_src="${scene_root}/validation/connectorhost"
model_patch="${scene_root}/dsh-plugin/scripts/deepseek-official.patch.yml"
evidence_dir="${CONNECTOR_E2E_EVIDENCE:-$(mktemp -d /tmp/kc-connector-e2e.XXXXXX)}"
public_repo="${evidence_dir}/public-connectors"
remote_repo="${evidence_dir}/public-connectors.git"
kc_home="${evidence_dir}/kc-home"
host_home="${evidence_dir}/host-home"
kc_bin="${evidence_dir}/kc"
host_bin="${evidence_dir}/connector-host"
kc_listen="${CONNECTOR_E2E_KC_LISTEN:-127.0.0.1:17381}"
host_listen="${CONNECTOR_E2E_HOST_LISTEN:-127.0.0.1:17480}"
kc_url="http://${kc_listen}"
host_url="http://${host_listen}"

cleanup() {
  if [[ "${CONNECTOR_E2E_KEEP_SERVICES:-0}" != "1" ]]; then
    [[ -z "${host_pid:-}" ]] || kill "${host_pid}" 2>/dev/null || true
    [[ -z "${kc_pid:-}" ]] || kill "${kc_pid}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

mkdir -p "${public_repo}" "${kc_home}" "${host_home}"

go -C "${scene_root}" build -o "${kc_bin}" ./cmd/kc
go -C "${scene_root}" build -o "${host_bin}" ./validation/connectorhost/cmd/connector-host

mkdir -p "${public_repo}/connectors"
cp -R "${host_src}/testdata/agent-repo/." "${public_repo}/"
cp "${host_src}/README.md" "${public_repo}/CONNECTOR_SPEC.md"
cp "${host_src}/AGENT_TASK.md" "${public_repo}/TASK.md"

git init --bare "${remote_repo}" >/dev/null
git -C "${public_repo}" init -b main >/dev/null
git -C "${public_repo}" add .
git -C "${public_repo}" -c user.name=connector-e2e -c user.email=connector-e2e@example.invalid \
  commit -m 'initialize public Connector repository' >/dev/null
git -C "${public_repo}" remote add origin "${remote_repo}"
git -C "${public_repo}" push -u origin main >/dev/null

"${kc_bin}" --home "${kc_home}" init --catalog kr://agent/catalog >/dev/null
"${kc_bin}" --home "${kc_home}" repo-add --repo kr://agent/public/services >/dev/null
"${kc_bin}" --home "${kc_home}" allow \
  --principal connector/service-observer --cmd commit \
  --repo kr://agent/public/services --ref refs/heads/main >/dev/null

"${kc_bin}" --home "${kc_home}" serve --listen "${kc_listen}" >"${evidence_dir}/kc.log" 2>&1 &
kc_pid=$!
for _ in $(seq 1 100); do
  curl -fsS "${kc_url}/health" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "${kc_url}/health" >"${evidence_dir}/kc-health.json"

task="$(cat "${public_repo}/TASK.md")"
(
  cd "${public_repo}"
  DSH_PERMISSION_MODE=danger-full-access \
    dsh --profile headless --patch "${model_patch}" "${task}"
) >"${evidence_dir}/agent.out" 2>"${evidence_dir}/agent.err"
grep -q 'CONNECTOR_DEVELOPED=service-observer' "${evidence_dir}/agent.out"

git -C "${public_repo}" add connectors/service-observer
git -C "${public_repo}" -c user.name=connector-e2e -c user.email=connector-e2e@example.invalid \
  commit -m 'add service observer' >/dev/null
git -C "${public_repo}" push origin main >/dev/null

"${host_bin}" --home "${host_home}" repo-set \
  --repo "${remote_repo}" --ref refs/heads/main --sync-every 1s --kc-url "${kc_url}" \
  >"${evidence_dir}/repo-set.json"
"${host_bin}" --home "${host_home}" validate --connector service-observer \
  >"${evidence_dir}/validate.json"
"${host_bin}" --home "${host_home}" run --connector service-observer --preview \
  >"${evidence_dir}/preview.json"
"${host_bin}" --home "${host_home}" run --connector service-observer \
  >"${evidence_dir}/run-initial.json"

python3 - "${public_repo}/sources/services.json" <<'PY'
import json, sys
path = sys.argv[1]
data = {
    "sourceRef": "file://agent/services.json",
    "capturedAt": "2026-08-24T13:00:00Z",
    "services": [
        {"key": "billing", "name": "Billing API", "owner": "finance-platform"},
        {"key": "inventory", "name": "Inventory API", "owner": "supply"},
    ],
}
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY

git -C "${public_repo}" add sources/services.json
git -C "${public_repo}" -c user.name=connector-e2e -c user.email=connector-e2e@example.invalid \
  commit -m 'update observed service source' >/dev/null
git -C "${public_repo}" push origin main >/dev/null
"${host_bin}" --home "${host_home}" sync >"${evidence_dir}/sync-refresh.json"

"${host_bin}" --home "${host_home}" run --connector service-observer \
  >"${evidence_dir}/run-refresh.json"
"${host_bin}" --home "${host_home}" activate --connector service-observer \
  >"${evidence_dir}/activate.json"
"${host_bin}" --home "${host_home}" history --connector service-observer \
  >"${evidence_dir}/history.json"

"${host_bin}" --home "${host_home}" serve --listen "${host_listen}" >"${evidence_dir}/host.log" 2>&1 &
host_pid=$!
for _ in $(seq 1 100); do
  curl -fsS "${host_url}/health" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "${host_url}/api/connectors" >"${evidence_dir}/connectors.json"

post() {
  local verb="$1"
  local body="$2"
  curl -fsS -X POST "${kc_url}/v1/${verb}" -H 'content-type: application/json' -d "${body}"
}
post read '{"repo":"kr://agent/public/services","ref":"refs/heads/main","object":"Service:billing"}' >"${evidence_dir}/billing.json"
post read '{"repo":"kr://agent/public/services","ref":"refs/heads/main","object":"Service:inventory"}' >"${evidence_dir}/inventory.json"
post provenance '{"repo":"kr://agent/public/services","ref":"refs/heads/main","object":"Service:billing"}' >"${evidence_dir}/provenance.json"
if post read '{"repo":"kr://agent/public/services","ref":"refs/heads/main","object":"Service:search"}' >"${evidence_dir}/search.json" 2>&1; then
  echo "removed Service:search is still readable" >&2
  exit 1
fi

python3 - "${evidence_dir}" <<'PY'
import json, sys
from pathlib import Path

root = Path(sys.argv[1])
preview = json.loads((root / "preview.json").read_text())
initial = json.loads((root / "run-initial.json").read_text())
refresh = json.loads((root / "run-refresh.json").read_text())
history = json.loads((root / "history.json").read_text())
if preview["outcome"] != "PREVIEWED" or preview["summary"]["added"] != 2:
    raise SystemExit(f"bad preview: {preview}")
if initial["outcome"] != "SUCCEEDED" or initial["summary"]["added"] != 2:
    raise SystemExit(f"bad initial run: {initial}")
summary = refresh["summary"]
if refresh["outcome"] != "SUCCEEDED" or (summary["added"], summary["updated"], summary["removed"]) != (1, 1, 1):
    raise SystemExit(f"bad refresh: {refresh}")
if len(history) < 3:
    raise SystemExit(f"missing run history: {history}")
prov = json.loads((root / "provenance.json").read_text())
if "file://agent/services.json" not in json.dumps(prov):
    raise SystemExit(f"missing source provenance: {prov}")
print("connector agent e2e passed")
PY

printf 'EVIDENCE=%s\nKC_URL=%s\nHOST_URL=%s\n' "${evidence_dir}" "${kc_url}" "${host_url}"
if [[ "${CONNECTOR_E2E_KEEP_SERVICES:-0}" == "1" ]]; then
  printf 'KC_PID=%s\nHOST_PID=%s\n' "${kc_pid}" "${host_pid}"
  trap - EXIT
fi
