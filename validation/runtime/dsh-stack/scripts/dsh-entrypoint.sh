#!/bin/sh
set -eu

: "${DSH_HOME:=/var/lib/dsh}"
export DSH_HOME
export KC_AUTH_TOKEN="$(cat /run/kc-secrets/agent.token)"
unset KC_AS

if [ ! -f "$DSH_HOME/profiles/web/package.json" ]; then
  mkdir -p "$DSH_HOME"
  cp -a /opt/dsh-home/. "$DSH_HOME/"
fi

if [ "${1:-}" = "serve" ]; then
  socat TCP-LISTEN:7400,fork,reuseaddr TCP:127.0.0.1:7401 &
  exec dsh web --host 127.0.0.1 --port 7401 --no-open
fi
exec "$@"
