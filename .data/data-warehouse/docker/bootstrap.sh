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

smoke() {
  local target_home="$1"
  kc local status --home "$target_home" >"$evidence/topology.json"
  jq -e --arg physical "$physical" --arg semantic "$semantic" '
    any(.repos[]; .id == $physical and .driver == "dolt") and
    any(.repos[]; .id == $semantic and .driver == "gitea")
  ' "$evidence/topology.json" >/dev/null
  kc operations projection sync --home "$target_home" --repo "$physical" --ref refs/heads/main \
    >"$evidence/physical-projection.json"
  kc operations projection sync --home "$target_home" --repo "$semantic" --ref refs/heads/main \
    >"$evidence/semantic-projection.json"
  kc catalog workspace resolve --home "$target_home" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" >"$evidence/pin.json"
  kc knowledge search --home "$target_home" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" --query lineitem >"$evidence/search.json"
  jq -e '.hits | length > 0' "$evidence/search.json" >/dev/null
  kc resource access --home "$target_home" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" \
    --object resource/mysql-tpch-sql --operation query \
    --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}' >"$evidence/resource.json"
  jq -e '.result.rows == ["1"] and .basis.runtimeGeneration == "mysql-tpch-fixture-v1"' \
    "$evidence/resource.json" >/dev/null
}

if [[ -f "$home/.compose-ready" ]]; then
  smoke "$home"
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
kc local store set --home "$staging" --repository dolt --index opensearch
kc local store set --home "$staging" --driver opensearch --url http://opensearch:9200
kc local repository attach --home "$staging" --catalog "$catalog" \
  --repo "$physical" --driver dolt
kc local repository attach --home "$staging" --catalog "$catalog" \
  --repo "$semantic" --driver gitea \
  --dsn http://gitea:3000/kc/kc-compose-semantic

kc writer ingest --home "$staging" --repo "$physical" \
  --dir "$fixture/knowledge/physical" \
  --out "$evidence/physical-schema.changeset.json" \
  --origin-kind DEFINITION --actor-ref data-warehouse-domain-model \
  --source-ref knowledge://data-warehouse/physical-aspects/v1
kc writer commit --home "$staging" --command-id compose-physical-schema \
  --changeset "$evidence/physical-schema.changeset.json" >"$evidence/physical-schema.receipt.json"

base_commit="$(jq -r '.result.commitId' "$evidence/physical-schema.receipt.json")"
printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' \
  | python3 "$fixture/connector/collector.py" >"$evidence/mysql.observation.json"
connector-preview \
  --manifest "$fixture/connector/connector.yaml" \
  --observation "$evidence/mysql.observation.json" \
  --base "$base_commit" \
  --out "$evidence/mysql.preview.json"
jq '.changeSet' "$evidence/mysql.preview.json" >"$evidence/mysql.changeset.json"
kc writer commit --home "$staging" --command-id compose-mysql-bootstrap \
  --changeset "$evidence/mysql.changeset.json" >"$evidence/mysql.receipt.json"

kc writer ingest --home "$staging" --repo "$semantic" \
  --dir "$fixture/knowledge/semantic" \
  --out "$evidence/semantic.changeset.json" \
  --origin-kind DEFINITION --actor-ref semantic-sales \
  --source-ref knowledge://finance/tpch-sales
kc writer commit --home "$staging" --command-id compose-semantic-bootstrap \
  --changeset "$evidence/semantic.changeset.json" >"$evidence/semantic.receipt.json"

kc catalog workspace define --home "$staging" --catalog "$catalog" \
  --workspace "$workspace" --revision 1 \
  --source "$physical=refs/heads/main@knowledge/physical" \
  --source "$semantic=refs/heads/main@knowledge/semantic"

for action in workspace.consume workspace.resolve resource.access; do
  kc admin grant add --home "$staging" --principal agent:dsh --action "$action" \
    --catalog "$catalog" --workspace "$workspace" >/dev/null
done
for repository in "$physical" "$semantic"; do
  for action in knowledge.read knowledge.search file.read knowledge.provenance knowledge.history.read; do
    kc admin grant add --home "$staging" --principal agent:dsh --action "$action" \
      --repo "$repository" >/dev/null
  done
done

smoke "$staging"
jq -n --arg catalog "$catalog" --arg workspace "$workspace" \
  '{version:1,catalog:$catalog,workspace:$workspace,authorities:{physical:"dolt",semantic:"gitea"}}' \
  >"$staging/.compose-ready"
mv "$staging" "$home"
