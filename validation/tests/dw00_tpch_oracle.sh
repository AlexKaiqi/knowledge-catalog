#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_dir="$(cd "${validation_dir}/.." && pwd)"
fixture_dir="${validation_dir}/fixtures/tpch-sf001"
cache_dir="${repo_dir}/.data/datawarehouse"
duckdb_version="1.5.4"
database_path="${cache_dir}/tpch-sf001.duckdb"
actual_path="${cache_dir}/actual/dw00.json"
expected_path="${fixture_dir}/expected/dw00.json"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "DW-00 FAIL: required command not found: $1" >&2
    exit 1
  fi
}

resolve_duckdb() {
  if [[ -n "${DUCKDB_BIN:-}" ]]; then
    if [[ ! -x "${DUCKDB_BIN}" ]]; then
      echo "DW-00 FAIL: DUCKDB_BIN is not executable: ${DUCKDB_BIN}" >&2
      exit 1
    fi
    printf '%s\n' "${DUCKDB_BIN}"
    return
  fi

  local platform checksum
  case "$(uname -s)-$(uname -m)" in
    Darwin-arm64)
      platform="osx-arm64"
      checksum="d6c35195683fd1378e5624b01ca390069d399f8341c38986b7e3dfa0b3470d10"
      ;;
    Darwin-x86_64)
      platform="osx-amd64"
      checksum="36e35ae59f417fb0b7e6c5e0b962f887e2b73ad52efc694b76e71fc57bd35b0a"
      ;;
    Linux-x86_64)
      platform="linux-amd64"
      checksum="1f2fa724fb054b3dbe1a9cbd13de5b76997d850e7087ec762ba88db04e0180cf"
      ;;
    Linux-aarch64|Linux-arm64)
      platform="linux-arm64"
      checksum="377f03fb9f17ab5a78f28f829cbfcb5333da8ab3c2d0788f27694f81df77ed29"
      ;;
    *)
      echo "DW-00 FAIL: unsupported platform $(uname -s)-$(uname -m); set DUCKDB_BIN" >&2
      exit 1
      ;;
  esac

  local install_dir="${cache_dir}/tools/duckdb-${duckdb_version}-${platform}"
  local binary_path="${install_dir}/duckdb"
  if [[ -x "${binary_path}" ]]; then
    printf '%s\n' "${binary_path}"
    return
  fi

  require_command curl
  require_command unzip
  require_command shasum

  local temp_dir archive actual_checksum
  temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/kc-dw00.XXXXXX")"
  archive="${temp_dir}/duckdb.zip"
  curl --fail --location --silent --show-error \
    "https://github.com/duckdb/duckdb/releases/download/v${duckdb_version}/duckdb_cli-${platform}.zip" \
    --output "${archive}"
  actual_checksum="$(shasum -a 256 "${archive}" | awk '{print $1}')"
  if [[ "${actual_checksum}" != "${checksum}" ]]; then
    echo "DW-00 FAIL: DuckDB archive checksum mismatch" >&2
    exit 1
  fi
  mkdir -p "${install_dir}"
  unzip -q "${archive}" -d "${install_dir}"
  chmod +x "${binary_path}"
  rm -f "${archive}"
  rmdir "${temp_dir}"
  printf '%s\n' "${binary_path}"
}

query_json() {
  local sql_file="$1"
  "${duckdb_bin}" -json "${database_path}" < "${sql_file}"
}

require_command jq
mkdir -p "${cache_dir}/actual"
duckdb_bin="$(resolve_duckdb)"

if [[ ! -f "${database_path}" ]]; then
  "${duckdb_bin}" "${database_path}" < "${fixture_dir}/sql/bootstrap.sql" >/dev/null
fi

table_counts="$(query_json "${fixture_dir}/sql/dw00_table_counts.sql")"
discount_profile="$(query_json "${fixture_dir}/sql/dw00_discount_profile.sql")"
discount_distribution="$(query_json "${fixture_dir}/sql/dw00_discount_distribution.sql")"
join_orphans="$(query_json "${fixture_dir}/sql/dw00_join_orphans.sql")"
supporting_counts="$(query_json "${fixture_dir}/sql/dw00_supporting_counts.sql")"
q5="$(query_json "${fixture_dir}/sql/dw00_q5.sql")"
q5_total="$(query_json "${fixture_dir}/sql/dw00_q5_total.sql")"

jq -n \
  --argjson table_counts "${table_counts}" \
  --argjson discount_profile "${discount_profile}" \
  --argjson discount_distribution "${discount_distribution}" \
  --argjson join_orphans "${join_orphans}" \
  --argjson supporting_counts "${supporting_counts}" \
  --argjson q5 "${q5}" \
  --argjson q5_total "${q5_total}" \
  '{
    fixture: "tpch-sf001",
    scale_factor: "0.01",
    table_counts: $table_counts,
    discount_profile: $discount_profile[0],
    discount_distribution: $discount_distribution,
    join_orphans: $join_orphans,
    supporting_counts: $supporting_counts[0],
    q5: $q5,
    q5_total: $q5_total[0].q5_total
  }' > "${actual_path}"

if ! diff -u <(jq -S . "${expected_path}") <(jq -S . "${actual_path}"); then
  echo "DW-00 FAIL: actual output differs from ${expected_path}" >&2
  echo "actual: ${actual_path}" >&2
  exit 1
fi

echo "DW-00 PASS: TPC-H SF0.01 counts, profile, joins, and Q5 match the fixed oracle"
echo "actual: ${actual_path}"
