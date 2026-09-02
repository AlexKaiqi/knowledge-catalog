#!/usr/bin/env bash
set -euo pipefail

catalog=kr://dw/catalog
workspace=warehouse-agent
physical=kr://dw/physical
semantic=kr://dw/semantic
data_root=/var/lib/kc
home="$data_root/home"
staging="$data_root/staging"
evidence=/evidence
token_file=/run/kc-secrets/gitea.token
fixture=/opt/data-warehouse

mkdir -p "$data_root" "$evidence"
[[ -s "$token_file" ]] || { echo "Gitea token was not initialized" >&2; exit 1; }
export KC_GITEA_TOKEN="$(tr -d '\r\n' <"$token_file")"
export KC_RESOURCE_ACCESS_URL=http://resource-access:7390
export KC_MYSQL_HOST=mysql KC_MYSQL_PORT=3306 KC_MYSQL_USER=root
export KC_MYSQL_PASSWORD=dw-test-root KC_MYSQL_DATABASE=tpch

bootstrap_server=http://127.0.0.1:7381
server_pid=

start_bootstrap_server() {
  local target_home="$1"
  kc serve --home "$target_home" --listen 127.0.0.1:7381 \
    --resource-access-url http://resource-access:7390 \
    >"$evidence/bootstrap-server.log" 2>&1 &
  server_pid="$!"
  for _ in {1..60}; do
    if curl -fsS "$bootstrap_server/readyz" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$server_pid" >/dev/null 2>&1; then
      wait "$server_pid"
      return 1
    fi
    sleep 0.25
  done
  curl -fsS "$bootstrap_server/readyz" >/dev/null
}

stop_bootstrap_server() {
  kill "$server_pid"
  wait "$server_pid" || true
  server_pid=
}

smoke() {
  local target_home="$1"
  kc local status --home "$target_home" >"$evidence/topology.json"
  jq -e --arg physical "$physical" --arg semantic "$semantic" '
    any(.repos[]; .id == $physical and .driver == "dolt") and
    any(.repos[]; .id == $semantic and .driver == "gitea")
  ' "$evidence/topology.json" >/dev/null

  kc operations projection sync --server "$bootstrap_server" --as agent:dsh \
    --repo "$physical" --ref refs/heads/main \
    >"$evidence/physical-projection.json"
  kc operations projection sync --server "$bootstrap_server" --as agent:dsh \
    --repo "$semantic" --ref refs/heads/main \
    >"$evidence/semantic-projection.json"
  kc catalog workspace resolve --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" >"$evidence/pin.json"
  kc knowledge search --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" --query lineitem >"$evidence/search.json"
  jq -e '.hits | length > 0' "$evidence/search.json" >/dev/null
  kc resource access --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" \
    --object resource/mysql-tpch-sql --operation query \
    --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}' >"$evidence/resource.json"
  jq -e '.result.rows == ["1"] and .basis.runtimeGeneration == "mysql-tpch-fixture-v1"' \
    "$evidence/resource.json" >/dev/null
}

if [[ -f "$home/.compose-ready" ]]; then
  start_bootstrap_server "$home"
  smoke "$home"
  stop_bootstrap_server
  exit 0
fi
if [[ -e "$home" ]]; then
  echo "KC Compose home exists without a ready marker; run '.data/data-warehouse/dev.sh reset'" >&2
  exit 1
fi

rm -rf /var/lib/kc/staging
mkdir -p "$staging"

# A failed first bootstrap may have created only this fixed fixture repository.
# Reset it before rebuilding the staged KC home; a ready deployment never enters
# this branch.
curl -sS -o /dev/null -X DELETE \
  -H "Authorization: token ${KC_GITEA_TOKEN}" \
  http://gitea:3000/api/v1/repos/kc/kc-compose-semantic || true
curl -fsS -X POST \
  -H "Authorization: token ${KC_GITEA_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"kc-compose-semantic","private":true,"auto_init":false,"default_branch":"main"}' \
  http://gitea:3000/api/v1/user/repos >/dev/null

kc local init --home "$staging" --catalog "$catalog"
kc local grant bootstrap --home "$staging" --principal service:bootstrap >/dev/null
kc local store set --home "$staging" --repository dolt --index opensearch
kc local store set --home "$staging" --driver opensearch --url http://opensearch:9200
kc local repository attach --home "$staging" --catalog "$catalog" \
  --repo "$physical" --driver dolt
kc local repository attach --home "$staging" --catalog "$catalog" \
  --repo "$semantic" --driver gitea \
  --dsn http://gitea:3000/kc/kc-compose-semantic

# Only kc local and kc serve open Home directly. All product setup and
# validation below crosses the same service boundary used after bootstrap.
start_bootstrap_server "$staging"

kc writer ingest --server "$bootstrap_server" --as service:bootstrap --repo "$physical" \
  --dir "$fixture/knowledge/schemas/physical" >"$evidence/physical-schema.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "data-warehouse-domain-model",
  sourceRefs: ["knowledge://data-warehouse/physical-aspects/v1"]
}' "$evidence/physical-schema.ingest.json" >"$evidence/physical-schema.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-physical-schema \
  --changeset "$evidence/physical-schema.changeset.json" >"$evidence/physical-schema.receipt.json"

kc writer ingest --server "$bootstrap_server" --as service:bootstrap --repo "$physical" \
  --dir "$fixture/knowledge/physical" >"$evidence/physical-resource.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "data-warehouse-domain-model",
  sourceRefs: ["knowledge://data-warehouse/physical-aspects/v1"]
}' "$evidence/physical-resource.ingest.json" >"$evidence/physical-resource.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-physical-resource \
  --changeset "$evidence/physical-resource.changeset.json" >"$evidence/physical-resource.receipt.json"

base_commit="$(jq -r '.result.commitId' "$evidence/physical-resource.receipt.json")"
printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' \
  | python3 "$fixture/connector/collector.py" >"$evidence/mysql.observation.json"
connector-preview \
  --manifest "$fixture/connector/connector.yaml" \
  --observation "$evidence/mysql.observation.json" \
  --base "$base_commit" \
  --out "$evidence/mysql.preview.json"
jq '.changeSet' "$evidence/mysql.preview.json" >"$evidence/mysql.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-mysql-bootstrap \
  --changeset "$evidence/mysql.changeset.json" >"$evidence/mysql.receipt.json"

kc writer ingest --server "$bootstrap_server" --as service:bootstrap --repo "$semantic" \
  --dir "$fixture/knowledge/schemas/semantic" >"$evidence/semantic-schema.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "semantic-sales",
  sourceRefs: ["knowledge://finance/tpch-sales"]
}' "$evidence/semantic-schema.ingest.json" >"$evidence/semantic-schema.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-semantic-schema \
  --changeset "$evidence/semantic-schema.changeset.json" >"$evidence/semantic-schema.receipt.json"

kc writer ingest --server "$bootstrap_server" --as service:bootstrap --repo "$semantic" \
  --dir "$fixture/knowledge/semantic" >"$evidence/semantic.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "semantic-sales",
  sourceRefs: ["knowledge://finance/tpch-sales"]
}' "$evidence/semantic.ingest.json" >"$evidence/semantic.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-semantic-bootstrap \
  --changeset "$evidence/semantic.changeset.json" >"$evidence/semantic.receipt.json"

kc catalog workspace define --server "$bootstrap_server" --as service:bootstrap \
  --catalog "$catalog" \
  --workspace "$workspace" --revision 1 \
  --source "$physical=refs/heads/main@knowledge/physical" \
  --source "$semantic=refs/heads/main@knowledge/semantic"

for action in workspace.consume workspace.resolve resource.access; do
  kc admin grant add --server "$bootstrap_server" --as service:bootstrap \
    --principal agent:dsh --action "$action" \
    --catalog "$catalog" --workspace "$workspace" >/dev/null
done
for repository in "$physical" "$semantic"; do
  for action in knowledge.read knowledge.search file.read knowledge.provenance knowledge.history.read projection.manage; do
    kc admin grant add --server "$bootstrap_server" --as service:bootstrap \
      --principal agent:dsh --action "$action" \
      --repo "$repository" >/dev/null
  done
done

smoke "$staging"
stop_bootstrap_server
jq -n --arg catalog "$catalog" --arg workspace "$workspace" \
  '{version:1,catalog:$catalog,workspace:$workspace,authorities:{physical:"dolt",semantic:"gitea"}}' \
  >"$staging/.compose-ready"
mv "$staging" "$home"
