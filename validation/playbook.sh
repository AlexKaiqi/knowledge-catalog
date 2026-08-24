#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
node="${1:-all}"

run_dw00() {
  "${validation_dir}/tests/dw00_tpch_oracle.sh"
}

run_dw01() {
  "${validation_dir}/tests/dw01_mysql_structure.sh"
}

run_dw02() {
  "${validation_dir}/tests/dw02_mysql_observations.sh"
}

run_dw03() {
  "${validation_dir}/tests/dw03_mysql_cdc.sh"
}

case "${node}" in
  DW-00|dw-00)
    run_dw00
    ;;
  DW-01|dw-01)
    run_dw01
    ;;
  DW-02|dw-02)
    run_dw02
    ;;
  DW-03|dw-03)
    run_dw03
    ;;
  all)
    run_dw00
    run_dw03
    ;;
  *)
    echo "unknown or not-yet-executable validation node: ${node}" >&2
    echo "executable nodes: DW-00 DW-01 DW-02 DW-03" >&2
    exit 2
    ;;
esac
