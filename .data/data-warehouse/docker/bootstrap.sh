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
    --auth local \
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

kc_bootstrap() {
  kc "$@" --server "$bootstrap_server" --as service:bootstrap
}

grant_rules_json() {
  kc_bootstrap admin grant list
}

has_grant() {
  local principal="$1" action="$2" catalog_id="${3:-}" repo_id="${4:-}" workspace_id="${5:-}"
  grant_rules_json | python3 -c '
import json, sys
principal, action, catalog_id, repo_id, workspace_id = sys.argv[1:6]
rules = json.load(sys.stdin).get("rules") or []
for rule in rules:
    if rule.get("principal") != principal:
        continue
    if action not in (rule.get("actions") or []):
        continue
    if (rule.get("catalog") or "") != catalog_id:
        continue
    if (rule.get("repo") or "") != repo_id:
        continue
    if (rule.get("workspace") or "") != workspace_id:
        continue
    raise SystemExit(0)
raise SystemExit(1)
' "$principal" "$action" "$catalog_id" "$repo_id" "$workspace_id"
}

ensure_grant() {
  local principal="$1" action="$2" catalog_id="${3:-}" repo_id="${4:-}" workspace_id="${5:-}"
  if has_grant "$principal" "$action" "$catalog_id" "$repo_id" "$workspace_id"; then
    return 0
  fi
  local args=(admin grant add --principal "$principal" --action "$action")
  [[ -n "$catalog_id" ]] && args+=(--catalog "$catalog_id")
  [[ -n "$repo_id" ]] && args+=(--repo "$repo_id")
  [[ -n "$workspace_id" ]] && args+=(--workspace "$workspace_id")
  kc_bootstrap "${args[@]}" >/dev/null
}

revoke_action() {
  local principal="$1" action="$2"
  local ids
  ids="$(grant_rules_json | python3 -c '
import json, sys
principal, action = sys.argv[1], sys.argv[2]
for rule in json.load(sys.stdin).get("rules") or []:
    if rule.get("principal") == principal and action in (rule.get("actions") or []):
        print(rule["id"])
' "$principal" "$action")"
  local id
  for id in $ids; do
    [[ -n "$id" ]] || continue
    kc_bootstrap admin grant remove --id "$id" >/dev/null
  done
}

# Consumer discovery (catalog.read, schema browse --repo) is not implied by a
# workspace-scoped workspace.consume rule. Projection sync belongs to the
# governor identity, not agent:dsh.
ensure_consumer_policy() {
  ensure_grant agent:dsh catalog.read "$catalog"
  local action repository
  for action in workspace.consume workspace.resolve resource.access; do
    ensure_grant agent:dsh "$action" "$catalog" "" "$workspace"
  done
  for repository in "$physical" "$semantic"; do
    for action in knowledge.read knowledge.search knowledge.schema.read \
      knowledge.provenance knowledge.history.read file.read; do
      ensure_grant agent:dsh "$action" "" "$repository"
    done
  done
  revoke_action agent:dsh projection.manage
}

smoke() {
  local target_home="$1"
  kc local status --home "$target_home" >"$evidence/topology.json"
  jq -e --arg physical "$physical" --arg semantic "$semantic" '
    any(.repos[]; .id == $physical and .driver == "dolt") and
    any(.repos[]; .id == $semantic and .driver == "gitea")
  ' "$evidence/topology.json" >/dev/null

  kc_bootstrap operations projection sync --repo "$physical" --ref refs/heads/main \
    >"$evidence/physical-projection.json"
  kc_bootstrap operations projection sync --repo "$semantic" --ref refs/heads/main \
    >"$evidence/semantic-projection.json"

  kc catalog list --server "$bootstrap_server" --as agent:dsh \
    >"$evidence/catalog-list.json"
  jq -e --arg catalog "$catalog" 'any(.catalogs[]; .id == $catalog)' \
    "$evidence/catalog-list.json" >/dev/null
  kc catalog show --server "$bootstrap_server" --as agent:dsh --catalog "$catalog" \
    >"$evidence/catalog-show.json"
  jq -e --arg workspace "$workspace" 'any(.workspaces[]; .workspaceId == $workspace)' \
    "$evidence/catalog-show.json" >/dev/null
  kc knowledge schema browse --server "$bootstrap_server" --as agent:dsh \
    --repo "$physical" >"$evidence/schema-browse.json"
  jq -e '(.schemas | length > 0)' \
    "$evidence/schema-browse.json" >/dev/null

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
  ensure_consumer_policy
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

ensure_consumer_policy
smoke "$staging"
stop_bootstrap_server
jq -n --arg catalog "$catalog" --arg workspace "$workspace" \
  '{version:1,catalog:$catalog,workspace:$workspace,authorities:{physical:"dolt",semantic:"gitea"}}' \
  >"$staging/.compose-ready"
mv "$staging" "$home"
