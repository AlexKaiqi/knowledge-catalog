#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scene_root="$(cd "${validation_dir}/.." && pwd)"
cache_dir="${scene_root}/.data/datawarehouse"
state_dir="${cache_dir}/dw04"
dw01_state="${cache_dir}/dw01"
tools_dir="${cache_dir}/tools"
compose_file="${validation_dir}/runtime/compose.yaml"
connector_source="${validation_dir}/connectors/mysql-structure-auto"
expected_path="${validation_dir}/fixtures/tpch-sf001/expected/dw04.json"
actual_path="${cache_dir}/actual/dw04.json"
repository_id="kr://tpch/validation/auto-physical"
catalog_id="kr://tpch/catalog"
workspace_id="tpch-auto-physical"
source_ref="mysql://127.0.0.1:13306/tpch"
mysql_password="dw-test-root"
compose_project="kc-dw-validation"
go_cache="${TMPDIR:-/tmp}/kc-dw-go-build-cache"
compose=(docker compose --project-name "${compose_project}" --file "${compose_file}")

fail() { echo "DW-04 FAIL: $*" >&2; exit 1; }

cleanup() {
  local status=$?
  if [[ "${mysql_paused:-0}" == "1" && -n "${mysql_container:-}" ]]; then
    docker unpause "${mysql_container}" >/dev/null 2>&1 || true
    mysql_paused=0
  fi
  if [[ -n "${host_pid:-}" ]]; then
    kill "${host_pid}" >/dev/null 2>&1 || true
    wait "${host_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${kc_pid:-}" ]]; then
    kill "${kc_pid}" >/dev/null 2>&1 || true
    wait "${kc_pid}" >/dev/null 2>&1 || true
  fi
  if [[ "${status}" != "0" || "${KC_DW_KEEP_MYSQL:-0}" != "1" ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  return "${status}"
}
trap cleanup EXIT

mysql_query() {
  "${compose[@]}" exec --no-TTY --env "MYSQL_PWD=${mysql_password}" mysql \
    mysql --user=root --database=tpch --batch --raw --skip-column-names --execute "$1"
}

free_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'
}

wait_health() {
  local url="$1"
  for _ in {1..100}; do
    if curl -fsS "${url}/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

object_id() {
  local kind="$1"
  local key="$2"
  python3 -c 'import hashlib,sys; print("dw-"+sys.argv[1]+"-"+hashlib.sha256(sys.argv[2].encode()).hexdigest()[:24])' "${kind}" "${key}"
}

repo_head() {
  "${kc_bin}" --home "${home_dir}" status \
    | jq -r --arg repo "${repository_id}" '.repos[] | select(.id==$repo) | .head'
}

connector_info() {
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto"
}

docker info >/dev/null 2>&1 || fail "Docker daemon is not running"
if [[ "${KC_DW_DEPENDENCIES_READY:-0}" != "1" ]]; then
  KC_DW_KEEP_MYSQL=1 "${validation_dir}/tests/dw01_mysql_structure.sh" >/dev/null
fi
[[ -f "${dw01_state}/source-key-map.json" ]] || fail "DW-01 source identity evidence is missing"
mysql_container="$(${compose[@]} ps -q mysql)"
[[ -n "${mysql_container}" ]] || fail "TPC-H MySQL container is not running"
mysql_paused=0

case "${state_dir}" in
  "${cache_dir}"/*) rm -rf -- "${state_dir}" ;;
  *) fail "refusing to reset unexpected state path ${state_dir}" ;;
esac
mkdir -p "${state_dir}" "${tools_dir}" "${cache_dir}/actual" "${go_cache}"

kc_bin="${tools_dir}/kc-dw01"
host_bin="${tools_dir}/connector-host-dw04"
[[ -x "${kc_bin}" ]] || fail "DW-01 kc binary is missing"
GOCACHE="${go_cache}" go -C "${scene_root}" build -buildvcs=false -o "${host_bin}" ./validation/connectorhost/cmd/connector-host

public_repo="${state_dir}/public-connectors"
remote_repo="${state_dir}/public-connectors.git"
host_home="${state_dir}/host-home"
home_dir="${state_dir}/kc-home"
evidence_dir="${state_dir}/evidence"
package_dir="${public_repo}/connectors/mysql-structure-auto"
mkdir -p "${package_dir}" "${host_home}" "${home_dir}" "${evidence_dir}"
cp "${connector_source}/connector.yaml" "${connector_source}/collector.py" \
  "${connector_source}/test_collector.py" "${package_dir}/"

git init --bare --initial-branch=main "${remote_repo}" >/dev/null
git -C "${public_repo}" init -b main >/dev/null
git -C "${public_repo}" add connectors/mysql-structure-auto
git -C "${public_repo}" -c user.name=dw04-validation -c user.email=dw04@example.invalid \
  commit -m 'add scheduled MySQL structure connector' >/dev/null
git -C "${public_repo}" remote add origin "${remote_repo}"
git -C "${public_repo}" push -u origin main >/dev/null

"${kc_bin}" --home "${home_dir}" init --catalog "${catalog_id}" >/dev/null
"${kc_bin}" --home "${home_dir}" repo-add --repo "${repository_id}" >"${evidence_dir}/repo-add.json"
"${kc_bin}" --home "${home_dir}" allow \
  --principal connector/mysql-structure-auto --cmd commit \
  --repo "${repository_id}" --ref refs/heads/main >"${evidence_dir}/allow.json"
"${kc_bin}" --home "${home_dir}" define-workspace --catalog "${catalog_id}" \
  --workspace "${workspace_id}" --revision 1 \
  --source "${repository_id}=refs/heads/main" >"${evidence_dir}/workspace.json"

kc_port="$(free_port)"
host_port="$(free_port)"
kc_listen="127.0.0.1:${kc_port}"
host_listen="127.0.0.1:${host_port}"
kc_url="http://${kc_listen}"
host_url="http://${host_listen}"

"${kc_bin}" --home "${home_dir}" serve --listen "${kc_listen}" >"${evidence_dir}/kc.log" 2>&1 &
kc_pid=$!
wait_health "${kc_url}" || fail "kc serve did not become healthy"

"${host_bin}" --home "${host_home}" repo-set \
  --repo "${remote_repo}" --ref refs/heads/main --sync-every 1s --kc-url "${kc_url}" \
  >"${evidence_dir}/repo-set.json"
"${host_bin}" --home "${host_home}" validate --connector mysql-structure-auto \
  >"${evidence_dir}/validate.json"
KC_MYSQL_CONTAINER="${mysql_container}" KC_MYSQL_PASSWORD="${mysql_password}" \
  "${host_bin}" --home "${host_home}" run --connector mysql-structure-auto --preview \
  >"${evidence_dir}/preview.json"
jq -e '.outcome=="PREVIEWED" and .summary=={added:69,updated:0,removed:0,unchanged:0,ignored:0}' \
  "${evidence_dir}/preview.json" >/dev/null || fail "initial preview did not contain 69 physical Addresses"
"${host_bin}" --home "${host_home}" activate --connector mysql-structure-auto \
  >"${evidence_dir}/activate.json"

KC_MYSQL_CONTAINER="${mysql_container}" KC_MYSQL_PASSWORD="${mysql_password}" \
  "${host_bin}" --home "${host_home}" serve --listen "${host_listen}" \
  >"${evidence_dir}/host.log" 2>&1 &
host_pid=$!
wait_health "${host_url}" || fail "Connector Host did not become healthy"

initial_run=""
for _ in {1..800}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=200" \
    >"${evidence_dir}/runs-initial.json"
  initial_run="$(jq -c '[.[]|select(.trigger.kind=="schedule" and .outcome=="SUCCEEDED" and .summary.added==69)][0] // empty' \
    "${evidence_dir}/runs-initial.json")"
  [[ -n "${initial_run}" ]] && break
  sleep 0.25
done
[[ -n "${initial_run}" ]] || fail "scheduler did not perform the initial physical structure collection"
initial_commit="$(jq -r '.targetCommit' <<<"${initial_run}")"
initial_checkpoint="$(jq -r '.checkpointVersion' <<<"${initial_run}")"
[[ -n "${initial_commit}" && "${initial_commit}" != "null" ]] || fail "initial scheduled run returned no commit"

old_pin="${evidence_dir}/before-ddl.pin.json"
fresh_pin="${evidence_dir}/after-ddl.pin.json"
"${kc_bin}" --home "${home_dir}" resolve --catalog "${catalog_id}" --workspace "${workspace_id}" >"${old_pin}"
[[ "$(jq -r --arg repo "${repository_id}" '.repositories[$repo]' "${old_pin}")" == "${initial_commit}" ]] \
  || fail "old Workspace pin does not match the initial scheduled commit"

orders_key="${source_ref}/table/tpch/orders"
part_key="${source_ref}/table/tpch/part"
phone_key="${source_ref}/column/tpch/customer/c_phone"
removed_key="${source_ref}/column/tpch/part/p_comment"
added_key="${source_ref}/column/tpch/orders/o_pipeline_note"
orders_id="$(object_id table "${orders_key}")"
part_id="$(object_id table "${part_key}")"
phone_id="$(object_id column "${phone_key}")"
removed_id="$(object_id column "${removed_key}")"
added_id="$(object_id column "${added_key}")"

old_orders="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" --object "${orders_id}")"
old_part="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" --object "${part_id}")"
old_phone="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" --object "${phone_id}")"
[[ "$(jq -r '.[0].value.structure.columnCount' <<<"${old_orders}")" == "9" ]] || fail "old orders column count is not 9"
[[ "$(jq -r '.[0].value.structure.columnCount' <<<"${old_part}")" == "9" ]] || fail "old part column count is not 9"
[[ "$(jq -r '.[0].value.structure.columnType' <<<"${old_phone}")" == "char(15)" ]] || fail "old c_phone type is not char(15)"
old_future="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${old_pin}" --object "${added_id}")"
[[ "$(jq 'length' <<<"${old_future}")" == "0" ]] || fail "old pin unexpectedly contains the future column"

# An activated generation is immutable. A synchronized package change must
# stop scheduled execution until the new generation is validated and activated.
generation_state_before="$(connector_info)"
generation_before="$(jq -r '.generation' <<<"${generation_state_before}")"
generation_checkpoint="$(jq -r '.state.checkpointVersion' <<<"${generation_state_before}")"
generation_head="$(repo_head)"
touch "${package_dir}/generation-v2.marker"
git -C "${public_repo}" add connectors/mysql-structure-auto/generation-v2.marker
git -C "${public_repo}" -c user.name=dw04-validation -c user.email=dw04@example.invalid \
  commit -m 'publish a new Connector generation' >/dev/null
git -C "${public_repo}" push origin main >/dev/null

generation_drift=""
for _ in {1..160}; do
  generation_drift="$(connector_info)"
  current_generation="$(jq -r '.generation' <<<"${generation_drift}")"
  active_generation="$(jq -r '.state.activeGeneration' <<<"${generation_drift}")"
  last_error="$(jq -r '.state.lastError' <<<"${generation_drift}")"
  if [[ "${current_generation}" != "${generation_before}" && "${current_generation}" != "${active_generation}" \
    && "${last_error}" == *"changed after activation"* ]]; then
    break
  fi
  sleep 0.25
done
[[ -n "${generation_drift}" && "$(jq -r '.generation' <<<"${generation_drift}")" != "${generation_before}" ]] \
  || fail "Host did not synchronize the changed Connector generation"
[[ "$(jq -r '.state.lastError' <<<"${generation_drift}")" == *"changed after activation"* ]] \
  || fail "scheduler did not block the unactivated Connector generation"
[[ "$(repo_head)" == "${generation_head}" ]] || fail "generation drift moved the target Repository"
generation_checkpoint="$(jq -r '.state.checkpointVersion' <<<"${generation_drift}")"
sleep 2
generation_blocked_again="$(connector_info)"
[[ "$(jq -r '.state.checkpointVersion' <<<"${generation_blocked_again}")" == "${generation_checkpoint}" ]] \
  || fail "blocked generation advanced the Connector checkpoint"
[[ "$(jq -r '.state.lastError' <<<"${generation_blocked_again}")" == *"changed after activation"* ]] \
  || fail "blocked generation resumed without reactivation"
[[ "$(repo_head)" == "${generation_head}" ]] || fail "blocked generation moved the target Repository"

curl -fsS -X POST "${host_url}/api/connectors/mysql-structure-auto/validate" \
  >"${evidence_dir}/validate-generation-v2.json"
curl -fsS -X POST "${host_url}/api/connectors/mysql-structure-auto/activate" \
  >"${evidence_dir}/activate-generation-v2.json"

generation_empty=""
for _ in {1..160}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=300" \
    >"${evidence_dir}/runs-generation-v2.json"
  generation_empty="$(jq -c --argjson checkpoint "${generation_checkpoint}" \
    '[.[]|select(.trigger.kind=="schedule" and .outcome=="EMPTY" and .checkpointVersion>$checkpoint and .summary.unchanged==69)][0] // empty' \
    "${evidence_dir}/runs-generation-v2.json")"
  [[ -n "${generation_empty}" ]] && break
  sleep 0.25
done
[[ -n "${generation_empty}" ]] || fail "reactivated generation did not resume with an empty scheduled reconcile"
stable_checkpoint="$(jq -r '.checkpointVersion' <<<"${generation_empty}")"
[[ "$(repo_head)" == "${initial_commit}" ]] || fail "unchanged source created a knowledge commit after reactivation"

# A temporary source outage must produce a failed scheduled run without moving
# either the target head or the checkpoint. Recovery must happen automatically.
docker pause "${mysql_container}" >/dev/null
mysql_paused=1
failure_state_before="$(connector_info)"
failure_checkpoint="$(jq -r '.state.checkpointVersion' <<<"${failure_state_before}")"
failure_head="$(repo_head)"
source_failure=""
for _ in {1..160}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=300" \
    >"${evidence_dir}/runs-source-failure.json"
  source_failure="$(jq -c --argjson checkpoint "${failure_checkpoint}" \
    '[.[]|select(.trigger.kind=="schedule" and .outcome=="FAILED" and .checkpointVersion==$checkpoint and (.error|contains("connector command failed")) and (.stderr|contains("mysql-structure-auto")))][0] // empty' \
    "${evidence_dir}/runs-source-failure.json")"
  [[ -n "${source_failure}" ]] && break
  sleep 0.25
done
[[ -n "${source_failure}" ]] || fail "paused MySQL did not produce a failed scheduled run"
failure_state_after="$(connector_info)"
[[ "$(jq -r '.state.checkpointVersion' <<<"${failure_state_after}")" == "${failure_checkpoint}" ]] \
  || fail "source failure advanced the Connector checkpoint"
[[ "$(jq -r '.state.lastCommit' <<<"${failure_state_after}")" == "${initial_commit}" ]] \
  || fail "source failure changed the Connector last commit"
[[ "$(repo_head)" == "${failure_head}" ]] || fail "source failure moved the target Repository"

docker unpause "${mysql_container}" >/dev/null
mysql_paused=0
recovery_empty=""
for _ in {1..160}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=300" \
    >"${evidence_dir}/runs-source-recovery.json"
  recovery_empty="$(jq -c --argjson checkpoint "${failure_checkpoint}" \
    '[.[]|select(.trigger.kind=="schedule" and .outcome=="EMPTY" and .checkpointVersion>$checkpoint and .summary.unchanged==69)][0] // empty' \
    "${evidence_dir}/runs-source-recovery.json")"
  [[ -n "${recovery_empty}" ]] && break
  sleep 0.25
done
[[ -n "${recovery_empty}" ]] || fail "scheduler did not recover automatically after MySQL resumed"

mysql_query "
ALTER TABLE orders ADD COLUMN o_pipeline_note VARCHAR(64) NULL COMMENT 'scheduled metadata probe';
ALTER TABLE customer MODIFY COLUMN c_phone VARCHAR(32) NOT NULL;
ALTER TABLE part DROP COLUMN p_comment;"

ddl_run=""
for _ in {1..400}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=300" \
    >"${evidence_dir}/runs-after-ddl.json"
  ddl_run="$(jq -c --arg initial "${initial_commit}" \
    '[.[]|select(.trigger.kind=="schedule" and .outcome=="SUCCEEDED" and .targetCommit!=$initial and .summary.added==1 and .summary.updated==3 and .summary.removed==1 and .summary.unchanged==65)][0] // empty' \
    "${evidence_dir}/runs-after-ddl.json")"
  [[ -n "${ddl_run}" ]] && break
  sleep 0.25
done
[[ -n "${ddl_run}" ]] || fail "scheduler did not reconcile the real MySQL DDL change"
ddl_commit="$(jq -r '.targetCommit' <<<"${ddl_run}")"
ddl_checkpoint="$(jq -r '.checkpointVersion' <<<"${ddl_run}")"
[[ "${ddl_commit}" != "${initial_commit}" ]] || fail "DDL collection did not create a new knowledge commit"
(( ddl_checkpoint > initial_checkpoint )) || fail "DDL collection did not advance the Connector checkpoint"

post_ddl_empty=""
for _ in {1..160}; do
  curl -fsS "${host_url}/api/connectors/mysql-structure-auto/runs?limit=400" \
    >"${evidence_dir}/runs-final.json"
  post_ddl_empty="$(jq -c --argjson checkpoint "${ddl_checkpoint}" \
    '[.[]|select(.trigger.kind=="schedule" and .outcome=="EMPTY" and .checkpointVersion>$checkpoint and .summary.unchanged==69)][0] // empty' \
    "${evidence_dir}/runs-final.json")"
  [[ -n "${post_ddl_empty}" ]] && break
  sleep 0.25
done
[[ -n "${post_ddl_empty}" ]] || fail "stable post-DDL source did not produce an empty scheduled reconcile"
[[ "$(repo_head)" == "${ddl_commit}" ]] || fail "empty post-DDL reconcile created another knowledge commit"
manual_commit_runs="$(jq '[.[]|select(.trigger.kind=="manual" and .outcome=="SUCCEEDED")]|length' "${evidence_dir}/runs-final.json")"
[[ "${manual_commit_runs}" == "0" ]] || fail "a manual Connector run wrote knowledge"

"${kc_bin}" --home "${home_dir}" resolve --catalog "${catalog_id}" --workspace "${workspace_id}" >"${fresh_pin}"
[[ "$(jq -r --arg repo "${repository_id}" '.repositories[$repo]' "${fresh_pin}")" == "${ddl_commit}" ]] \
  || fail "fresh Workspace pin does not match the DDL commit"

old_count="$(${kc_bin} --home "${home_dir}" list --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" | jq 'length')"
fresh_count="$(${kc_bin} --home "${home_dir}" list --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${fresh_pin}" | jq 'length')"
new_orders="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${fresh_pin}" --object "${orders_id}")"
new_part="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${fresh_pin}" --object "${part_id}")"
new_phone="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${fresh_pin}" --object "${phone_id}")"
new_column="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${fresh_pin}" --object "${added_id}")"
[[ "${old_count}" == "69" && "${fresh_count}" == "69" ]] || fail "Workspace object counts drifted: old=${old_count} fresh=${fresh_count}"
[[ "$(jq -r '.[0].value.structure.columnCount' <<<"${new_orders}")" == "10" ]] || fail "fresh orders column count is not 10"
[[ "$(jq -r '.[0].value.structure.columnCount' <<<"${new_part}")" == "8" ]] || fail "fresh part column count is not 8"
[[ "$(jq -r '.[0].value.structure.columnType' <<<"${new_phone}")" == "varchar(32)" ]] || fail "fresh c_phone type is not varchar(32)"
[[ "$(jq -r '.[0].value.structure.comment' <<<"${new_column}")" == "scheduled metadata probe" ]] || fail "new column comment was not collected"
fresh_removed="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${fresh_pin}" --object "${removed_id}")"
[[ "$(jq 'length' <<<"${fresh_removed}")" == "0" ]] || fail "fresh pin still contains the removed column"

old_orders_again="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" --object "${orders_id}")"
[[ "$(jq -r '.[0].value.structure.columnCount' <<<"${old_orders_again}")" == "9" ]] || fail "old Workspace pin drifted after automatic DDL collection"
old_removed="$(${kc_bin} --home "${home_dir}" read --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin}" --object "${removed_id}")"
[[ "$(jq -r '.[0].value.structure.name' <<<"${old_removed}")" == "p_comment" ]] || fail "old pin no longer reproduces the removed column"

provenance="$(${kc_bin} --home "${home_dir}" provenance --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${fresh_pin}" --object "${added_id}")"
jq -e --arg source "${source_ref}" '
  [.[].chain[] | select(.originKind=="SOURCE" and .actorRef=="connector/mysql-structure-auto" and (.sourceRefs|index($source)))] | length >= 1
' <<<"${provenance}" >/dev/null || fail "automatic DDL knowledge lost SOURCE provenance"

jq -n \
  --arg fixture "tpch-sf001" --arg source_ref "${source_ref}" \
  --argjson initial_checkpoint "${initial_checkpoint}" --argjson ddl_checkpoint "${ddl_checkpoint}" \
  --argjson manual_commit_runs "${manual_commit_runs}" \
  --argjson old_count "${old_count}" --argjson fresh_count "${fresh_count}" \
  '{
    fixture:$fixture,
    automation:{
      trigger:"schedule",manualCommitRuns:$manual_commit_runs,
      initialCollection:{added:69,updated:0,removed:0,unchanged:0},
      ddlReconcile:{added:1,updated:3,removed:1,unchanged:65},
      checkpointAdvanced:($ddl_checkpoint>$initial_checkpoint),commitsDiffer:true,
      generationDriftBlocked:true,sourceFailureCheckpointStable:true,
      sourceRecoveryAutomatic:true,stableNoopCreatedCommit:false
    },
    ddl:{added:"tpch.orders.o_pipeline_note",changed:"tpch.customer.c_phone",removed:"tpch.part.p_comment"},
    workspace:{
      oldObjectCount:$old_count,freshObjectCount:$fresh_count,
      oldOrdersColumnCount:9,freshOrdersColumnCount:10,
      oldPartColumnCount:9,freshPartColumnCount:8,
      oldPhoneType:"char(15)",freshPhoneType:"varchar(32)",
      addedColumnComment:"scheduled metadata probe",
      oldPinReproduced:true,freshPinObservedDDL:true
    },
    provenance:{originKind:"SOURCE",actorRef:"connector/mysql-structure-auto",sourceRef:$source_ref}
  }' >"${actual_path}"

if ! diff -u <(jq -S . "${expected_path}") <(jq -S . "${actual_path}"); then
  fail "actual output differs from ${expected_path}; actual: ${actual_path}"
fi

echo "DW-04 PASS: scheduled Connector reconciled real MySQL ADD/MODIFY/DROP DDL, advanced checkpoint, and preserved the old Workspace pin"
echo "actual: ${actual_path}"
