#!/usr/bin/env bash
set -euo pipefail

command -v kc >/dev/null
command -v ttyd >/dev/null
command -v kcfs >/dev/null
unset KC_AS
export HOME="${HOME:-/tmp}/kc-compose-cli-smoke"
mkdir -p "$HOME" /tmp/kc-compose-project

if kc catalog workspace resolve >/dev/null 2>&1; then
  echo "kc must require login before catalog commands" >&2
  exit 1
fi

kc login --mode local --as agent:dsh | jq -e '
  .status == "authenticated"
  and .principal == "agent:dsh"
  and .mode == "local"
' >/dev/null
kc identity whoami | jq -e '.principal == "agent:dsh"' >/dev/null
kc catalog list | jq -e 'any(.catalogs[]; .id == "kr://dw/catalog")' >/dev/null
kc catalog show | jq -e '
  .catalogId == "kr://dw/catalog"
  and any(.workspaces[]; .workspaceId == "warehouse-agent")
' >/dev/null
kc catalog workspace list | jq -e 'any(.workspaces[]; .workspaceId == "warehouse-agent")' >/dev/null
kc catalog repository list | jq -e '
  (.repositories | index("kr://dw/physical"))
  and (.repositories | index("kr://dw/semantic"))
' >/dev/null
kc knowledge schema browse --repo kr://dw/physical | jq -e '
  .repository == "kr://dw/physical" and (.schemas | length > 0)
' >/dev/null
kc catalog workspace resolve | jq -e '
  .workspaceId == "warehouse-agent"
  and (.repositories["kr://dw/physical"] | type == "string")
  and (.repositories["kr://dw/semantic"] | type == "string")
' >/dev/null

search="$(kc knowledge search --query lineitem)"
jq -e '.hits | length > 0' <<<"$search" >/dev/null

read_result="$(kc knowledge read --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068)"
jq -e 'if type == "array" then .[0] else . end | .value.properties.name == "lineitem"' \
  <<<"$read_result" >/dev/null

kc knowledge provenance --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 >/dev/null
kc knowledge log --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 >/dev/null
kc knowledge relations --object kc://dw/physical/dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 | jq -e '
  .hits | type == "array"
' >/dev/null

resource="$(kc resource access \
  --object resource/mysql-tpch-sql \
  --operation query \
  --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}')"
jq -e '.result.rows == ["1"] and .basis.runtimeGeneration == "mysql-tpch-fixture-v1"' \
  <<<"$resource" >/dev/null

kcfs plan --server "${KC_SERVER_URL:?}" --as agent:dsh \
  --catalog kr://dw/catalog --workspace warehouse-agent \
  --view semantic --root /tmp/kc-compose-project | jq -e '
  .workspaceId == "warehouse-agent"
  and any(.mounts[]; .path == "knowledge/physical")
  and any(.mounts[]; .path == "knowledge/semantic")
' >/dev/null

kc login --mode local --as service:bootstrap | jq -e '
  .status == "authenticated" and .principal == "service:bootstrap"
' >/dev/null
kc identity whoami | jq -e '.principal == "service:bootstrap"' >/dev/null
kc writer head --repo kr://dw/physical | jq -e '.commit | type == "string"' >/dev/null
kc writer ingest --repo kr://dw/semantic \
  --dir /opt/data-warehouse/knowledge/semantic \
  --out /tmp/kc-compose-semantic-preview.changeset.json | jq -e '
  .diagnostics.files > 0
' >/dev/null
test -s /tmp/kc-compose-semantic-preview.changeset.json
kc knowledge read --repo kr://dw/physical \
  --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 | jq -e '
  if type == "array" then .[0] else . end | .value.properties.name == "lineitem"
' >/dev/null
kc catalog show | jq -e '.catalogId == "kr://dw/catalog"' >/dev/null
kc catalog audit | jq -e '.entries | type == "array"' >/dev/null
kc admin grant list | jq -e 'any(.rules[]; .principal == "agent:dsh")' >/dev/null
kc operations projection describe --repo kr://dw/physical >/dev/null
kc operations access describe | jq -e '
  .workspaceId == "warehouse-agent" and (.specs | length >= 2)
' >/dev/null
kc operations hook list >/dev/null
kc operations gate list >/dev/null

jq -n --argjson searchHits "$(jq '.hits | length' <<<"$search")" \
  '{ok:true,surface:"cli",searchHits:$searchHits,roles:["consumer","provider","governor"]}'
