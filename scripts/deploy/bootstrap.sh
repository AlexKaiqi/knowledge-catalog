#!/usr/bin/env bash
set -euo pipefail

data_root=/var/lib/kc
deployment_config="${data_root}/deployment.json"
lakefs_url="${KC_LAKEFS_URL:-http://lakefs:8000}"
opensearch_url="${KC_OPENSEARCH_URL:-http://opensearch:9200}"
catalog="${KC_CATALOG:-kr://acme/catalog}"
principal="${KC_AS:-admin}"
access="${KC_LAKEFS_ACCESS_KEY:-AKIAKOSLOCALEXAMPLE}"
secret="${KC_LAKEFS_SECRET_KEY:-wJalrXUtnFEMILocalLakeFSSecret12}"
export KC_LAKEFS_CREDENTIAL="${access}:${secret}"

wait_http() {
  local url="$1"
  local deadline=$((SECONDS + 180))
  while (( SECONDS < deadline )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "FAIL: ${url} did not become ready" >&2
  return 1
}

lakefs_basic() {
  printf '%s' "${access}:${secret}"
}

setup_lakefs() {
  local status
  status="$(curl -sS -o /tmp/kc-lakefs-setup.json -w '%{http_code}' \
    -X POST "${lakefs_url}/api/v1/setup_lakefs" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"kc\",\"key\":{\"access_key_id\":\"${access}\",\"secret_access_key\":\"${secret}\"}}")"
  case "$status" in
    200|201|409) return 0 ;;
  esac
  echo "FAIL: lakeFS setup returned HTTP ${status}" >&2
  cat /tmp/kc-lakefs-setup.json >&2 || true
  return 1
}

ensure_repo() {
  local name="$1"
  local namespace="$2"
  local status
  status="$(curl -sS -o /tmp/kc-lakefs-repo.json -w '%{http_code}' \
    -u "$(lakefs_basic)" "${lakefs_url}/api/v1/repositories/${name}")"
  if [[ "$status" == "200" ]]; then
    return 0
  fi
  status="$(curl -sS -o /tmp/kc-lakefs-repo.json -w '%{http_code}' \
    -u "$(lakefs_basic)" \
    -X POST "${lakefs_url}/api/v1/repositories" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${name}\",\"storage_namespace\":\"${namespace}\",\"default_branch\":\"main\"}")"
  case "$status" in
    201) return 0 ;;
  esac
  echo "FAIL: create lakeFS repository ${name} returned HTTP ${status}" >&2
  cat /tmp/kc-lakefs-repo.json >&2 || true
  return 1
}

mkdir -p "$data_root"
wait_http "${lakefs_url}/_health"
wait_http "${opensearch_url}/_cluster/health"
setup_lakefs
ensure_repo kc-catalog s3://kc-authority/kc-catalog
ensure_repo kc-system s3://kc-authority/kc-system

if [[ ! -f "$deployment_config" ]]; then
  fixture-deployment \
    --root "$data_root" \
    --catalog "$catalog" \
    --catalog-driver lakefs \
    --catalog-dsn "${lakefs_url}/kc-catalog" \
    --principal "$principal" \
    --opensearch "$opensearch_url" \
    --lakefs-repo "kr://kc/system=${lakefs_url}/kc-system" \
    --managed-lakefs-dsn "${lakefs_url}" \
    --managed-lakefs-namespace s3://kc-authority/tianqiong \
    --managed-public-url "${KC_PUBLIC_URL:-http://127.0.0.1:7381}"
fi

kc deployment init --config "$deployment_config"
kc deployment system publish --config "$deployment_config"
replay="$(kc deployment system publish --config "$deployment_config")"
if ! grep -Eq '"seeded"[[:space:]]*:[[:space:]]*false' <<<"$replay"; then
  echo "FAIL: second system publish must verify without rewriting" >&2
  printf '%s\n' "$replay" >&2
  exit 1
fi
