#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scene_root="$(cd "${validation_dir}/.." && pwd)"
cache_dir="${scene_root}/.data/datawarehouse"
home_dir="${cache_dir}/dw01/kc-home"
state_dir="${cache_dir}/user-tpch-journey"
kc_bin="${cache_dir}/tools/kc-dw01"
catalog_id="kr://tpch/catalog"
workspace_id="tpch-analyst"
preview_path="${cache_dir}/dw02/first-preview.json"
old_pin_path="${state_dir}/before-source-change.pin.json"
old_read_path="${state_dir}/replayed-before-source-change.read.json"
fresh_pin_path="${state_dir}/after-source-change.pin.json"
fresh_read_path="${state_dir}/after-source-change.read.json"
actual_path="${state_dir}/source-change-to-reproducible-consumption.json"

fail() { echo "USER-TPCH-COMPARE FAIL: $*" >&2; exit 1; }

[[ -x "${kc_bin}" ]] || fail "knowledge producer has not prepared kc"
[[ -f "${old_pin_path}" ]] || fail "consumer has no saved pre-change Workspace pin"
[[ -f "${cache_dir}/actual/dw03.json" ]] || fail "knowledge producer has not published the source change"

profile_object="$(jq -r '.changeSet.operations[]|select(.address.aspectName=="profile")|.address.objectId' "${preview_path}")"
"${kc_bin}" read --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${old_pin_path}" --object "${profile_object}" > "${old_read_path}"
"${kc_bin}" resolve --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" > "${fresh_pin_path}"
"${kc_bin}" read --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${fresh_pin_path}" --object "${profile_object}" > "${fresh_read_path}"
fresh_provenance="$(${kc_bin} provenance --home "${home_dir}" --catalog "${catalog_id}" --workspace "${workspace_id}" \
  --pin "${fresh_pin_path}" --object "${profile_object}")"

old_commit="$(jq -r '.[0].commit' "${old_read_path}")"
fresh_commit="$(jq -r '.[0].commit' "${fresh_read_path}")"
old04="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.04")|.rowCount' "${old_read_path}")"
old05="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.05")|.rowCount' "${old_read_path}")"
fresh04="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.04")|.rowCount' "${fresh_read_path}")"
fresh05="$(jq -r '.[0].value.profile.distribution[]|select(.value=="0.05")|.rowCount' "${fresh_read_path}")"
binlog_trace_count="$(jq '[.[].chain[]|select(.originKind=="SOURCE" and any(.sourceRefs[]?; contains("/binlog/")))]|length' <<< "${fresh_provenance}")"

[[ "${old_commit}" != "${fresh_commit}" ]] || fail "fresh consumer did not resolve the published update"
[[ "${old04}" == "5444" && "${old05}" == "5562" ]] || fail "saved pin no longer reproduces the old result"
[[ "${fresh04}" == "5443" && "${fresh05}" == "5563" ]] || fail "fresh consumer did not see the source update"
[[ "${binlog_trace_count}" == "1" ]] || fail "fresh result cannot be traced to the binlog update"

jq -n \
  --arg fixture "tpch-sf001" --arg producer "knowledge-producer" --arg consumer "knowledge-consumer" \
  --arg goal "Publish a source change so new work sees it while an in-flight analysis remains reproducible" \
  --arg workspace "${workspace_id}" --arg object "${profile_object}" \
  --arg old_commit "${old_commit}" --arg fresh_commit "${fresh_commit}" \
  --argjson old04 "${old04}" --argjson old05 "${old05}" --argjson fresh04 "${fresh04}" --argjson fresh05 "${fresh05}" \
  --argjson binlog_trace_count "${binlog_trace_count}" '{
    fixture:$fixture,users:[$producer,$consumer],goal:$goal,outcome:"PASSED",
    entry:{workspace:$workspace,repositoryHiddenFromConsumer:true},
    before:{commit:$old_commit,count04:$old04,count05:$old05},
    after:{commit:$fresh_commit,count04:$fresh04,count05:$fresh05,binlogTraceCount:$binlog_trace_count},
    invariants:{savedPinReproducible:true,freshTaskSeesPublishedUpdate:true,canonicalProvenanceExplainsChange:true}
  }' > "${actual_path}"

echo "USER-TPCH-COMPARE PASS: old analysis reproduced V1; fresh Workspace resolve read V2 with binlog provenance"
echo "actual: ${actual_path}"
