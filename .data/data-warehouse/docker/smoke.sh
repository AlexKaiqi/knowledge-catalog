#!/usr/bin/env bash
set -euo pipefail

[[ -s "$DSH_HOME/profile-build-id" ]]
/usr/local/bin/kc-compose-web-smoke http://127.0.0.1:7400

model_catalog="$(curl -fsS http://127.0.0.1:7400/dsh-multi-model-provider/catalog)"
jq -e '
  .languageFailures == [] and
  ([.languageModels[] | select(.provider == "lore-openai" and .model == "gpt-5.6-luna" and .status == "live")] | length == 1) and
  ([.languageModels[] | select(.provider == "lore-openai" and .model == "gpt-5.6-sol" and .status == "live")] | length == 1)
' <<<"$model_catalog" >/dev/null

search="$(kc knowledge search --query lineitem)"
jq -e '.hits | length > 0' <<<"$search" >/dev/null
search_hits="$(jq -r '.hits | length' <<<"$search")"

read_result="$(kc knowledge read --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068)"
jq -e 'if type == "array" then .[0] else . end | .value.properties.name == "lineitem"' \
  <<<"$read_result" >/dev/null

resource="$(kc resource access \
  --object resource/mysql-tpch-sql \
  --operation query \
  --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}')"
jq -e '.result.rows == ["1"] and .basis.runtimeGeneration == "mysql-tpch-fixture-v1"' \
  <<<"$resource" >/dev/null

smoke_root="$(mktemp -d /workspace/kc-smoke.XXXXXX)"
mount_pid=""
cleanup_mount() {
  if [[ -n "$mount_pid" ]]; then
    kcfs stop --pid "$mount_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$smoke_root"
}
trap cleanup_mount EXIT
manifest="$(kcfs daemon-mount \
  --server "$KC_SERVER_URL" \
  --catalog "$KC_CATALOG" \
  --workspace "$KC_WORKSPACE" \
  --root "$smoke_root" \
  --as "$KC_AS")"
mount_pid="$(jq -r '.pid' <<<"$manifest")"
remote_file=""
for _attempt in $(seq 1 60); do
  candidate="$(find "$smoke_root/knowledge/semantic/objects/metrics" -name properties.okf -print -quit 2>/dev/null || true)"
  if [[ -n "$candidate" && -f "$candidate" ]] \
    && grep -q '"name": "Gross merchandise value"' "$candidate" 2>/dev/null; then
    remote_file="$candidate"
    break
  fi
  sleep 0.5
done
[[ -n "$remote_file" ]] || {
  echo "kcfs semantic metric did not become readable" >&2
  exit 1
}
kcfs stop --pid "$mount_pid" >/dev/null
mount_pid=""
rm -rf "$smoke_root"
trap - EXIT

jq -n \
  --arg pin "$(jq -r '.pinId' <<<"$manifest")" \
  --arg customerCount "$(jq -r '.result.rows[0]' <<<"$resource")" \
  --argjson searchHits "$search_hits" \
  '{ok:true,pinId:$pin,searchHits:$searchHits,remoteFile:"Gross merchandise value",customerCount:$customerCount}'
