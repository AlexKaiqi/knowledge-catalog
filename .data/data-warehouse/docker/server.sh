#!/usr/bin/env bash
set -euo pipefail

token_file=/run/kc-secrets/gitea.token
[[ -s "$token_file" ]] || { echo "Gitea token was not initialized" >&2; exit 1; }
export KC_GITEA_TOKEN="$(tr -d '\r\n' <"$token_file")"
exec kc serve \
  --config /var/lib/kc/deployment.json \
  --listen 0.0.0.0:7380 \
  --resource-access-url http://resource-access:7390
