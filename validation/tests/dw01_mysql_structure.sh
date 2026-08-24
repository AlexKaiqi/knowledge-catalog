#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_dir="$(cd "${validation_dir}/.." && pwd)"
fixture_dir="${validation_dir}/fixtures/tpch-sf001"
cache_dir="${repo_dir}/.data/datawarehouse"
state_dir="${cache_dir}/dw01"
import_dir="${cache_dir}/mysql-import"
tools_dir="${cache_dir}/tools"
compose_file="${validation_dir}/runtime/compose.yaml"
expected_path="${fixture_dir}/expected/dw01.json"
actual_path="${cache_dir}/actual/dw01.json"
source_ref="mysql://127.0.0.1:13306/tpch"
repository_id="kr://tpch/public/physical"
compose_project="kc-dw-validation"
mysql_password="dw-test-root"
go_cache="${TMPDIR:-/tmp}/kc-dw-go-build-cache"
build_mirror=""

compose=(docker compose --project-name "${compose_project}" --file "${compose_file}")

fail() {
  echo "DW-01 FAIL: $*" >&2
  exit 1
}

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

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

mysql_query() {
  local sql="$1"
  "${compose[@]}" exec --no-TTY --env "MYSQL_PWD=${mysql_password}" mysql \
    mysql --user=root --database=tpch --batch --raw --skip-column-names --execute "${sql}"
}

require_command docker
require_command jq
require_command go
docker info >/dev/null 2>&1 || fail "Docker daemon is not running"

if [[ "${KC_DW_DEPENDENCIES_READY:-0}" != "1" ]]; then
  "${validation_dir}/tests/dw00_tpch_oracle.sh" >/dev/null
fi
if [[ -n "${DUCKDB_BIN:-}" ]]; then
  duckdb_bin="${DUCKDB_BIN}"
else
  duckdb_bin="$(find "${tools_dir}" -type f -name duckdb -perm -111 -print | sort | head -n 1)"
fi
[[ -x "${duckdb_bin}" ]] || fail "DuckDB binary was not prepared by DW-00"

case "${state_dir}" in
  "${cache_dir}"/*) rm -rf -- "${state_dir}" ;;
  *) fail "refusing to reset unexpected state path ${state_dir}" ;;
esac
mkdir -p "${state_dir}" "${import_dir}" "${tools_dir}" "${cache_dir}/actual" "${go_cache}"

database_path="${cache_dir}/tpch-sf001.duckdb"
for table in region nation supplier customer part partsupp orders lineitem; do
  target="${import_dir}/${table}.tbl"
  rm -f -- "${target}"
  "${duckdb_bin}" "${database_path}" \
    "COPY (SELECT * FROM ${table}) TO '${target}' (FORMAT CSV, DELIMITER '|', HEADER false);"
done

"${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
"${compose[@]}" up --detach --wait

mysql_version="$(mysql_query "SELECT VERSION();")"
[[ "${mysql_version}" == "8.4.8" ]] || fail "MySQL version ${mysql_version}, expected 8.4.8"

tables_jsonl="${state_dir}/tables.jsonl"
columns_jsonl="${state_dir}/columns.jsonl"
snapshot_path="${state_dir}/mysql-snapshot.json"
mapping_path="${state_dir}/source-key-map.json"
first_preview="${state_dir}/first-preview.json"
second_preview="${state_dir}/second-preview.json"
observed_path="${state_dir}/observed.json"
observed_jsonl="${state_dir}/observed.jsonl"
home_dir="${state_dir}/kc-home"

mysql_query "
SELECT JSON_OBJECT(
  'tableSchema', TABLE_SCHEMA,
  'tableName', TABLE_NAME,
  'tableType', TABLE_TYPE,
  'engine', ENGINE,
  'tableComment', TABLE_COMMENT,
  'tableCollation', TABLE_COLLATION
)
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = 'tpch'
ORDER BY TABLE_NAME;" > "${tables_jsonl}"

mysql_query "
SELECT JSON_OBJECT(
  'tableSchema', TABLE_SCHEMA,
  'tableName', TABLE_NAME,
  'columnName', COLUMN_NAME,
  'ordinalPosition', ORDINAL_POSITION,
  'columnDefault', COLUMN_DEFAULT,
  'isNullable', IS_NULLABLE,
  'dataType', DATA_TYPE,
  'columnType', COLUMN_TYPE,
  'characterMaximumLength', CHARACTER_MAXIMUM_LENGTH,
  'numericPrecision', NUMERIC_PRECISION,
  'numericScale', NUMERIC_SCALE,
  'columnKey', COLUMN_KEY,
  'extra', EXTRA,
  'columnComment', COLUMN_COMMENT
)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'tpch'
ORDER BY TABLE_NAME, ORDINAL_POSITION;" > "${columns_jsonl}"

captured_at="$(mysql_query "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ');")"
jq -n \
  --slurpfile tables "${tables_jsonl}" \
  --slurpfile columns "${columns_jsonl}" \
  --arg captured_at "${captured_at}" \
  --arg source_ref "${source_ref}" \
  '{capturedAt:$captured_at,sourceRef:$source_ref,tables:$tables,columns:$columns}' > "${snapshot_path}"

table_count="$(jq '.tables | length' "${snapshot_path}")"
column_count="$(jq '.columns | length' "${snapshot_path}")"
[[ "${table_count}" == "8" ]] || fail "metadata table count ${table_count}, expected 8"
[[ "${column_count}" == "61" ]] || fail "metadata column count ${column_count}, expected 61"

kc_bin="${tools_dir}/kc-dw01"
connector_bin="${tools_dir}/mysql-structure"
build_mirror="$(mktemp -d "${TMPDIR:-/tmp}/kc-dw-build.XXXXXX")"
(
  cd "${repo_dir}"
  git ls-files --cached --others --exclude-standard -z \
    | xargs -0 -P 20 -n 1 sh -c '
        dest_root="$1"
        source_file="$2"
        [ -f "${source_file}" ] || exit 0
        mkdir -p "${dest_root}/$(dirname "${source_file}")"
        cp -p "${source_file}" "${dest_root}/${source_file}"
      ' sh "${build_mirror}"
)
(
  cd "${build_mirror}"
  GOCACHE="${go_cache}" go build -o "${kc_bin}" ./cmd/kc
  GOCACHE="${go_cache}" go build -o "${connector_bin}" ./validation/cmd/mysql-structure
)

"${kc_bin}" init --home "${home_dir}" --catalog kr://tpch/catalog >/dev/null
repo_add="$(${kc_bin} repo-add --home "${home_dir}" --repo "${repository_id}")"
root_commit="$(jq -r '.head' <<< "${repo_add}")"
[[ -n "${root_commit}" && "${root_commit}" != "null" ]] || fail "repo-add returned no head"

"${connector_bin}" \
  --snapshot "${snapshot_path}" \
  --mapping "${mapping_path}" \
  --base "${root_commit}" \
  --produced-at "2026-08-23T00:00:00Z" \
  --out "${first_preview}"

jq -e '
  .summary == {added:69,updated:0,removed:0,unchanged:0,ignored:0}
  and (.empty == false)
  and (.changeSet.operations | length == 69)
  and ([.changeSet.operations[].address | @json] | unique | length == 69)
' "${first_preview}" >/dev/null || fail "first connector preview did not add 69 unique Addresses"

mapping_entries="$(jq '.objects | length' "${mapping_path}")"
unique_object_ids="$(jq '[.objects[]] | unique | length' "${mapping_path}")"
fqn_leaks="$(jq '[.objects[] | select(test("customer|lineitem|nation|orders|part|partsupp|region|supplier"))] | length' "${mapping_path}")"
[[ "${mapping_entries}" == "69" && "${unique_object_ids}" == "69" && "${fqn_leaks}" == "0" ]] \
  || fail "identity map entries=${mapping_entries} unique=${unique_object_ids} fqnLeaks=${fqn_leaks}"

commit_receipt="$(${kc_bin} commit \
  --home "${home_dir}" \
  --command-id connector:mysql-tpch-structure:dw01-initial \
  --changeset "${first_preview}")"
disposition="$(jq -r '.disposition' <<< "${commit_receipt}")"
new_commit="$(jq -r '.result.newCommit' <<< "${commit_receipt}")"
[[ "${disposition}" == "APPLIED" && -n "${new_commit}" && "${new_commit}" != "null" ]] \
  || fail "first commit was not APPLIED: ${commit_receipt}"

catalog_list="$(${kc_bin} list --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}")"
catalog_object_count="$(jq 'length' <<< "${catalog_list}")"
[[ "${catalog_object_count}" == "69" ]] || fail "catalog object count ${catalog_object_count}, expected 69"

: > "${observed_jsonl}"
while IFS= read -r address; do
  object_id="$(jq -r '.objectId' <<< "${address}")"
  aspect_name="$(jq -r '.aspectName' <<< "${address}")"
  resolution="$(${kc_bin} resolve \
    --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" \
    --object "${object_id}" --aspect "${aspect_name}")"
  jq -c '{address:.address,digest:.digest}' <<< "${resolution}" >> "${observed_jsonl}"
done < <(jq -c '.changeSet.operations[].address' "${first_preview}")
jq -s '.' "${observed_jsonl}" > "${observed_path}"
[[ "$(jq 'length' "${observed_path}")" == "69" ]] || fail "observed digest count is not 69"

"${connector_bin}" \
  --snapshot "${snapshot_path}" \
  --mapping "${mapping_path}" \
  --observed "${observed_path}" \
  --base "${new_commit}" \
  --produced-at "2026-08-23T00:00:01Z" \
  --out "${second_preview}"
jq -e '
  .summary == {added:0,updated:0,removed:0,unchanged:69,ignored:0}
  and (.empty == true)
  and (.changeSet.operations | length == 0)
' "${second_preview}" >/dev/null || fail "second full reconcile was not empty"

table_object="$(jq -r '.changeSet.operations[] | select(.value.entityType == "table") | .address.objectId' "${first_preview}" | head -n 1)"
column_object="$(jq -r '.changeSet.operations[] | select(.value.entityType == "column") | .address.objectId' "${first_preview}" | head -n 1)"
for object_id in "${table_object}" "${column_object}"; do
  provenance="$(${kc_bin} provenance --home "${home_dir}" --repo "${repository_id}" --commit "${new_commit}" --object "${object_id}")"
  jq -e --arg source_ref "${source_ref}" \
    '.chain[0].originKind == "SOURCE" and .chain[0].sourceRefs[0] == $source_ref' \
    <<< "${provenance}" >/dev/null || fail "SOURCE provenance missing for ${object_id}"
done

table_counts='[]'
for table in customer lineitem nation orders part partsupp region supplier; do
  count="$(mysql_query "SELECT COUNT(*) FROM ${table};")"
  table_counts="$(jq -c --arg table "${table}" --argjson count "${count}" '. + [{table_name:$table,row_count:$count}]' <<< "${table_counts}")"
done
columns_by_table="$(jq '
  [.columns[] | .tableName]
  | group_by(.)
  | map({key:.[0],value:length})
  | from_entries
' "${snapshot_path}")"

jq -n \
  --arg mysql_version "${mysql_version}" \
  --arg source_ref "${source_ref}" \
  --argjson table_counts "${table_counts}" \
  --argjson columns_by_table "${columns_by_table}" \
  --argjson table_count "${table_count}" \
  --argjson column_count "${column_count}" \
  '{
    fixture:"tpch-sf001",
    mysql_version:$mysql_version,
    source_ref:$source_ref,
    table_counts:$table_counts,
    metadata:{table_count:$table_count,column_count:$column_count,columns_by_table:$columns_by_table},
    first_preview:{
      summary:{added:69,updated:0,removed:0,unchanged:0,ignored:0},
      empty:false,operation_count:69,unique_address_count:69
    },
    identity_map:{entry_count:69,unique_object_id_count:69,fqn_leak_count:0},
    commit:{disposition:"APPLIED",catalog_object_count:69},
    provenance:{origin_kind:"SOURCE",source_ref:$source_ref,checked_object_count:2},
    second_preview:{
      summary:{added:0,updated:0,removed:0,unchanged:69,ignored:0},
      empty:true,operation_count:0
    }
  }' > "${actual_path}"

if ! diff -u <(jq -S . "${expected_path}") <(jq -S . "${actual_path}"); then
  fail "actual output differs from ${expected_path}; actual: ${actual_path}"
fi

echo "DW-01 PASS: MySQL SF0.01 produced 8 tables + 61 columns, one SOURCE commit, and an empty second reconcile"
echo "actual: ${actual_path}"
