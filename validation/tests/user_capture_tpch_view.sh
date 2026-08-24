#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scene_root="$(cd "${validation_dir}/.." && pwd)"
cache_dir="${scene_root}/.data/datawarehouse"
home_dir="${cache_dir}/dw01/kc-home"
state_dir="${cache_dir}/user-tpch-journey"
kc_bin="${cache_dir}/tools/kc-dw01"
repository_id="kr://tpch/public/physical"
catalog_id="kr://tpch/catalog"
workspace_id="tpch-analyst"
preview_path="${cache_dir}/dw02/first-preview.json"
old_pin_path="${state_dir}/before-source-change.pin.json"
old_read_path="${state_dir}/before-source-change.read.json"
actual_path="${state_dir}/consumer-opens-published-knowledge.json"

fail() { echo "USER-TPCH-CAPTURE FAIL: $*" >&2; exit 1; }

[[ -x "${kc_bin}" ]] || fail "knowledge producer has not prepared kc"
[[ -f "${preview_path}" ]] || fail "knowledge producer has not published observations"
mkdir -p "${state_dir}"

profile_object="$(jq -r '.changeSet.operations[]|select(.address.aspectName=="profile")|.address.objectId' "${preview_path}")"
[[ -n "${profile_object}" && "${profile_object}" != "null" ]] || fail "published profile object is missing"

"${kc_bin}" define-workspace --home "${home_dir}" --catalog "${catalog_id}" \
  --workspace "${workspace_id}" --revision 1 --source "${repository_id}=refs/heads/main" >/dev/null
"${kc_bin}" resolve --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" > "${old_pin_path}"
"${kc_bin}" read --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${old_pin_path}" --object "${profile_object}" > "${old_read_path}"

object_count="$("${kc_bin}" list --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" --pin "${old_pin_path}" | jq 'length')"
provenance="$(${kc_bin} provenance --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${old_pin_path}" --object "${profile_object}")"
commit="$(jq -r '.[0].commit' "${old_read_path}")"
count04="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.04")|.rowCount' "${old_read_path}")"
count05="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.05")|.rowCount' "${old_read_path}")"
source_trace_count="$(jq '[.[].chain[]|select(.originKind=="SOURCE")]|length' <<< "${provenance}")"

[[ "${object_count}" == "69" ]] || fail "consumer saw ${object_count} objects, expected 69"
[[ "${count04}" == "5444" && "${count05}" == "5562" ]] || fail "consumer did not read the published pre-change profile"
[[ "${source_trace_count}" -ge "2" ]] || fail "consumer cannot explain the knowledge source"

jq -n \
  --arg fixture "tpch-sf001" --arg user "knowledge-consumer" \
  --arg goal "Find the published l_discount profile and understand where it came from" \
  --arg workspace "${workspace_id}" --arg object "${profile_object}" --arg commit "${commit}" \
  --argjson object_count "${object_count}" --argjson count04 "${count04}" --argjson count05 "${count05}" \
  --argjson source_trace_count "${source_trace_count}" '{
    fixture:$fixture,user:$user,goal:$goal,outcome:"PASSED",
    entry:{workspace:$workspace,repositoryHiddenFromConsumer:true},
    observed:{object:$object,commit:$commit,objectCount:$object_count,count04:$count04,count05:$count05,sourceTraceCount:$source_trace_count}
  }' > "${actual_path}"

echo "USER-TPCH-CAPTURE PASS: consumer entered by Workspace, read the published profile, and explained its SOURCE provenance"
echo "actual: ${actual_path}"
