#!/usr/bin/env bash
# Live Taihu authentication (not authorization). Browser PAR/PKCE, then
# introspection whoami against a local --auth taihu Server.
#
#   export KC_SERVICE_CLIENT_SECRET   # Taihu app secret for introspection
#   ./scripts/live-taihu-auth.sh        # prints auth URL, then waits
#   KC_LIVE_TAIHU=1 KC_AUTH_TOKEN=… go test -count=1 -run TestLiveTaihuAuthentication ./cli
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ -f "${HOME}/.config/kc/taihu.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${HOME}/.config/kc/taihu.env"
  set +a
fi

if [[ -z "${KC_SERVICE_CLIENT_SECRET:-}" ]]; then
  printf 'export KC_SERVICE_CLIENT_SECRET (Taihu application secret for introspection)\n' >&2
  printf 'optional: %s/.config/kc/taihu.env (gitignored)\n' "${HOME}" >&2
  exit 1
fi

go_bin="${GO:-go}"
listen="${KC_LIVE_TAIHU_LISTEN:-127.0.0.1:7382}"
home="${KC_LIVE_TAIHU_HOME:-/tmp/kc-taihu-live}"
auth_url="${KC_TAIHU_AUTH_URL:-http://iam.it.woa.com}"
client_id="${KC_TAIHU_CLIENT_ID:-knowledge-catalog}"
server_url="http://${listen}"
bin="${TMPDIR:-/tmp}/kc-taihu-live-bin"

"${go_bin}" build -o "${bin}" ./cmd/kc
mkdir -p "${home}"
if [[ ! -e "${home}/allow.json" && ! -e "${home}/layout.yaml" ]]; then
  "${bin}" -- --home "${home}" local init --catalog kr://taihu/live >/dev/null
fi

printf 'starting kc serve --auth taihu on %s\n' "${listen}" >&2
"${bin}" -- serve \
  --home "${home}" \
  --listen "${listen}" \
  --auth taihu \
  --auth-url "${auth_url}" \
  --service-client-id "${client_id}" &
serve_pid=$!
cleanup() { kill "${serve_pid}" 2>/dev/null || true; }
trap cleanup EXIT

auth_json=""
for _ in $(seq 1 50); do
  if auth_json="$(curl -fsS --max-time 1 "${server_url}/identity/v1/auth" 2>/dev/null)"; then
    break
  fi
  sleep 0.2
done
printf '%s\n' "${auth_json}"
python3 - "${auth_json}" <<'PY'
import json, sys
body = json.loads(sys.argv[1])
if body.get("mode") != "taihu" or body.get("localAssertion") is not False:
    raise SystemExit("server is not --auth taihu: %s" % body)
PY

export KC_SERVER_URL="${server_url}"
unset KC_AS || true
rm -f "${HOME}/.config/kc/pending-taihu-auth.json"

"${bin}" -- login --server "${server_url}"
printf '\nauthorize in the browser, this process will wait up to 5 minutes\n' >&2
"${bin}" -- login --wait --server "${server_url}"
"${bin}" -- --server "${server_url}" identity whoami
printf '\nnegative pairing checks\n' >&2
as_status="$(curl -sS -o /tmp/kc-taihu-whoami-as.json -w '%{http_code}' -H 'X-Kc-As: agent:dsh' "${server_url}/identity/v1/whoami" || true)"
rm -f /tmp/kc-taihu-whoami-as.json
if [[ "${as_status}" != "401" ]]; then
  printf 'X-Kc-As against Taihu must be 401, got %s\n' "${as_status}" >&2
  exit 1
fi
printf 'live Taihu authentication passed\n' >&2
