#!/usr/bin/env bash
# Private deployment initialization only; business content must use Writer.
set -euo pipefail

lakefs_url=http://lakefs:8000
data_root=/var/lib/kc
access="${KC_ZK_LAKEFS_ACCESS_KEY:?}"
secret="${KC_ZK_LAKEFS_SECRET_KEY:?}"
export KC_LAKEFS_CREDENTIAL="${access}:${secret}"

response="$(mktemp)"
trap 'rm -f "$response"' EXIT
status="$(curl -sS -o "$response" -w '%{http_code}' -X POST \
  "${lakefs_url}/api/v1/setup_lakefs" -H 'Content-Type: application/json' \
  -d "{\"username\":\"zk-trial\",\"key\":{\"access_key_id\":\"${access}\",\"secret_access_key\":\"${secret}\"}}")"
case "$status" in 200|201|409) ;; *) echo "lakeFS setup failed: HTTP $status" >&2; exit 1 ;; esac

for repository in kc-catalog kc-system; do
  status="$(curl -sS -o "$response" -w '%{http_code}' -u "${access}:${secret}" \
    "${lakefs_url}/api/v1/repositories/${repository}")"
  if [[ "$status" == 200 ]]; then continue; fi
  [[ "$status" == 404 ]] || { echo "lakeFS repository check failed: HTTP $status" >&2; exit 1; }
  status="$(curl -sS -o "$response" -w '%{http_code}' -u "${access}:${secret}" -X POST \
    "${lakefs_url}/api/v1/repositories" -H 'Content-Type: application/json' \
    -d "{\"name\":\"${repository}\",\"storage_namespace\":\"s3://kc-authority/${repository}\",\"default_branch\":\"main\"}")"
  [[ "$status" == 201 ]] || { echo "lakeFS repository create failed: HTTP $status" >&2; exit 1; }
done

if [[ ! -f "$data_root/deployment.json" ]]; then
  fixture-deployment --root "$data_root" \
    --registry-id "${KC_ZK_REGISTRY_ID:-kr://zk-trial/catalog}" \
    --registry-driver lakefs --registry-dsn "${lakefs_url}/kc-catalog" \
    --principal "${KC_ZK_ADMIN:-admin}" \
    --lakefs-repo "kr://kc/system=${lakefs_url}/kc-system" \
    --managed-lakefs-dsn "$lakefs_url" \
    --managed-lakefs-namespace s3://kc-authority/zk-trial \
    --managed-public-url http://kc-upstream:7380
fi
kc deployment init --config "$data_root/deployment.json"
kc deployment system publish --config "$data_root/deployment.json"
