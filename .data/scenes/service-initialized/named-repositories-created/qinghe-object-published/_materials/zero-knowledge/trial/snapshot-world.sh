#!/usr/bin/env bash
# Operator-only isolated Snapshot/Object world. Never resets another deployment.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
export KC_ZK_PROJECT="${KC_ZK_PROJECT:-kc-zk-snapshot}"
export KC_ZK_NETWORK="${KC_ZK_NETWORK:-${KC_ZK_PROJECT}_default}"
export KC_ZK_KC_IMAGE="${KC_ZK_KC_IMAGE:-kc-zk-snapshot:local}"
admin="${KC_ZK_ADMIN:-admin}"
supplier="${KC_ZK_SUPPLIER:-zk-supplier}"
colleague="${KC_ZK_COLLEAGUE:-zk-colleague}"
dataset="${KC_ZK_SHARED_DATASET:-qinghe-shared}"
table_repository="${KC_ZK_TABLE_REPOSITORY:-table-meta}"
semantic_repository="${KC_ZK_SEMANTIC_REPOSITORY:-sales-semantic}"

case "$KC_ZK_PROJECT" in
  kc-zk-*) ;;
  *) echo 'FAIL: trial project must start with kc-zk-' >&2; exit 2 ;;
esac
[[ "$KC_ZK_NETWORK" == "${KC_ZK_PROJECT}_default" ]] || {
  echo 'FAIL: trial network must belong to its own project (<project>_default)' >&2; exit 2;
}

compose() {
  docker compose --project-name "$KC_ZK_PROJECT" -f "$HERE/compose.snapshot.yaml" "$@"
}

kc_as() {
  local principal="$1"
  shift
  docker run --rm --network "$KC_ZK_NETWORK" \
    -e KC_SERVER_URL=http://kc-upstream:7380 -e "KC_AS=$principal" \
    --entrypoint /usr/local/bin/kc "$KC_ZK_KC_IMAGE" "$@"
}

kc() { kc_as "$admin" "$@"; }

ensure_repository() {
  local repository="$1"
  local inventory
  inventory="$(kc show)"
  if python3 -c 'import json,sys; p=json.load(sys.stdin); sys.exit(0 if any(r["id"] == sys.argv[1] for r in p["repositories"]) else 1)' "$repository" <<<"$inventory"; then
    return
  fi
  kc create --name "$repository" --store lakefs
  kc attach --repo "$repository"
}

ensure_grant() {
  local principal="$1" actions="$2" repository="${3:-}" scope="${4:-}" ref="${5:-}"
  local args=(grant add --principal "$principal" --action "$actions")
  [[ -z "$repository" ]] || args+=(--repo "$repository")
  [[ -z "$scope" ]] || args+=(--dataset "$scope")
  [[ -z "$ref" ]] || args+=(--ref "$ref")
  local grants
  grants="$(kc grant list --principal "$principal")"
  if python3 -c '
import json, sys
principal, actions, repo, dataset, ref = sys.argv[1:]
rules = json.load(sys.stdin)["rules"]
found = any(r["principal"] == principal and set(r.get("actions", [])) == set(actions.split(","))
            and r.get("repo", "") == repo and r.get("dataset", "") == dataset
            and r.get("ref", "") == ref and not r.get("object") and not r.get("aspect") for r in rules)
sys.exit(0 if found else 1)
' "$principal" "$actions" "$repository" "$scope" "$ref" <<<"$grants"; then
    return
  fi
  kc "${args[@]}"
}

cmd_setup() {
  kc whoami
  for repository in "$table_repository" "$semantic_repository"; do
    ensure_repository "$repository"
    ensure_grant "$supplier" 'object.read,object.schema.read,object.history.read,object.provenance,writer.preview,writer.commit,writer.branch,writer.merge,writer.receipt.read' "$repository"
    ensure_grant "$supplier" repository.dataset.publish "$repository" "$dataset" refs/heads/main
  done
  ensure_grant "$supplier" inventory.read
  ensure_grant "$supplier" dataset.manage,dataset.resolve,file.read,admin.grants.manage '' "$dataset"
  ensure_grant "$colleague" inventory.read
  kc show
  echo "Snapshot/Object world ready: $KC_ZK_PROJECT; publisher $supplier; colleague $colleague; intended delivery $dataset"
}

case "${1:-}" in
  up)
    docker image inspect "$KC_ZK_KC_IMAGE" >/dev/null
    compose up --detach --wait
    ;;
  setup) cmd_setup ;;
  check)
    compose ps
    kc whoami
    kc show
    kc_as "$colleague" show
    ;;
  down)
    compose down --volumes --remove-orphans
    ;;
  config) compose config --quiet ;;
  kc)
    shift
    kc_as "${KC_ZK_PRINCIPAL:-$admin}" "$@"
    ;;
  *) echo "usage: $0 up|setup|check|down|config|kc <arguments...>" >&2; exit 2 ;;
esac
