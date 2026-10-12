#!/usr/bin/env bash
set -euo pipefail

kcfs_script_dir="${BASH_SOURCE[0]%/*}"
if [[ "$kcfs_script_dir" == "${BASH_SOURCE[0]}" ]]; then
  kcfs_script_dir="."
fi
source "$kcfs_script_dir/kcfs-env.sh"
kcfs_require_lakefs_binding

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "FAIL: kcfs host mount smoke test requires Linux" >&2
  exit 1
fi
if [[ ! -e /dev/fuse ]]; then
  echo "FAIL: /dev/fuse is unavailable" >&2
  exit 1
fi
if ! command -v fusermount3 >/dev/null 2>&1 && ! command -v fusermount >/dev/null 2>&1; then
  echo "FAIL: install fuse3 (fusermount3)" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_root="$(mktemp -d)"
kc_pid=""
remote_pid=""
daemon_pid=""
daemon_log_dir="$run_root/daemon-tmp"
server_pid=""
cleanup() {
  if [[ -n "$daemon_pid" ]]; then
    TMPDIR="$daemon_log_dir" "$run_root/kcfs" stop --pid "$daemon_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$kc_pid" ]]; then
    kill -TERM "$kc_pid" >/dev/null 2>&1 || true
    wait "$kc_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$remote_pid" ]]; then
    kill -TERM "$remote_pid" >/dev/null 2>&1 || true
    wait "$remote_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$server_pid" ]]; then
    kill -TERM "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$run_root"
}
trap cleanup EXIT INT TERM

(cd "$repo_root" && go build -o "$run_root/kc" ./cmd/kc)
(cd "$repo_root" && go build -o "$run_root/kcfs" ./cmd/kcfs)

lakefs_url="${KC_LAKEFS_URL%/}"

# Presigned backing-store URLs are signed for the advertised loopback host, so
# they cannot be retargeted. When lakeFS is reached through the container
# gateway, tunnel the advertised loopback port to the real host instead.
if [[ "$lakefs_url" == *host.docker.internal* && -n "${KC_PRESIGN_TUNNEL:-}" ]]; then
  command -v socat >/dev/null 2>&1 || apt-get install -y -qq socat >/dev/null
  socat "TCP-LISTEN:${KC_PRESIGN_TUNNEL},fork,reuseaddr" "TCP:host.docker.internal:${KC_PRESIGN_TUNNEL}" &
  TUNNEL_PID=$!
  trap 'kill "$TUNNEL_PID" 2>/dev/null || true; cleanup' EXIT INT TERM
fi

home_dir="$run_root/home"
project_dir="$run_root/project"
mkdir -p "$project_dir" "$run_root/kc-config"
export KC_CONFIG_DIR="$run_root/kc-config"
printf 'local\n' >"$project_dir/LOCAL.txt"

lakefs_ensure_repo() {
  local name="$1" status
  status="$(curl -sS -o "$run_root/repo.json" -w '%{http_code}' -u "$KC_LAKEFS_CREDENTIAL" "$lakefs_url/api/v1/repositories/${name}")"
  if [[ "$status" == "200" ]]; then
    return 0
  fi
  status="$(curl -sS -o "$run_root/repo.json" -w '%{http_code}' -u "$KC_LAKEFS_CREDENTIAL" \
    -X POST "$lakefs_url/api/v1/repositories" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${name}\",\"storage_namespace\":\"s3://kc-authority/kcfs-${name}-${run_suffix}\",\"default_branch\":\"main\"}")"
  if [[ "$status" != "201" ]]; then
    echo "FAIL: create lakeFS repository ${name} returned HTTP $status" >&2
    cat "$run_root/repo.json" >&2 || true
    exit 1
  fi
}

# lakefs_put stages one object on main and commits it. This is the upstream
# tree-mutation path: the FUSE mount must stay frozen at its pinned commit.
lakefs_put() {
  # lakeFS 1.86 rejects direct object PUTs with a request body, so stage the
  # bytes through the presigned backing-store URL exactly like the Writer:
  # obtain a staging location, upload, then register checksum and size.
  local repo="$1" path="$2" content="$3" message="$4" enc staging upload_url etag size
  enc="$(python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=""))' "$path")"
  staging="$(curl -fsS -u "$KC_LAKEFS_CREDENTIAL" \
    "$lakefs_url/api/v1/repositories/${repo}/branches/main/staging/backing?path=${enc}&presign=true")" \
    || { echo "FAIL: staging GET ${repo}/${path}" >&2; exit 1; }
  upload_url="$(python3 -c 'import json,sys;print(json.load(sys.stdin)["presigned_url"])' <<<"$staging")"
  etag="$(curl -fsS -X PUT "$upload_url" --data-binary "$content" -D - -o /dev/null \
    | tr -d '\r' | awk 'tolower($1)=="etag:"{gsub(/"/,"",$2);print $2}')" \
    || { echo "FAIL: presigned PUT ${repo}/${path}" >&2; exit 1; }
  if [[ -z "$etag" ]]; then
    etag="$(printf '%s' "$content" | (sha256sum 2>/dev/null || shasum -a 256) | awk '{print $1}')"
  fi
  size="${#content}"
  curl -fsS -u "$KC_LAKEFS_CREDENTIAL" -X PUT \
    "$lakefs_url/api/v1/repositories/${repo}/branches/main/staging/backing?path=${enc}" \
    -H 'Content-Type: application/json' \
    -d "{\"staging\":${staging},\"checksum\":\"${etag}\",\"size_bytes\":${size}}" >/dev/null \
    || { echo "FAIL: staging confirm ${repo}/${path}" >&2; exit 1; }
  commit_status="$(curl -sS -o "$run_root/commit.json" -w '%{http_code}' -u "$KC_LAKEFS_CREDENTIAL" -X POST \
    "$lakefs_url/api/v1/repositories/${repo}/branches/main/commits" \
    -H 'Content-Type: application/json' -d "{\"message\":\"${message}\"}")"
  if [[ "$commit_status" != "201" ]]; then
    echo "FAIL: commit ${repo}/${path} returned HTTP $commit_status: $(head -c 200 "$run_root/commit.json")" >&2
    exit 1
  fi
}

# Storage namespaces are never reusable after a repository deletion, so each
# run gets a unique namespace under the shared bucket.
run_suffix="$(date +%s)$RANDOM"
lakefs_ensure_repo team
lakefs_ensure_repo policy
lakefs_ensure_repo kc-system
lakefs_put team docs/team/README.md 'team
' seed
lakefs_put team docs/runbooks/incident.md 'incident
' seed
lakefs_put policy knowledge/policy/rules.md 'policy
' seed

deployment_config="$(go -C "$repo_root" run ./scripts/fixture-deployment --root "$home_dir" --registry-id kr://test/registry --registry-dsn "${lakefs_url}/kc-catalog" --principal agent:test --lakefs-repo "kr://kc/system=${lakefs_url}/kc-system" --lakefs-repo "kr://test/team=${lakefs_url}/team" --lakefs-repo "kr://test/policy=${lakefs_url}/policy")"
"$run_root/kc" deployment init --config "$deployment_config" >/dev/null

server_port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
)"
server_url="http://127.0.0.1:$server_port"
"$run_root/kc" serve --config "$deployment_config" --listen "127.0.0.1:$server_port" >"$run_root/server.log" 2>&1 &
server_pid=$!
server_ready=0
for _ in $(seq 1 100); do
  if curl -fsS "$server_url/readyz/consumer" >/dev/null 2>&1; then
    server_ready=1
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    cat "$run_root/server.log" >&2
    exit 1
  fi
  sleep 0.05
done
if [[ "$server_ready" != "1" ]]; then
  echo "FAIL: kc service did not become consumer-ready" >&2
  cat "$run_root/server.log" >&2
  exit 1
fi

run_kc() {
  if ! "$run_root/kc" --server "$server_url" --as agent:test "$@" >"$run_root/kc.out" 2>"$run_root/kc.err"; then
    echo "FAIL: kc $*" >&2
    cat "$run_root/kc.out" >&2
    cat "$run_root/kc.err" >&2
    exit 1
  fi
}

advance_tree() {
  local repo="$1"
  local path="$2"
  local content="$3"
  local message="$4"
  lakefs_put "$repo" "$path" "$content" "$message"
}

run_kc attach --repo kr://test/team
run_kc attach --repo kr://test/policy

define_dataset() {
  local revision="$1"
  run_kc dataset define --dataset agent --revision "$revision" \
    --source 'kr://test/team=refs/heads/main@docs/team@team' \
    --source 'kr://test/team=refs/heads/main@docs/runbooks@runbooks' \
    --source 'kr://test/policy=refs/heads/main@knowledge/policy'
}

define_dataset 1

"$run_root/kcfs" plan --server "$server_url" --as agent:test --dataset agent --root "$project_dir" >"$run_root/plan.json"
python3 - "$run_root/plan.json" <<'PY'
import json, sys
plan = json.load(open(sys.argv[1]))
assert plan["setId"] == "agent"
assert plan["pinId"]
assert {m["path"] for m in plan["mounts"]} == {"docs/team", "docs/runbooks", "knowledge/policy"}
assert len({m["commit"] for m in plan["mounts"] if m["repository"] == "kr://test/team"}) == 1
PY

"$run_root/kcfs" mount --server "$server_url" --as agent:test --dataset agent --root "$project_dir" >"$run_root/mount.json" 2>"$run_root/kcfs.log" &
kc_pid=$!
team_file="$project_dir/docs/team/README.md"
runbook_file="$project_dir/docs/runbooks/incident.md"
policy_file="$project_dir/knowledge/policy/rules.md"
mounted=0
for _ in $(seq 1 200); do
  if [[ -f "$team_file" && -f "$runbook_file" && -f "$policy_file" ]]; then
    mounted=1
    break
  fi
  if ! kill -0 "$kc_pid" >/dev/null 2>&1; then
    cat "$run_root/kcfs.log" >&2
    exit 1
  fi
  sleep 0.05
done
if [[ "$mounted" != "1" ]]; then
  echo "FAIL: kcfs mount did not expose Dataset files" >&2
  find "$project_dir" -print >&2 || true
  cat "$run_root/plan.json" >&2
  cat "$run_root/kcfs.log" >&2
  exit 1
fi

assert_file_content() {
  local file="$1"
  local expected="$2"
  if [[ "$(cat "$file")" != "$expected" ]]; then
    echo "FAIL: unexpected content in $file" >&2
    cat "$file" >&2
    cat "$run_root/plan.json" >&2
    cat "$run_root/kcfs.log" >&2
    exit 1
  fi
}
assert_file_content "$team_file" team
assert_file_content "$runbook_file" incident
assert_file_content "$policy_file" policy
rg -q team "$project_dir/docs/team"
rg -q policy "$project_dir/knowledge/policy"
[[ "$(cat "$project_dir/LOCAL.txt")" == "local" ]]
advance_tree team docs/team/README.md 'advanced
' advance
assert_file_content "$team_file" team
if (printf 'mutated\n' >"$team_file") 2>/dev/null; then
  echo "FAIL: kcfs mount accepted a write" >&2
  exit 1
fi

kill -TERM "$kc_pid"
wait "$kc_pid"
kc_pid=""
[[ ! -e "$project_dir/docs/team" ]]
[[ ! -e "$project_dir/docs/runbooks" ]]
[[ ! -e "$project_dir/knowledge/policy" ]]
[[ "$(cat "$project_dir/LOCAL.txt")" == "local" ]]

"$run_root/kcfs" plan --server "$server_url" --as agent:test --dataset agent --root "$project_dir" >"$run_root/plan-still-frozen.json"
python3 - "$run_root/plan.json" "$run_root/plan-still-frozen.json" <<'PY'
import json, sys
first, later = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
assert first["pin"]["repositories"]["kr://test/team"] == later["pin"]["repositories"]["kr://test/team"]
assert first["pinId"] == later["pinId"]
PY
define_dataset 2

daemon_project="$run_root/daemon-project"
mkdir -p "$daemon_project" "$daemon_log_dir"
printf 'daemon-local\n' >"$daemon_project/LOCAL.txt"
TMPDIR="$daemon_log_dir" "$run_root/kcfs" daemon-mount --server "$server_url" \
  --dataset agent --root "$daemon_project" --as agent:test >"$run_root/daemon-mount.json"
daemon_pid="$(python3 - "$run_root/daemon-mount.json" <<'PY'
import json, sys
pid = json.load(open(sys.argv[1]))["pid"]
assert isinstance(pid, int) and pid > 1
print(pid)
PY
)"
python3 - "$run_root/daemon-mount.json" "$daemon_project" <<'PY'
import json, sys
manifest = json.load(open(sys.argv[1]))
assert manifest["setId"] == "agent"
assert manifest["root"] == sys.argv[2]
assert manifest["readOnly"] is True
assert manifest["pinId"]
assert {m["path"] for m in manifest["mounts"]} == {"docs/team", "docs/runbooks", "knowledge/policy"}
PY
assert_file_content "$daemon_project/docs/team/README.md" advanced
assert_file_content "$daemon_project/knowledge/policy/rules.md" policy
[[ "$(cat "$daemon_project/LOCAL.txt")" == "daemon-local" ]]
python3 - "$daemon_project/docs/team/README.md" <<'PY'
import errno, sys
try:
    with open(sys.argv[1], "w") as output:
        output.write("mutated\n")
except OSError as error:
    assert error.errno == errno.EROFS, error
else:
    raise AssertionError("kcfs daemon mount accepted a write")
PY
TMPDIR="$daemon_log_dir" "$run_root/kcfs" stop --pid "$daemon_pid"
daemon_pid=""
[[ ! -e "$daemon_project/docs/team" ]]
[[ ! -e "$daemon_project/docs/runbooks" ]]
[[ ! -e "$daemon_project/knowledge/policy" ]]
[[ "$(cat "$daemon_project/LOCAL.txt")" == "daemon-local" ]]
if find "$daemon_log_dir" -maxdepth 1 -name 'kcfs-daemon-*.log' -print -quit | rg -q .; then
  echo "FAIL: kcfs stop left a daemon log behind" >&2
  find "$daemon_log_dir" -maxdepth 1 -name 'kcfs-daemon-*.log' -print >&2
  exit 1
fi
echo "PASS: native kcfs daemon lifecycle"

remote_project="$run_root/remote-project"
mkdir -p "$remote_project"
printf 'remote-local\n' >"$remote_project/LOCAL.txt"
"$run_root/kcfs" mount --server "$server_url" --dataset agent --root "$remote_project" --as agent:test >"$run_root/remote-mount.json" 2>"$run_root/remote-kcfs.log" &
remote_pid=$!
remote_team="$remote_project/docs/team/README.md"
remote_mounted=0
for _ in $(seq 1 200); do
  if [[ -f "$remote_team" ]]; then
    remote_mounted=1
    break
  fi
  if ! kill -0 "$remote_pid" >/dev/null 2>&1; then
    cat "$run_root/remote-kcfs.log" >&2
    exit 1
  fi
  sleep 0.05
done
if [[ "$remote_mounted" != "1" ]]; then
  echo "FAIL: remote kcfs mount did not expose Dataset files" >&2
  cat "$run_root/remote-kcfs.log" >&2
  exit 1
fi
assert_file_content "$remote_team" advanced
[[ "$(cat "$remote_project/LOCAL.txt")" == "remote-local" ]]
advance_tree team docs/team/README.md 'remote-advance
' remote-advance
assert_file_content "$remote_team" advanced
if (printf 'mutated\n' >"$remote_team") 2>/dev/null; then
  echo "FAIL: remote kcfs mount accepted a write" >&2
  exit 1
fi
printf 'still-writable\n' >"$remote_project/LOCAL.txt"
[[ "$(cat "$remote_project/LOCAL.txt")" == "still-writable" ]]

kill -TERM "$remote_pid"
wait "$remote_pid"
remote_pid=""
kill -TERM "$server_pid"
wait "$server_pid"
server_pid=""
echo "PASS: kcfs multi-mount host filesystem smoke test"
