#!/usr/bin/env bash
set -euo pipefail

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

rm -rf /workspace/smoke
mkdir -p /workspace/smoke
manifest="$(kcfs daemon-mount \
  --server "$KC_SERVER_URL" \
  --catalog "$KC_CATALOG" \
  --workspace "$KC_WORKSPACE" \
  --root /workspace/smoke \
  --as "$KC_AS")"
pid="$(jq -r '.pid' <<<"$manifest")"
trap 'kcfs stop --pid "$pid" >/dev/null 2>&1 || true' EXIT
file="$(find /workspace/smoke/knowledge/semantic/objects/metrics -name properties.okf -print -quit)"
[[ -n "$file" ]]
grep -q '"name": "Gross merchandise value"' "$file"
kcfs stop --pid "$pid" >/dev/null
trap - EXIT

jq -n \
  --arg pin "$(jq -r '.pinId' <<<"$manifest")" \
  --arg customerCount "$(jq -r '.result.rows[0]' <<<"$resource")" \
  --argjson searchHits "$search_hits" \
  '{ok:true,pinId:$pin,searchHits:$searchHits,remoteFile:"Gross merchandise value",customerCount:$customerCount}'
