#!/bin/sh
set -eu

if [ "$(id -u)" = "0" ]; then
  mkdir -p /run/kc-secrets
  chown 1000:1000 /run/kc-secrets
  exec su-exec git /bin/sh "$0" --as-git
fi

config=/data/gitea/conf/app.ini
origin=http://gitea:3000

gitea admin user create --config "$config" \
  --username "$GITEA_ADMIN_USER" --password "$GITEA_ADMIN_PASSWORD" \
  --email kc-admin@example.invalid --admin --must-change-password=false >/dev/null 2>&1 || true
gitea admin user create --config "$config" \
  --username "$GITEA_AGENT_USER" --password "$GITEA_AGENT_PASSWORD" \
  --email dsh-agent@example.invalid --must-change-password=false >/dev/null 2>&1 || true

make_token() {
  user=$1
  file=$2
  if [ -s "$file" ] && curl -fsS -H "Authorization: token $(cat "$file")" "$origin/api/v1/user" >/dev/null; then
    return
  fi
  token=$(gitea admin user generate-access-token --config "$config" \
    --username "$user" --token-name "kc-compose-$(date +%s)" --scopes all --raw)
  umask 077
  printf '%s' "$token" >"$file"
}

make_token "$GITEA_ADMIN_USER" /run/kc-secrets/admin.token
make_token "$GITEA_AGENT_USER" /run/kc-secrets/agent.token

admin_token=$(cat /run/kc-secrets/admin.token)
create_repo() {
  name=$1
  code=$(curl -sS -o "/tmp/repo-$name.json" -w '%{http_code}' \
    -X POST "$origin/api/v1/user/repos" \
    -H "Authorization: token $admin_token" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"$name\",\"private\":true,\"auto_init\":false}")
  case "$code" in
    201|409|422) ;;
    *) cat "/tmp/repo-$name.json" >&2; exit 1 ;;
  esac
}

for repo in metadata semantics personal; do
  create_repo "$repo"
done

admin_json=$(curl -fsS -H "Authorization: token $admin_token" "$origin/api/v1/user")
agent_json=$(curl -fsS -H "Authorization: token $(cat /run/kc-secrets/agent.token)" "$origin/api/v1/user")
admin_id=$(printf '%s' "$admin_json" | grep -o '"id":[0-9]*' | head -n 1 | cut -d: -f2)
agent_id=$(printf '%s' "$agent_json" | grep -o '"id":[0-9]*' | head -n 1 | cut -d: -f2)
printf 'gitea:%s' "$admin_id" >/run/kc-secrets/admin.principal
printf 'gitea:%s' "$agent_id" >/run/kc-secrets/agent.principal
