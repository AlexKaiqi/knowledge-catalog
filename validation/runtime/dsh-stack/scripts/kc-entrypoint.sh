#!/bin/sh
set -eu

: "${KC_HOME:=/var/lib/kc}"
: "${KC_GITEA_ORIGIN:=http://gitea:3000}"
: "${KC_ELASTICSEARCH_URL:=http://elasticsearch:9200}"
: "${KC_CATALOG:=kr://acme/catalog}"
: "${KC_WORKSPACE:=warehouse}"

export KC_GITEA_TOKEN="$(cat /run/kc-secrets/admin.token)"
admin_principal=$(cat /run/kc-secrets/admin.principal)
agent_principal=$(cat /run/kc-secrets/agent.principal)
marker="$KC_HOME/.compose-ready"
metadata=kr://acme/public/metadata
semantics=kr://acme/org/semantics
personal=kr://acme/personals/dsh-agent

run() {
  kc --home "$KC_HOME" "$@"
}

if [ ! -f "$marker" ]; then
  mkdir -p "$KC_HOME"
  if [ ! -f "$KC_HOME/layout.yaml" ]; then
    run init --catalog "$KC_CATALOG"
  fi
  run store-set --profile scale --repository gitea --index elasticsearch \
    --driver elasticsearch --url "$KC_ELASTICSEARCH_URL"

  run repo-add --repo "$metadata" --driver gitea --dsn "$KC_GITEA_ORIGIN/kc-admin/metadata"
  run repo-add --repo "$semantics" --driver gitea --dsn "$KC_GITEA_ORIGIN/kc-admin/semantics"
  run repo-add --repo "$personal" --driver gitea --dsn "$KC_GITEA_ORIGIN/kc-admin/personal"

  run put --command-id compose-schema-dataset --repo "$metadata" \
    --object schema/dataset.structure \
    --value '{"entity":"Dataset","aspect":"structure","pattern":"record","fields":{"name":{"type":"string","access":["text","filter"]},"description":{"type":"string","access":["text"]},"system":{"type":"string","access":["filter"]}}}'
  run put --command-id compose-seed-orders --repo "$metadata" \
    --object Dataset:orders --aspect structure --schema-ref schema/dataset.structure \
    --value '{"name":"orders","description":"TPC-H customer orders","system":"mysql"}'
  run put --command-id compose-schema-metric --repo "$semantics" \
    --object schema/metric.definition \
    --value '{"entity":"Metric","aspect":"definition","pattern":"record","fields":{"name":{"type":"string","access":["text","filter"]},"description":{"type":"string","access":["text"]},"owner":{"type":"string","access":["filter"]}}}'
  run put --command-id compose-seed-gmv --repo "$semantics" \
    --object Metric:gmv --aspect definition --schema-ref schema/metric.definition \
    --value '{"name":"gmv","description":"Gross merchandise value from lineitem extended price and discount","owner":"finance"}'
  run put --command-id compose-schema-note --repo "$personal" \
    --object schema/note.body \
    --value '{"entity":"Note","aspect":"body","pattern":"record","fields":{"title":{"type":"string","access":["text"]},"body":{"type":"string","access":["text"]}}}'
  run put --command-id compose-seed-note --repo "$personal" \
    --object Note:welcome --aspect body --schema-ref schema/note.body \
    --value '{"title":"DSH personal workspace","body":"Write personal notes below the personal mount."}'

  run define-workspace --workspace "$KC_WORKSPACE" --revision 1 \
    --source "$metadata=refs/heads/main@" \
    --source "$semantics=refs/heads/main@semantic" \
    --source "$personal=refs/heads/main@personal"
  run allow --principal "$agent_principal" --cmd read-workspace \
    --catalog "$KC_CATALOG" --workspace "$KC_WORKSPACE"
  for repo in "$metadata" "$semantics" "$personal"; do
    run allow --principal "$agent_principal" --cmd read --repo "$repo"
  done
  run allow --principal "$agent_principal" --cmd put,commit --repo "$personal"
  touch "$marker"
fi

# Projection state is disposable and may have been removed independently of the
# authority volumes. Reconcile every fixed authority head before serving.
for repo in "$metadata" "$semantics" "$personal"; do
  run index-sync --repo "$repo" --ref refs/heads/main >/dev/null
done

exec kc --home "$KC_HOME" serve --listen 0.0.0.0:7380 \
  --auth gitea --auth-url "$KC_GITEA_ORIGIN" --auth-admin "$admin_principal"
