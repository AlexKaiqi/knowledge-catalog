#!/bin/sh
set -eu

secret_dir=/run/kc-secrets
token_file="${secret_dir}/gitea.token"
install -d -m 0700 -o git -g git "$secret_dir"

if ! su-exec git gitea admin user list | awk 'NR > 1 {print $2}' | grep -qx kc; then
  su-exec git gitea admin user create \
    --username kc \
    --password kc-compose-password \
    --email kc-compose@example.invalid \
    --admin \
    --must-change-password=false
fi

if [ -s "$token_file" ]; then
  token="$(tr -d '\r\n' <"$token_file")"
  if curl -fsS -H "Authorization: token ${token}" http://gitea:3000/api/v1/user >/dev/null; then
    exit 0
  fi
fi

token_name="kc-compose-$(date +%s)"
su-exec git gitea admin user generate-access-token \
  --username kc --token-name "$token_name" --scopes all --raw >"$token_file"
chmod 0600 "$token_file"
