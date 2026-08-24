#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_dir="$(cd "${validation_dir}/.." && pwd)"
cache_dir="${repo_dir}/.data/datawarehouse"
state_dir="${cache_dir}/dw03"
dw01_state="${cache_dir}/dw01"
dw02_state="${cache_dir}/dw02"
tools_dir="${cache_dir}/tools"
compose_file="${validation_dir}/runtime/compose.yaml"
expected_path="${validation_dir}/fixtures/tpch-sf001/expected/dw03.json"
actual_path="${cache_dir}/actual/dw03.json"
repository_id="kr://tpch/public/physical"
source_ref="mysql://127.0.0.1:13306/tpch"
mysql_password="dw-test-root"
compose_project="kc-dw-validation"
go_cache="${TMPDIR:-/tmp}/kc-dw-go-build-cache"
compose=(docker compose --project-name "${compose_project}" --file "${compose_file}")

fail() { echo "DW-03 FAIL: $*" >&2; exit 1; }

cleanup() {
  if [[ "${KC_DW_KEEP_MYSQL:-0}" != "1" ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

mysql_query() {
  "${compose[@]}" exec --no-TTY --env "MYSQL_PWD=${mysql_password}" mysql \
    mysql --user=root --database=tpch --batch --raw --skip-column-names --execute "$1"
}

docker info >/dev/null 2>&1 || fail "Docker daemon is not running"
KC_DW_KEEP_MYSQL=1 "${validation_dir}/tests/dw02_mysql_observations.sh" >/dev/null

case "${state_dir}" in
  "${cache_dir}"/*) rm -rf -- "${state_dir}" ;;
  *) fail "refusing to reset unexpected state path ${state_dir}" ;;
esac
mkdir -p "${state_dir}" "${cache_dir}/actual"

kc_bin="${tools_dir}/kc-dw01"
observation_bin="${tools_dir}/mysql-observations"
checkpoint_bin="${tools_dir}/mysql-cdc-checkpoint"
home_dir="${dw01_state}/kc-home"
mapping_path="${dw01_state}/source-key-map.json"
baseline_snapshot="${dw02_state}/observation-snapshot.json"
baseline_observed="${dw02_state}/observed.json"
event_path="${state_dir}/event.json"
old_event_path="${state_dir}/old-event.json"
checkpoint_path="${state_dir}/checkpoint.json"
updated_snapshot="${state_dir}/observation-snapshot.json"
updated_preview="${state_dir}/updated-preview.json"
stable_preview="${state_dir}/stable-preview.json"
updated_observed="${state_dir}/observed.json"

if [[ ! -x "${checkpoint_bin}" ]]; then
  (cd "${repo_dir}" && GOCACHE="${go_cache}" go build -buildvcs=false -o "${checkpoint_bin}" ./validation/cmd/mysql-cdc-checkpoint)
fi

read -r binlog_file start_position _ <<< "$(mysql_query "SHOW BINARY LOG STATUS;")"
[[ "${binlog_file}" == "mysql-bin.000003" && "${start_position}" == "158" ]] \
  || fail "unexpected deterministic start coordinate ${binlog_file}:${start_position}"
before_value="$(mysql_query "SELECT CAST(l_discount AS CHAR) FROM lineitem WHERE l_orderkey=131 AND l_linenumber=3;")"
[[ "${before_value}" == "0.04" ]] || fail "mutation precondition was ${before_value}, expected 0.04"
affected_rows="$(mysql_query "UPDATE lineitem SET l_discount=0.05 WHERE l_orderkey=131 AND l_linenumber=3; SELECT ROW_COUNT();")"
[[ "${affected_rows}" == "1" ]] || fail "mutation affected ${affected_rows} rows"
after_value="$(mysql_query "SELECT CAST(l_discount AS CHAR) FROM lineitem WHERE l_orderkey=131 AND l_linenumber=3;")"
read -r event_file event_position _ <<< "$(mysql_query "SHOW BINARY LOG STATUS;")"
[[ "${after_value}" == "0.05" ]] || fail "mutation result was ${after_value}, expected 0.05"
[[ "${event_file}" == "mysql-bin.000003" && "${event_position}" == "687" ]] \
  || fail "unexpected deterministic event coordinate ${event_file}:${event_position}"

event_id="${event_file}:${event_position}"
jq -n --arg source_ref "${source_ref}" --arg event_id "${event_id}" --arg file "${event_file}" \
  --argjson position "${event_position}" '{
    sourceRef:$source_ref,eventId:$event_id,binlogFile:$file,position:$position,
    operation:"UPDATE",schema:"tpch",table:"lineitem",
    key:{l_orderkey:131,l_linenumber:3},before:{l_discount:"0.04"},after:{l_discount:"0.05"}
  }' > "${event_path}"

first_decision="$(${checkpoint_bin} --event "${event_path}" --checkpoint "${checkpoint_path}" | jq -r '.decision')"
[[ "${first_decision}" == "APPLY" ]] || fail "first checkpoint decision was ${first_decision}"
event_payload="$(jq -c . "${event_path}")"
append_command_id="connector:mysql-tpch-cdc:${event_id}"
first_append="$(${kc_bin} append --home "${home_dir}" --command-id "${append_command_id}" \
  --repo "${repository_id}" --stream mysql-binlog --event-id "${event_id}" --event-type mysql.row.UPDATE \
  --payload "${event_payload}")"
first_disposition="$(jq -r '.disposition' <<< "${first_append}")"
first_cursor="$(jq -r '.result.cursor' <<< "${first_append}")"
[[ "${first_disposition}" == "APPLIED" && "${first_cursor}" == "1" ]] || fail "first APPEND did not apply at cursor 1"

profile_base="${state_dir}/profile-base.json"
distribution_jsonl="${state_dir}/distribution.jsonl"
mysql_query "
SELECT JSON_OBJECT(
  'schema','tpch','table','lineitem','column','l_discount',
  'rowCount',COUNT(*),'ndv',COUNT(DISTINCT l_discount),
  'minValue',CAST(MIN(l_discount) AS CHAR),'maxValue',CAST(MAX(l_discount) AS CHAR),
  'avgValue',CAST(CAST(AVG(l_discount) AS DECIMAL(20,8)) AS CHAR)
) FROM lineitem;" > "${profile_base}"
mysql_query "
SELECT JSON_OBJECT('value',CAST(l_discount AS CHAR),'rowCount',COUNT(*))
FROM lineitem GROUP BY l_discount ORDER BY l_discount;" > "${distribution_jsonl}"
captured_at="$(mysql_query "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ');")"
binlog_ref="${source_ref}/binlog/${event_id}"
jq --slurpfile profile "${profile_base}" --slurpfile distribution "${distribution_jsonl}" \
  --arg captured_at "${captured_at}" --arg binlog_ref "${binlog_ref}" --arg query_ref "${source_ref}/query/dw03" '
    .capturedAt=$captured_at
    | .provenanceRefs=[$binlog_ref,$query_ref]
    | .profile=($profile[0]+{distribution:$distribution})
  ' "${baseline_snapshot}" > "${updated_snapshot}"

profile_object="$(jq -r '.changeSet.operations[]|select(.address.aspectName=="profile")|.address.objectId' "${dw02_state}/first-preview.json")"
base_resolution="$(${kc_bin} resolve --home "${home_dir}" --repo "${repository_id}" --ref refs/heads/main \
  --object "${profile_object}" --aspect profile)"
base_commit="$(jq -r '.commit' <<< "${base_resolution}")"
"${observation_bin}" --snapshot "${updated_snapshot}" --mapping "${mapping_path}" --observed "${baseline_observed}" \
  --base "${base_commit}" --produced-at "2026-08-23T00:02:00Z" --out "${updated_preview}"
jq -e '.summary=={added:0,updated:1,removed:0,unchanged:7,ignored:0} and (.changeSet.operations|length==1) and .changeSet.operations[0].address.aspectName=="profile"' \
  "${updated_preview}" >/dev/null || fail "CDC reconcile was not exactly one profile update"

catalog_receipt="$(${kc_bin} commit --home "${home_dir}" --command-id connector:mysql-tpch-observations:dw03-000003-687 \
  --changeset "${updated_preview}")"
catalog_disposition="$(jq -r '.disposition' <<< "${catalog_receipt}")"
new_commit="$(jq -r '.result.newCommit' <<< "${catalog_receipt}")"
[[ "${catalog_disposition}" == "APPLIED" ]] || fail "profile update commit was not APPLIED"

new_resolution="$(${kc_bin} resolve --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" \
  --object "${profile_object}" --aspect profile)"
new_digest="$(jq -r '.digest' <<< "${new_resolution}")"
jq --arg object "${profile_object}" --arg digest "${new_digest}" '
  map(if .address.objectId==$object and .address.aspectName=="profile" then .digest=$digest else . end)
' "${baseline_observed}" > "${updated_observed}"
"${observation_bin}" --snapshot "${updated_snapshot}" --mapping "${mapping_path}" --observed "${updated_observed}" \
  --base "${new_commit}" --produced-at "2026-08-23T00:02:01Z" --out "${stable_preview}"
jq -e '.summary=={added:0,updated:0,removed:0,unchanged:8,ignored:0} and .empty and (.changeSet.operations|length==0)' \
  "${stable_preview}" >/dev/null || fail "post-commit reconcile was not empty"

provenance="$(${kc_bin} provenance --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" --object "${profile_object}")"
binlog_provenance_count="$(jq --arg ref "${binlog_ref}" '[.chain[]|select(.originKind=="SOURCE" and (.sourceRefs|index($ref))!=null)]|length' <<< "${provenance}")"
[[ "${binlog_provenance_count}" == "1" ]] || fail "updated profile lacks exact binlog provenance"

"${checkpoint_bin}" --event "${event_path}" --checkpoint "${checkpoint_path}" \
  --advance-stream-cursor "${first_cursor}" --catalog-commit "${new_commit}" >/dev/null
stored_position="$(jq -r '.position' "${checkpoint_path}")"
duplicate_decision="$(${checkpoint_bin} --event "${event_path}" --checkpoint "${checkpoint_path}" | jq -r '.decision')"
duplicate_append="$(${kc_bin} append --home "${home_dir}" --command-id "${append_command_id}" \
  --repo "${repository_id}" --stream mysql-binlog --event-id "${event_id}" --event-type mysql.row.UPDATE \
  --payload "${event_payload}")"
duplicate_disposition="$(jq -r '.disposition' <<< "${duplicate_append}")"
duplicate_cursor="$(jq -r '.result.cursor' <<< "${duplicate_append}")"
head_after_replay="$(${kc_bin} resolve --home "${home_dir}" --repo "${repository_id}" --ref refs/heads/main \
  --object "${profile_object}" --aspect profile | jq -r '.commit')"

old_position="$((event_position-1))"
jq --arg id "${event_file}:${old_position}" --argjson position "${old_position}" '.eventId=$id|.position=$position' \
  "${event_path}" > "${old_event_path}"
set +e
old_error="$(${checkpoint_bin} --event "${old_event_path}" --checkpoint "${checkpoint_path}" 2>&1)"
old_rc=$?
set -e
[[ "${old_rc}" != "0" ]] || fail "old binlog position was accepted"
old_error_code="$(jq -r '.error.code' <<< "${old_error}")"
[[ "${old_error_code}" == "POSITION_REGRESSION" ]] || fail "old position error was ${old_error_code}"

stream_slice="$(${kc_bin} stream --home "${home_dir}" --repo "${repository_id}" --stream mysql-binlog)"
stream_cursor="$(jq -r '.cursor' <<< "${stream_slice}")"
record_count="$(jq '.records|length' <<< "${stream_slice}")"
object_count="$(${kc_bin} list --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" | jq 'length')"

q5_tsv="${state_dir}/q5.tsv"
mysql_query "
SELECT n.n_name, CAST(CAST(ROUND(SUM(l.l_extendedprice*(1-l.l_discount)),4) AS DECIMAL(20,4)) AS CHAR)
FROM customer c JOIN orders o ON c.c_custkey=o.o_custkey
JOIN lineitem l ON l.l_orderkey=o.o_orderkey JOIN supplier s ON l.l_suppkey=s.s_suppkey
JOIN nation n ON c.c_nationkey=n.n_nationkey AND s.s_nationkey=n.n_nationkey
JOIN region r ON n.n_regionkey=r.r_regionkey
WHERE r.r_name='ASIA' AND o.o_orderdate>='1994-01-01' AND o.o_orderdate<'1995-01-01'
GROUP BY n.n_name ORDER BY SUM(l.l_extendedprice*(1-l.l_discount)) DESC;" > "${q5_tsv}"
q5_rows="$(jq -Rn '[inputs|split("\t")|{nation:.[0],revenue:.[1]}]' < "${q5_tsv}")"
q5_total="$(jq -r '[.[].revenue|tonumber]|add|@text' <<< "${q5_rows}")"
q5_total="$(printf '%.4f' "${q5_total}")"
q5_line_count="$(mysql_query "
SELECT COUNT(*) FROM customer c JOIN orders o ON c.c_custkey=o.o_custkey
JOIN lineitem l ON l.l_orderkey=o.o_orderkey JOIN supplier s ON l.l_suppkey=s.s_suppkey
JOIN nation n ON c.c_nationkey=n.n_nationkey AND s.s_nationkey=n.n_nationkey
JOIN region r ON n.n_regionkey=r.r_regionkey
WHERE r.r_name='ASIA' AND o.o_orderdate>='1994-01-01' AND o.o_orderdate<'1995-01-01';")"

profile_row_count="$(jq -r '.profile.rowCount' "${updated_snapshot}")"
profile_ndv="$(jq -r '.profile.ndv' "${updated_snapshot}")"
profile_min="$(jq -r '.profile.minValue' "${updated_snapshot}")"
profile_max="$(jq -r '.profile.maxValue' "${updated_snapshot}")"
profile_avg="$(jq -r '.profile.avgValue' "${updated_snapshot}")"
count04="$(jq -r '.profile.distribution[]|select(.value=="0.04")|.rowCount' "${updated_snapshot}")"
count05="$(jq -r '.profile.distribution[]|select(.value=="0.05")|.rowCount' "${updated_snapshot}")"

jq -n \
  --arg binlog_file "${event_file}" --argjson start_position "${start_position}" --argjson event_position "${event_position}" \
  --arg first_decision "${first_decision}" --argjson stored_position "${stored_position}" \
  --arg duplicate_decision "${duplicate_decision}" --argjson old_position "${old_position}" --arg old_error_code "${old_error_code}" \
  --arg first_disposition "${first_disposition}" --arg duplicate_disposition "${duplicate_disposition}" \
  --arg first_cursor "${first_cursor}" --arg duplicate_cursor "${duplicate_cursor}" --argjson record_count "${record_count}" \
  --arg catalog_disposition "${catalog_disposition}" --argjson object_count "${object_count}" \
  --argjson binlog_provenance_count "${binlog_provenance_count}" --argjson commit_stable "$([[ "${head_after_replay}" == "${new_commit}" ]] && echo true || echo false)" \
  --argjson row_count "${profile_row_count}" --argjson ndv "${profile_ndv}" --arg min "${profile_min}" --arg max "${profile_max}" \
  --arg avg "${profile_avg}" --argjson count04 "${count04}" --argjson count05 "${count05}" \
  --argjson q5_rows "${q5_rows}" --arg q5_total "${q5_total}" --argjson q5_line_count "${q5_line_count}" '{
    fixture:"tpch-sf001",
    mutation:{table:"lineitem",key:{l_orderkey:131,l_linenumber:3},column:"l_discount",before:"0.04",after:"0.05",affectedRows:1,binlogFile:$binlog_file,startPosition:$start_position,eventPosition:$event_position},
    checkpoint:{firstDecision:$first_decision,storedPosition:$stored_position,duplicateDecision:$duplicate_decision,oldPosition:$old_position,oldErrorCode:$old_error_code},
    stream:{firstDisposition:$first_disposition,duplicateDisposition:$duplicate_disposition,cursorAfterFirst:$first_cursor,cursorAfterDuplicate:$duplicate_cursor,recordCount:$record_count},
    catalog:{updated:1,unchanged:7,operationCount:1,commitDisposition:$catalog_disposition,emptyReconcileAfterCommit:true,commitStableAfterReplay:$commit_stable,objectCount:$object_count,binlogProvenanceCount:$binlog_provenance_count},
    profile:{rowCount:$row_count,ndv:$ndv,minValue:$min,maxValue:$max,avgValue:$avg,count04:$count04,count05:$count05},
    q5:{rows:$q5_rows,total:$q5_total,lineCount:$q5_line_count}
  }' > "${actual_path}"

if ! diff -u <(jq -S . "${expected_path}") <(jq -S . "${actual_path}"); then
  fail "actual output differs from ${expected_path}; actual: ${actual_path}"
fi

echo "DW-03 PASS: real binlog update applied once, replayed once, rejected at old position, and changed profile/Q5 exactly"
echo "actual: ${actual_path}"
