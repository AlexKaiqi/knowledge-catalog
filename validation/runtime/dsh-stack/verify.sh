#!/usr/bin/env bash
set -euo pipefail

stack_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
compose=(docker compose -f "$stack_dir/compose.yaml")
kc_url="http://127.0.0.1:${KC_PORT:-17380}"
gitea_url="http://127.0.0.1:${GITEA_PORT:-13000}"
dsh_url="http://127.0.0.1:${DSH_PORT:-17400}"

"${compose[@]}" up --build -d --wait

admin_token=$("${compose[@]}" exec -T kc sh -c 'cat /run/kc-secrets/admin.token')
agent_token=$("${compose[@]}" exec -T kc sh -c 'cat /run/kc-secrets/agent.token')
auth=(-H "Authorization: Bearer $agent_token")

curl -fsS "$gitea_url/api/healthz" >/dev/null
for repo in metadata semantics personal; do
  curl -fsS -H "Authorization: token $admin_token" "$gitea_url/api/v1/repos/kc-admin/$repo" >/dev/null
done
curl -fsS "$kc_url/health" >/dev/null
curl -fsS "$dsh_url/" >/dev/null

post() {
  verb=$1
  body=$2
  curl -fsS -X POST "$kc_url/v1/$verb" "${auth[@]}" \
    -H 'Content-Type: application/json' -d "$body"
}

post whoami '{}' | grep -q 'gitea:'
post resolve '{"workspace":"warehouse"}' >/dev/null
post search '{"workspace":"warehouse","query":"orders"}' | grep -q 'Dataset:orders'
post search '{"workspace":"warehouse","query":"merchandise value"}' | grep -q 'Metric:gmv'
post vfs-list '{"workspace":"warehouse"}' | grep -q 'semantic/'
post vfs-write '{"workspace":"warehouse","command-id":"compose-agent-note","path":"personal/agent-note.md","content":"RFNIIGNvbXBvc2Ugc3RhY2sgaXMgcmVhZHkuCg=="}' >/dev/null
post vfs-read '{"workspace":"warehouse","path":"personal/agent-note.md"}' | grep -q 'RFNIIGNvbXBvc2Ugc3RhY2sgaXMgcmVhZHkuCg=='
post resolve '{"workspace":"warehouse"}' >"$stack_dir/.resolve.before.json"

"${compose[@]}" exec -T dsh dsh --profile web --dump-config >"$stack_dir/.dsh-config.yaml"
grep -q 'loom-control' "$stack_dir/.dsh-config.yaml"
grep -q 'loom-fs' "$stack_dir/.dsh-config.yaml"

"${compose[@]}" restart kc >/dev/null
for _ in $(seq 1 90); do
  if curl -fsS "$kc_url/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
post resolve '{"workspace":"warehouse"}' >"$stack_dir/.resolve.after.json"
cmp "$stack_dir/.resolve.before.json" "$stack_dir/.resolve.after.json"
post vfs-read '{"workspace":"warehouse","path":"personal/agent-note.md"}' | grep -q 'RFNIIGNvbXBvc2Ugc3RhY2sgaXMgcmVhZHkuCg=='

echo "PASS: Gitea authority, Elasticsearch projection, kc restart persistence, authenticated Workspace/VFS, and independent DSH profile"
