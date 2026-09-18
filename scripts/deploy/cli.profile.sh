# Sourced by bash --login in the local deploy ttyd container.
if [[ -z "${KC_CLI_BANNER:-}" ]]; then
  export KC_CLI_BANNER=1
  cat <<EOF
Knowledge Catalog CLI (local deploy / ttyd)
  KC_SERVER_URL=${KC_SERVER_URL:-}
  KC_CATALOG=${KC_CATALOG:-}

This Server is --auth local. Login first; do not export KC_AS.
kc serve is a typed API (browser / is 404). This page is the CLI.

  kc login --mode local --as admin
  kc whoami
  kc catalog list
  kc schema list --repo kr://kc/system
  kc help consume|write|compose
EOF
fi
PS1='kc-deploy:\w\$ '
