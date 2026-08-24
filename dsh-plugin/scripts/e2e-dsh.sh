#!/usr/bin/env bash
# Drive a real DeepSeek Harness headless agent against the Workspace VFS.
# Requires: dsh on PATH with a configured DeepSeek credential. The model route
# is fixed by scripts/deepseek-official.patch.yml; kc is built by this script.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PLUGIN="$ROOT/dsh-plugin"
PROFILE_NAME="${DSH_PROFILE:-loom-e2e}"
TOPOLOGY="${KC_E2E_TOPOLOGY:-ordinary-git}"
PORT="${KC_E2E_PORT:-17380}"
KC_HOME="${KC_E2E_HOME:-$(mktemp -d /tmp/kc-loom-e2e.XXXXXX)}"
KC_BIN="${KC_BIN:-$(mktemp /tmp/kc-loom-e2e-bin.XXXXXX)}"
LISTEN="127.0.0.1:${PORT}"
BASE="http://${LISTEN}"
export PATH="${HOME}/.local/go/bin:${HOME}/.local/bin:${PATH}"

cleanup() {
  if [[ -n "${SERVE_PID:-}" ]]; then
    kill "${SERVE_PID}" 2>/dev/null || true
    wait "${SERVE_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

start_server() {
  echo "==> kc serve --home ${KC_HOME} --listen ${LISTEN}"
  "$KC_BIN" --home "$KC_HOME" serve --listen "$LISTEN" >>"$KC_HOME/serve.log" 2>&1 &
  SERVE_PID=$!
  for _ in $(seq 1 100); do
    if curl -sS -f "${BASE}/health" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "$SERVE_PID" 2>/dev/null; then
      echo "kc serve exited early:" >&2
      sed -n '1,240p' "$KC_HOME/serve.log" >&2
      exit 1
    fi
    sleep 0.1
  done
  echo "kc serve did not become healthy" >&2
  exit 1
}

stop_server() {
  if [[ -n "${SERVE_PID:-}" ]]; then
    kill "$SERVE_PID"
    wait "$SERVE_PID" 2>/dev/null || true
    SERVE_PID=""
  fi
}

echo "==> build kc"
go -C "$ROOT" build -o "$KC_BIN" ./cmd/kc

echo "==> build dsh-loom"
(cd "$PLUGIN" && npm install --legacy-peer-deps && npm run build)

post() {
  local verb="$1"
  local body="$2"
  curl -sS -f -X POST "${BASE}/v1/${verb}" \
    -H 'content-type: application/json' \
    -d "$body"
}

start_server

echo "==> seed workspace notes"
post init '{"catalog":"kr://acme/catalog"}' >/dev/null
post repo-add '{"repo":"kr://acme/personals/alice"}' >/dev/null
PLAIN_GIT="$(mktemp -d /tmp/kc-plain-git.XXXXXX)"
git -C "$PLAIN_GIT" init -q -b main
git -C "$PLAIN_GIT" -c user.name=fixture -c user.email=fixture@local commit -q --allow-empty -m root
post repo-add "{\"repo\":\"kr://acme/clone/reference\",\"driver\":\"filegit\",\"link\":\"${PLAIN_GIT}\"}" >/dev/null
case "$TOPOLOGY" in
  ordinary-git)
    post repo-add "{\"repo\":\"kr://acme/shared/semantic\",\"driver\":\"filegit\",\"dir\":\"${PLAIN_GIT}\"}" >/dev/null
    ;;
  gitea)
    : "${KC_GITEA_DSN:?KC_GITEA_DSN is required for the gitea topology}"
    : "${KC_GITEA_TOKEN:?KC_GITEA_TOKEN is required for the gitea topology}"
    post repo-add "{\"repo\":\"kr://acme/shared/semantic\",\"driver\":\"gitea\",\"dsn\":\"${KC_GITEA_DSN}\"}" >/dev/null
    ;;
  dolt)
    post repo-add '{"repo":"kr://acme/shared/semantic","driver":"dolt"}' >/dev/null
    ;;
  *)
    echo "unknown KC_E2E_TOPOLOGY: $TOPOLOGY" >&2
    exit 2
    ;;
esac
post define-workspace '{"workspace":"notes","revision":1,"source":["kr://acme/personals/alice=refs/heads/main@","kr://acme/shared/semantic=refs/heads/main@refs/semantic","kr://acme/clone/reference=refs/heads/main@refs/clone"]}' >/dev/null
if [[ "$TOPOLOGY" == "ordinary-git" ]]; then
  if git -C "$PLAIN_GIT" config --local --get kc.repositoryId >/dev/null 2>&1 || [[ -e "$PLAIN_GIT/streams" ]]; then
    echo "FAIL: attaching the ordinary Git repository polluted its config or tree" >&2
    exit 1
  fi
fi

echo "==> persist mount, grant, write receipt, and pin across process restart"
post allow '{"principal":"restart-reader","cmd":"read-workspace","catalog":"kr://acme/catalog","workspace":"notes"}' >/dev/null
post vfs-write '{"workspace":"notes","command-id":"restart-seed","path":"restart/probe.txt","content":"cGVyc2lzdGVkCg=="}' >/dev/null
post receipt '{"command-id":"restart-seed"}' >"$KC_HOME/receipt.before.json"
post resolve '{"workspace":"notes"}' >"$KC_HOME/pin.before.json"
stop_server
start_server
post receipt '{"command-id":"restart-seed"}' >"$KC_HOME/receipt.after.json"
post resolve '{"workspace":"notes"}' >"$KC_HOME/pin.after.json"
post allowed '{"principal":"restart-reader","cmd":"read-workspace","catalog":"kr://acme/catalog","workspace":"notes"}' >/dev/null
post vfs-read '{"workspace":"notes","path":"restart/probe.txt"}' >"$KC_HOME/restart-read.json"
post vfs-write '{"workspace":"notes","command-id":"restart-seed","path":"restart/probe.txt","content":"cGVyc2lzdGVkCg=="}' >"$KC_HOME/replay.json"
post resolve '{"workspace":"notes"}' >"$KC_HOME/pin.after-replay.json"
python3 - "$KC_HOME" "$BASE" "$TOPOLOGY" <<'PY'
import concurrent.futures
import json
import sys
import urllib.request
from pathlib import Path

home = Path(sys.argv[1])
base = sys.argv[2]
request_timeout = 180 if sys.argv[3] == "dolt" else 20
before = json.loads((home / "pin.before.json").read_text())
after = json.loads((home / "pin.after.json").read_text())
after_replay = json.loads((home / "pin.after-replay.json").read_text())
if before != after or before != after_replay:
    raise SystemExit(f"FAIL: restart changed Workspace pin: {before!r} != {after!r}")
if (home / "receipt.before.json").read_text() != (home / "receipt.after.json").read_text():
    raise SystemExit("FAIL: restart changed idempotency receipt")
read = json.loads((home / "restart-read.json").read_text())
if read.get("content") != "cGVyc2lzdGVkCg==":
    raise SystemExit(f"FAIL: persisted VFS content missing after restart: {read!r}")
replay = json.loads((home / "replay.json").read_text())
if replay.get("disposition") != "REPLAYED":
    raise SystemExit(f"FAIL: identical post-restart write was not replayed: {replay!r}")

def read_once(_: int) -> None:
    req = urllib.request.Request(
        base + "/v1/vfs-read",
        data=json.dumps({"workspace": "notes", "path": "restart/probe.txt"}).encode(),
        headers={"content-type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=request_timeout) as response:
        body = json.load(response)
    if body.get("content") != "cGVyc2lzdGVkCg==":
        raise AssertionError(body)

with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
    list(pool.map(read_once, range(48)))
print("restart: state, grant, receipt, pin, content and 48 concurrent reads passed")
PY

echo "==> dsh profile ${PROFILE_NAME}"
dsh plugin --profile "$PROFILE_NAME" add "file:${PLUGIN}"
PROFILE_DIR="${DSH_HOME:-$HOME/.dsh}/profiles/${PROFILE_NAME}"
python3 - "$PROFILE_DIR" <<'PY'
import json, sys
p = sys.argv[1] + "/package.json"
with open(p) as f:
    data = json.load(f)
bundles = data.setdefault("dsh", {}).setdefault("profile", {}).setdefault("bundles", [])
for name in ("@deepseek-ai/dsh-base", "@deepseek-ai/dsh-headless", "dsh-loom"):
    if name not in bundles:
        bundles.append(name)
with open(p, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
print("bundles:", bundles)
PY

echo "==> composed tree (must contain loom-fs, fs-sandbox disabled)"
KC_SERVE="$BASE" KC_WORKSPACE=notes dsh --profile "$PROFILE_NAME" --dump-config | tee /tmp/kc-loom-e2e-dump.yml >/dev/null
python3 - <<'PY'
from pathlib import Path
text = Path("/tmp/kc-loom-e2e-dump.yml").read_text()
if "loom-fs" not in text and "dsh-loom/fs" not in text:
    raise SystemExit("FAIL: composed tree has no loom-fs")
print("dump-config: loom-fs present")
PY

echo "==> publisher then consumer (real dsh agents, DeepSeek official)"
export KC_SERVE="$BASE" KC_WORKSPACE=notes DSH_PROFILE="$PROFILE_NAME"
python3 "$PLUGIN/scripts/e2e_publish_consume.py"
