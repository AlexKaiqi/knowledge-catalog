#!/usr/bin/env bash
set -euo pipefail

token_file=/run/kc-secrets/gitea.token
[[ -s "$token_file" ]] || { echo "Gitea token was not initialized" >&2; exit 1; }
export KC_GITEA_TOKEN="$(tr -d '\r\n' <"$token_file")"
exec kc serve \
  --home /var/lib/kc/home \
  --listen 0.0.0.0:7380 \
  --auth local \
  --resource-access-url http://resource-access:7390
