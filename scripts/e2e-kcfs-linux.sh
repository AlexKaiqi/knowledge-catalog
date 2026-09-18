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

case "$(uname -m)" in
  aarch64|arm64) dolt_arch="arm64" ;;
  x86_64|amd64) dolt_arch="amd64" ;;
  *) echo "FAIL: unsupported Dolt test architecture $(uname -m)" >&2; exit 1 ;;
esac
curl -fsSL "https://github.com/dolthub/dolt/releases/latest/download/dolt-linux-${dolt_arch}.tar.gz" | tar -xz -C "$run_root"
export KC_DOLT_BIN="$run_root/dolt-linux-${dolt_arch}/bin/dolt"

home_dir="$run_root/home"
project_dir="$run_root/project"
team_repo="$run_root/team-repo"
policy_repo="$run_root/policy-repo"
mkdir -p "$project_dir" "$run_root/kc-config"
export KC_CONFIG_DIR="$run_root/kc-config"
printf 'local\n' >"$project_dir/LOCAL.txt"

deployment_config="$(go -C "$repo_root" run ./scripts/fixture-deployment --root "$home_dir" --catalog kr://test/catalog --principal agent:test --repo "kr://test/team=$team_repo" --repo "kr://test/policy=$policy_repo")"
"$run_root/kc" deployment init --config "$deployment_config" >/dev/null
(cd "$team_repo" && "$KC_DOLT_BIN" sql -q "INSERT INTO kc_files(path,content) VALUES ('team/README.md',FROM_BASE64('dGVhbQo=')),('runbooks/incident.md',FROM_BASE64('aW5jaWRlbnQK'))" >/dev/null && "$KC_DOLT_BIN" add . && "$KC_DOLT_BIN" commit -m seed >/dev/null)
(cd "$policy_repo" && "$KC_DOLT_BIN" sql -q "INSERT INTO kc_files(path,content) VALUES ('rules.md',FROM_BASE64('cG9saWN5Cg=='))" >/dev/null && "$KC_DOLT_BIN" add . && "$KC_DOLT_BIN" commit -m seed >/dev/null)

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

# Server keeps a live `dolt sql -r json --continue` session that holds the write
# lease. Drop those sessions (same as snapshot/dolt closeEngineAt) so this
# harness can commit an upstream tree change.
advance_tree() {
  local dir="$1"
  local sql="$2"
  local message="$3"
  local i
  for i in $(seq 1 20); do
    pkill -f "$KC_DOLT_BIN sql -r json --continue" >/dev/null 2>&1 || true
    sleep 0.05
    if (cd "$dir" && "$KC_DOLT_BIN" sql -q "$sql" >/dev/null && "$KC_DOLT_BIN" add . && "$KC_DOLT_BIN" commit -m "$message" >/dev/null); then
      return 0
    fi
    sleep 0.1
  done
  echo "FAIL: could not advance $dir ($message)" >&2
  exit 1
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
advance_tree "$team_repo" "UPDATE kc_files SET content=FROM_BASE64('YWR2YW5jZWQK') WHERE path='team/README.md'" advance
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
advance_tree "$team_repo" "UPDATE kc_files SET content=FROM_BASE64('cmVtb3RlLW5ldwo=') WHERE path='team/README.md'" remote-advance
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
