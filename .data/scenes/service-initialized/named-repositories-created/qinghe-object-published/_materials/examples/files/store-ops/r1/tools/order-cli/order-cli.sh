#!/usr/bin/env bash
# order-cli：下单服务的示例命令行。登记为对象 tool/qinghe.order-cli，随 store-oncall Dataset 交付。
set -euo pipefail

url="${ORDER_SERVICE_URL:-}"
usage() {
  printf '%s\n' 'order-cli 0.1.0' 'usage: order-cli health | get <orderId>' \
    'Set ORDER_SERVICE_URL to the authorized order service endpoint before querying.'
}
usage_error() { usage >&2; exit 2; }

case "${1:-}" in
  --help|-h|help) usage; exit 0 ;;
esac
[[ $# -ge 1 ]] || usage_error
if [[ -z "$url" ]]; then
  echo '{"error":"ORDER_SERVICE_URL is not set; this public example has no live order service"}' >&2
  exit 3
fi
case "$1" in
  health) curl -fsS "$url/health" ;;
  get) [[ $# -eq 2 ]] || usage_error; curl -fsS "$url/orders/$2" ;;
  *) usage_error ;;
esac
