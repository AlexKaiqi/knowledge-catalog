#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_dir="$(cd "${validation_dir}/.." && pwd)"
cache_dir="${repo_dir}/.data/datawarehouse"
state_dir="${cache_dir}/dw02"
dw01_state="${cache_dir}/dw01"
tools_dir="${cache_dir}/tools"
compose_file="${validation_dir}/runtime/compose.yaml"
compose_project="kc-dw-validation"
mysql_password="dw-test-root"
source_ref="mysql://127.0.0.1:13306/tpch"
repository_id="kr://tpch/public/physical"
expected_path="${validation_dir}/fixtures/tpch-sf001/expected/dw02.json"
actual_path="${cache_dir}/actual/dw02.json"
go_cache="${TMPDIR:-/tmp}/kc-dw-go-build-cache"
build_mirror=""
compose=(docker compose --project-name "${compose_project}" --file "${compose_file}")

fail() { echo "DW-02 FAIL: $*" >&2; exit 1; }

cleanup() {
  local status=$?
  if [[ "${status}" != "0" || "${KC_DW_KEEP_MYSQL:-0}" != "1" ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  if [[ -n "${build_mirror}" && "${build_mirror}" == "${TMPDIR:-/tmp}"/kc-dw-build.* ]]; then
    rm -rf -- "${build_mirror}"
  fi
  return "${status}"
}
trap cleanup EXIT

mysql_query() {
  "${compose[@]}" exec --no-TTY --env "MYSQL_PWD=${mysql_password}" mysql \
    mysql --user=root --database=tpch --batch --raw --skip-column-names --execute "$1"
}

docker info >/dev/null 2>&1 || fail "Docker daemon is not running"
if [[ "${KC_DW_DEPENDENCIES_READY:-0}" != "1" ]]; then
  KC_DW_KEEP_MYSQL=1 "${validation_dir}/tests/dw01_mysql_structure.sh" >/dev/null
fi

case "${state_dir}" in
  "${cache_dir}"/*) rm -rf -- "${state_dir}" ;;
  *) fail "refusing to reset unexpected state path ${state_dir}" ;;
esac
mkdir -p "${state_dir}" "${cache_dir}/actual"

profile_base="${state_dir}/profile-base.json"
distribution_jsonl="${state_dir}/distribution.jsonl"
joins_jsonl="${state_dir}/joins.jsonl"
annotation_path="${state_dir}/annotation.json"
snapshot_path="${state_dir}/observation-snapshot.json"
first_preview="${state_dir}/first-preview.json"
second_preview="${state_dir}/second-preview.json"
observed_jsonl="${state_dir}/observed.jsonl"
observed_path="${state_dir}/observed.json"
mapping_path="${dw01_state}/source-key-map.json"
home_dir="${dw01_state}/kc-home"
kc_bin="${tools_dir}/kc-dw01"
observation_bin="${tools_dir}/mysql-observations"

mysql_query "
SELECT JSON_OBJECT(
  'schema','tpch','table','lineitem','column','l_discount',
  'rowCount',COUNT(*),'ndv',COUNT(DISTINCT l_discount),
  'minValue',CAST(MIN(l_discount) AS CHAR),
  'maxValue',CAST(MAX(l_discount) AS CHAR),
  'avgValue',CAST(CAST(AVG(l_discount) AS DECIMAL(20,8)) AS CHAR)
)
FROM lineitem;" > "${profile_base}"
mysql_query "
SELECT JSON_OBJECT('value',CAST(l_discount AS CHAR),'rowCount',COUNT(*))
FROM lineitem GROUP BY l_discount ORDER BY l_discount;" > "${distribution_jsonl}"

mysql_query "
SELECT JSON_OBJECT('relation','orders.customer','childSchema','tpch','childTable','orders','childColumns',JSON_ARRAY('o_custkey'),'parentSchema','tpch','parentTable','customer','parentColumns',JSON_ARRAY('c_custkey'),'childRowCount',(SELECT COUNT(*) FROM orders),'orphanCount',(SELECT COUNT(*) FROM orders o LEFT JOIN customer c ON o.o_custkey=c.c_custkey WHERE c.c_custkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM orders o LEFT JOIN customer c ON o.o_custkey=c.c_custkey WHERE c.c_custkey IS NULL')
UNION ALL SELECT JSON_OBJECT('relation','lineitem.orders','childSchema','tpch','childTable','lineitem','childColumns',JSON_ARRAY('l_orderkey'),'parentSchema','tpch','parentTable','orders','parentColumns',JSON_ARRAY('o_orderkey'),'childRowCount',(SELECT COUNT(*) FROM lineitem),'orphanCount',(SELECT COUNT(*) FROM lineitem l LEFT JOIN orders o ON l.l_orderkey=o.o_orderkey WHERE o.o_orderkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM lineitem l LEFT JOIN orders o ON l.l_orderkey=o.o_orderkey WHERE o.o_orderkey IS NULL')
UNION ALL SELECT JSON_OBJECT('relation','lineitem.supplier','childSchema','tpch','childTable','lineitem','childColumns',JSON_ARRAY('l_suppkey'),'parentSchema','tpch','parentTable','supplier','parentColumns',JSON_ARRAY('s_suppkey'),'childRowCount',(SELECT COUNT(*) FROM lineitem),'orphanCount',(SELECT COUNT(*) FROM lineitem l LEFT JOIN supplier s ON l.l_suppkey=s.s_suppkey WHERE s.s_suppkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM lineitem l LEFT JOIN supplier s ON l.l_suppkey=s.s_suppkey WHERE s.s_suppkey IS NULL')
UNION ALL SELECT JSON_OBJECT('relation','customer.nation','childSchema','tpch','childTable','customer','childColumns',JSON_ARRAY('c_nationkey'),'parentSchema','tpch','parentTable','nation','parentColumns',JSON_ARRAY('n_nationkey'),'childRowCount',(SELECT COUNT(*) FROM customer),'orphanCount',(SELECT COUNT(*) FROM customer c LEFT JOIN nation n ON c.c_nationkey=n.n_nationkey WHERE n.n_nationkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM customer c LEFT JOIN nation n ON c.c_nationkey=n.n_nationkey WHERE n.n_nationkey IS NULL')
UNION ALL SELECT JSON_OBJECT('relation','supplier.nation','childSchema','tpch','childTable','supplier','childColumns',JSON_ARRAY('s_nationkey'),'parentSchema','tpch','parentTable','nation','parentColumns',JSON_ARRAY('n_nationkey'),'childRowCount',(SELECT COUNT(*) FROM supplier),'orphanCount',(SELECT COUNT(*) FROM supplier s LEFT JOIN nation n ON s.s_nationkey=n.n_nationkey WHERE n.n_nationkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM supplier s LEFT JOIN nation n ON s.s_nationkey=n.n_nationkey WHERE n.n_nationkey IS NULL')
UNION ALL SELECT JSON_OBJECT('relation','nation.region','childSchema','tpch','childTable','nation','childColumns',JSON_ARRAY('n_regionkey'),'parentSchema','tpch','parentTable','region','parentColumns',JSON_ARRAY('r_regionkey'),'childRowCount',(SELECT COUNT(*) FROM nation),'orphanCount',(SELECT COUNT(*) FROM nation n LEFT JOIN region r ON n.n_regionkey=r.r_regionkey WHERE r.r_regionkey IS NULL),'evidenceSql','SELECT COUNT(*) FROM nation n LEFT JOIN region r ON n.n_regionkey=r.r_regionkey WHERE r.r_regionkey IS NULL');" > "${joins_jsonl}"

mysql_query "
SELECT JSON_OBJECT(
  'schema','tpch','table','lineitem','column','l_discount','comment',COLUMN_COMMENT,
  'sourceFragment',CONCAT(COLUMN_NAME,' ',COLUMN_TYPE,' ',IF(IS_NULLABLE='NO','NOT NULL','NULL'),' COMMENT ',QUOTE(COLUMN_COMMENT))
)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA='tpch' AND TABLE_NAME='lineitem' AND COLUMN_NAME='l_discount';" > "${annotation_path}"

captured_at="$(mysql_query "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ');")"
jq -n \
  --slurpfile profile "${profile_base}" --slurpfile distribution "${distribution_jsonl}" \
  --slurpfile joins "${joins_jsonl}" --slurpfile annotation "${annotation_path}" \
  --arg captured_at "${captured_at}" --arg source_ref "${source_ref}" \
  '{capturedAt:$captured_at,sourceRef:$source_ref,profile:($profile[0]+{distribution:$distribution}),joins:$joins,annotation:$annotation[0]}' \
  > "${snapshot_path}"

table_probe_key="${source_ref}/table/tpch/lineitem"
table_probe="$(jq -r --arg key "${table_probe_key}" '.objects[$key]' "${mapping_path}")"
base_resolution="$(${kc_bin} resolve --home "${home_dir}" --repo "${repository_id}" --ref refs/heads/main --object "${table_probe}" --aspect structure)"
base_commit="$(jq -r '.commit' <<< "${base_resolution}")"

build_mirror="$(mktemp -d "${TMPDIR:-/tmp}/kc-dw-build.XXXXXX")"
(
  cd "${repo_dir}"
  # This translator only imports connector -> repository/kernel.  Keeping the
  # mirror minimal avoids hydrating unrelated iCloud-backed workspace files.
  git ls-files --cached --others --exclude-standard -z -- \
    go.mod go.sum connector kernel repository \
    validation/cmd/mysql-observations \
    | xargs -0 -P 20 -n 1 sh -c '
        dest_root="$1"; source_file="$2"; [ -f "${source_file}" ] || exit 0
        mkdir -p "${dest_root}/$(dirname "${source_file}")"
        cp -p "${source_file}" "${dest_root}/${source_file}"
      ' sh "${build_mirror}"
)
(cd "${build_mirror}" && GOCACHE="${go_cache}" go build -o "${observation_bin}" ./validation/cmd/mysql-observations)

"${observation_bin}" --snapshot "${snapshot_path}" --mapping "${mapping_path}" --base "${base_commit}" --produced-at "2026-08-23T00:01:00Z" --out "${first_preview}"
jq -e '
  .summary == {added:8,updated:0,removed:0,unchanged:0,ignored:0}
  and (.changeSet.operations|length == 8)
  and ([.changeSet.operations[]|select(.address.aspectName=="profile")]|length == 1)
  and ([.changeSet.operations[]|select(.address.kind=="Member" and .address.aspectName=="joinEvidence")]|length == 6)
  and ([.changeSet.operations[]|select(.address.aspectName=="annotation")]|length == 1)
' "${first_preview}" >/dev/null || fail "first observation preview did not produce 1 profile + 6 joins + 1 annotation"

receipt="$(${kc_bin} commit --home "${home_dir}" --command-id connector:mysql-tpch-observations:dw02-initial --changeset "${first_preview}")"
disposition="$(jq -r '.disposition' <<< "${receipt}")"
new_commit="$(jq -r '.result.newCommit' <<< "${receipt}")"
[[ "${disposition}" == "APPLIED" ]] || fail "observation commit was not APPLIED"

: > "${observed_jsonl}"
while IFS= read -r address; do
  object_id="$(jq -r '.objectId' <<< "${address}")"
  aspect_name="$(jq -r '.aspectName' <<< "${address}")"
  member_key="$(jq -r '.memberKey // empty' <<< "${address}")"
  args=(resolve --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" --object "${object_id}" --aspect "${aspect_name}")
  [[ -z "${member_key}" ]] || args+=(--member "${member_key}")
  resolution="$(${kc_bin} "${args[@]}")"
  jq -c '{address:.address,digest:.digest}' <<< "${resolution}" >> "${observed_jsonl}"
done < <(jq -c '.changeSet.operations[].address' "${first_preview}")
jq -s '.' "${observed_jsonl}" > "${observed_path}"

"${observation_bin}" --snapshot "${snapshot_path}" --mapping "${mapping_path}" --observed "${observed_path}" --base "${new_commit}" --produced-at "2026-08-23T00:01:01Z" --out "${second_preview}"
jq -e '.summary == {added:0,updated:0,removed:0,unchanged:8,ignored:0} and .empty and (.changeSet.operations|length==0)' "${second_preview}" >/dev/null \
  || fail "second observation reconcile was not empty"

profile_object="$(jq -r '.changeSet.operations[]|select(.address.aspectName=="profile")|.address.objectId' "${first_preview}")"
join_object="$(jq -r '.changeSet.operations[]|select(.address.aspectName=="joinEvidence")|.address.objectId' "${first_preview}" | head -n 1)"
for object_id in "${profile_object}" "${join_object}"; do
  provenance="$(${kc_bin} provenance --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" --object "${object_id}")"
  jq -e --arg info "${source_ref}/information_schema" --arg query "${source_ref}/query/dw02" '
    any(.chain[]; .originKind=="SOURCE" and (.sourceRefs|index($info)) != null and (.sourceRefs|index($query)) != null)
  ' <<< "${provenance}" >/dev/null || fail "observation SOURCE provenance missing for ${object_id}"
done

catalog_count="$(${kc_bin} list --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" | jq 'length')"
[[ "${catalog_count}" == "69" ]] || fail "observation aspects created extra entity identities"

jq -n --slurpfile snapshot "${snapshot_path}" '{
  fixture:"tpch-sf001",
  profile:$snapshot[0].profile,
  joins:($snapshot[0].joins|map({relation,childRowCount,orphanCount})|sort_by(.relation)),
  annotation:$snapshot[0].annotation,
  first_preview:{added:8,operation_count:8,profile_count:1,join_member_count:6,annotation_count:1},
  commit:{disposition:"APPLIED",catalog_object_count:69},
  provenance:{checked_object_count:2,source_ref_count:2},
  second_preview:{unchanged:8,operation_count:0,digest_stable_count:8,empty:true}
}' > "${actual_path}"

if ! diff -u <(jq -S . "${expected_path}") <(jq -S . "${actual_path}"); then
  fail "actual output differs from ${expected_path}; actual: ${actual_path}"
fi

echo "DW-02 PASS: profile, six join evidences, annotation, provenance, and eight stable digests match the oracle"
echo "actual: ${actual_path}"
