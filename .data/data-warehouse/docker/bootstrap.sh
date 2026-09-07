#!/usr/bin/env bash
set -euo pipefail

catalog=kr://dw/catalog
workspace=warehouse-agent
physical=kr://dw/physical
semantic=kr://dw/semantic
data_root=/var/lib/kc
deployment_config="$data_root/deployment.json"
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
  kc serve --config "$deployment_config" --listen 127.0.0.1:7381 \
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

# Consumer discovery (catalog.read, schema list --repo) is not implied by a
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
  kc deployment status --config "$deployment_config" >"$evidence/deployment-status.json"
  jq -e '.status == "ready"' "$evidence/deployment-status.json" >/dev/null
  cp "$deployment_config" "$evidence/topology.json"
  jq -e --arg physical "$physical" --arg semantic "$semantic" '
    any(.repositories[]; .id == $physical and .driver == "dolt") and
    any(.repositories[]; .id == $semantic and .driver == "gitea")
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
  kc knowledge schema list --server "$bootstrap_server" --as agent:dsh \
    --repo "$physical" >"$evidence/schema-browse.json"
  jq -e '(.schemas | length > 0)' \
    "$evidence/schema-browse.json" >/dev/null

  kc workspace pin --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" >"$evidence/pin.json"
  kc knowledge search --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" --query lineitem >"$evidence/search.json"
  jq -e '.hits | length > 0' "$evidence/search.json" >/dev/null
  kc knowledge invoke --server "$bootstrap_server" --as agent:dsh \
    --catalog "$catalog" --workspace "$workspace" \
    --object resource/mysql-tpch-sql --operation query \
    --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}' >"$evidence/resource.json"
  jq -e '.result.rows == ["1"] and .basis.runtimeGeneration == "mysql-tpch-fixture-v1"' \
    "$evidence/resource.json" >/dev/null
}

if [[ -f "$data_root/.compose-ready" ]]; then
  start_bootstrap_server
  smoke
  stop_bootstrap_server
  exit 0
fi
if [[ -e "$deployment_config" || -e "$data_root/home" || -e "$data_root/authority.git" || -e "$data_root/durable" ]]; then
  echo "KC Compose has an incomplete or legacy deployment; restore it or explicitly run '.data/data-warehouse/dev.sh reset'" >&2
  exit 1
fi

# The fixture supplies an already-published ordinary Git source; Catalog attach
# only validates it. Source provisioning is outside the kc product surface.
curl -fsS -X POST \
  -H "Authorization: token ${KC_GITEA_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"kc-compose-semantic","private":true,"auto_init":true,"default_branch":"main"}' \
  http://gitea:3000/api/v1/user/repos >/dev/null

fixture-deployment --root "$data_root" --catalog "$catalog" --principal service:bootstrap \
  --repo "$physical=$data_root/sources/physical" \
  --gitea-repo "$semantic=http://gitea:3000/kc/kc-compose-semantic" \
  --opensearch http://opensearch:9200 >/dev/null
kc deployment init --config "$deployment_config"

# All publication and Catalog admission below uses the formal Server boundary.
start_bootstrap_server
kc catalog repo attach --server "$bootstrap_server" --as service:bootstrap \
  --catalog "$catalog" --repo "$physical"
kc catalog repo attach --server "$bootstrap_server" --as service:bootstrap \
  --catalog "$catalog" --repo "$semantic"

kc pack --server "$bootstrap_server" --as service:bootstrap --repo "$physical" \
  --dir "$fixture/knowledge/schemas/physical" >"$evidence/physical-schema.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "data-warehouse-domain-model",
  sourceRefs: ["knowledge://data-warehouse/physical-aspects/v1"]
}' "$evidence/physical-schema.ingest.json" >"$evidence/physical-schema.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-physical-schema \
  --changeset "$evidence/physical-schema.changeset.json" >"$evidence/physical-schema.receipt.json"

kc pack --server "$bootstrap_server" --as service:bootstrap --repo "$physical" \
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

kc pack --server "$bootstrap_server" --as service:bootstrap --repo "$semantic" \
  --dir "$fixture/knowledge/schemas/semantic" >"$evidence/semantic-schema.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "semantic-sales",
  sourceRefs: ["knowledge://finance/tpch-sales"]
}' "$evidence/semantic-schema.ingest.json" >"$evidence/semantic-schema.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-semantic-schema \
  --changeset "$evidence/semantic-schema.changeset.json" >"$evidence/semantic-schema.receipt.json"

kc pack --server "$bootstrap_server" --as service:bootstrap --repo "$semantic" \
  --dir "$fixture/knowledge/semantic" >"$evidence/semantic.ingest.json"
jq '.changeSet | .provenance = {
  originKind: "DEFINITION",
  actorRef: "semantic-sales",
  sourceRefs: ["knowledge://finance/tpch-sales"]
}' "$evidence/semantic.ingest.json" >"$evidence/semantic.changeset.json"
kc writer commit --server "$bootstrap_server" --as service:bootstrap \
  --command-id compose-semantic-bootstrap \
  --changeset "$evidence/semantic.changeset.json" >"$evidence/semantic.receipt.json"

kc workspace define --server "$bootstrap_server" --as service:bootstrap \
  --catalog "$catalog" \
  --workspace "$workspace" --revision 1 \
  --source "$physical=refs/heads/main@knowledge/physical" \
  --source "$semantic=refs/heads/main@knowledge/semantic"

ensure_consumer_policy
smoke
stop_bootstrap_server
jq -n --arg catalog "$catalog" --arg workspace "$workspace" \
  '{version:1,catalog:$catalog,workspace:$workspace,authorities:{physical:"dolt",semantic:"gitea"}}' \
  >"$data_root/.compose-ready"
