#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "SKIP: kcfs host mount smoke test requires Linux"
  exit 0
fi
if [[ ! -e /dev/fuse ]]; then
  echo "SKIP: /dev/fuse is unavailable"
  exit 0
fi
if ! command -v fusermount3 >/dev/null 2>&1 && ! command -v fusermount >/dev/null 2>&1; then
  echo "SKIP: install fuse3 (fusermount3)"
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_root="$(mktemp -d)"
kc_pid=""
remote_pid=""
server_pid=""
cleanup() {
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

if [[ -z "${KC_LAKEFS_URL:-}" || -z "${KC_LAKEFS_CREDENTIAL:-}" ]]; then
  echo "SKIP: kcfs host mount smoke test needs the local lakeFS stack; run scripts/system-lakefs.sh local up first (exports KC_LAKEFS_URL and KC_LAKEFS_CREDENTIAL)"
  exit 0
fi
lakefs_url="${KC_LAKEFS_URL%/}"

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
    -d "{\"name\":\"${name}\",\"storage_namespace\":\"s3://kc-authority/kcfs-${name}\",\"default_branch\":\"main\"}")"
  if [[ "$status" != "201" ]]; then
    echo "FAIL: create lakeFS repository ${name} returned HTTP $status" >&2
    cat "$run_root/repo.json" >&2 || true
    exit 1
  fi
}

# lakefs_put stages one object on main and commits it. This is the upstream
# tree-mutation path: the FUSE mount must stay frozen at its pinned commit.
lakefs_put() {
  local repo="$1" path="$2" content="$3" message="$4" enc
  enc="$(python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=""))' "$path")"
  curl -fsS -u "$KC_LAKEFS_CREDENTIAL" -X PUT \
    "$lakefs_url/api/v1/repositories/${repo}/branches/main/objects?path=${enc}" \
    -H 'Content-Type: application/octet-stream' --data-binary "$content" >/dev/null
  curl -fsS -u "$KC_LAKEFS_CREDENTIAL" -X POST \
    "$lakefs_url/api/v1/repositories/${repo}/branches/main/commits" \
    -H 'Content-Type: application/json' -d "{\"message\":\"${message}\"}" >/dev/null
}

lakefs_ensure_repo team
lakefs_ensure_repo policy
lakefs_put team docs/team/README.md 'team
' seed
lakefs_put team docs/runbooks/incident.md 'incident
' seed
lakefs_put policy knowledge/policy/rules.md 'policy
' seed

deployment_config="$(go -C "$repo_root" run ./scripts/fixture-deployment --root "$home_dir" --catalog kr://test/catalog --principal agent:test --lakefs-repo "kr://test/team=${lakefs_url}/team" --lakefs-repo "kr://test/policy=${lakefs_url}/policy")"
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

run_kc catalog use kr://test/catalog
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

if [[ -n "${KC_DSH_PLUGIN_MOUNT_MODULE:-}" ]]; then
  if [[ ! -f "$KC_DSH_PLUGIN_MOUNT_MODULE" ]]; then
    echo "FAIL: DSH MountController module is missing: $KC_DSH_PLUGIN_MOUNT_MODULE" >&2
    exit 1
  fi
  if ! command -v node >/dev/null 2>&1; then
    echo "FAIL: node is required for the DSH MountController live test" >&2
    exit 1
  fi
  plugin_project="$run_root/plugin-project"
  plugin_home="$run_root/plugin-home"
  mkdir -p "$plugin_project" "$plugin_home"
  printf 'plugin-local\n' >"$plugin_project/LOCAL.txt"
  node --input-type=module - "$KC_DSH_PLUGIN_MOUNT_MODULE" "$plugin_home" "$run_root/kcfs" "$plugin_project" "$server_url" <<'JS'
import assert from 'node:assert/strict';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const [modulePath, home, kcfs, root, server] = process.argv.slice(2);
const { MountController } = await import(pathToFileURL(modulePath).href);
const controller = new MountController({
  mountFiles: true,
  home,
  bin: kcfs,
  server,
  catalog: 'kr://test/catalog',
  workspace: 'agent',
  view: 'repository',
  principal: 'agent:test',
});
const session = { id: 'docker-live', header: { cwd: root } };
controller.created(session);
try {
  const contextPath = path.join(home, 'tasks', Buffer.from(session.id).toString('base64url'), 'context.json');
  const context = JSON.parse(readFileSync(contextPath, 'utf8'));
  assert.equal(context.dataset, 'agent');
  assert.equal(context.workspace, 'agent');
  assert.equal(context.root, root);
  assert.equal(context.readOnly, true);
  assert.ok(context.pinId);
  assert.equal(readFileSync(path.join(root, 'docs/team/README.md'), 'utf8'), 'advanced\n');
  assert.equal(readFileSync(path.join(root, 'knowledge/policy/rules.md'), 'utf8'), 'policy\n');
  assert.equal(readFileSync(path.join(root, 'LOCAL.txt'), 'utf8'), 'plugin-local\n');
  assert.throws(
    () => writeFileSync(path.join(root, 'docs/team/README.md'), 'mutated\n'),
    (error) => error && error.code === 'EROFS',
  );
} finally {
  controller.disposed(session);
}
assert.equal(existsSync(path.join(root, 'docs/team')), false);
assert.equal(existsSync(path.join(home, 'tasks', Buffer.from(session.id).toString('base64url'))), false);
assert.equal(readFileSync(path.join(root, 'LOCAL.txt'), 'utf8'), 'plugin-local\n');
console.log('PASS: DSH MountController real kcfs daemon lifecycle');
JS
  if find /tmp -maxdepth 1 -name 'kcfs-daemon-*.log' -print -quit | grep -q .; then
    echo "FAIL: DSH MountController stop left a daemon log behind" >&2
    find /tmp -maxdepth 1 -name 'kcfs-daemon-*.log' -print >&2
    exit 1
  fi
fi

remote_project="$run_root/remote-project"
mkdir -p "$remote_project"
printf 'remote-local\n' >"$remote_project/LOCAL.txt"
"$run_root/kcfs" mount --server "$server_url" --catalog kr://test/catalog --dataset agent --root "$remote_project" --as agent:test >"$run_root/remote-mount.json" 2>"$run_root/remote-kcfs.log" &
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
