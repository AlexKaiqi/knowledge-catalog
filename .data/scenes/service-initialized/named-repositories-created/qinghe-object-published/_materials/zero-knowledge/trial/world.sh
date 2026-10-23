#!/usr/bin/env bash
# 零知识试跑的世界准备。operator 执行，不属于 Agent 可见过程；只绑定已部署的 local 走查栈，不启动 KC 服务。
#
#   consumer          在 goto qinghe-object-published 之后：zk-index、qinghe-analysis、qinghe-ops、消费身份授权、MySQL 账号
#   supplier          在 goto named-repositories-created 之后：起墙外 MySQL、接入方身份授权、MySQL 账号
#   supplier-finish   接入方交付后：核对并准备已发表 Dataset 修订的检索投影（运维动作，不交给接入方 Agent）
#   mysql <world>     每次运行前重置 MySQL：seed（基础种子）| stale-mart | real-drop
#   check             以消费身份核对世界可用，只读
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ZK="$(dirname "$HERE")"
NODE="$(cd "$ZK/../.." && pwd)"
REPO_ROOT="$(cd "$NODE/../../../../.." && pwd)"
SEED_DIR="$REPO_ROOT/.data/scenes/service-initialized/source-repositories-configured/repository-attached/_materials/accessor/mysql/init"
INDEX_SRC="$NODE/_materials/acceptance/sales-analysis/indexes/sales-search"

CLI_IMAGE="${KC_DEPLOY_CLI_IMAGE:-kc-deploy-local-cli:local}"
NETWORK="${KC_ACCESSOR_NETWORK:-kc-deploy-local_default}"
MYSQL_CONTAINER="${KC_ZK_MYSQL_CONTAINER:-kc-scene-accessor-mysql-1}"
CONSUMER="${KC_ZK_CONSUMER:-zk-colleague}"
SUPPLIER="${KC_ZK_SUPPLIER:-zk-supplier}"
SHARED_DATASET="${KC_ZK_SHARED_DATASET:-qinghe-shared}"

kc_as() {
  local principal="$1"
  shift
  docker run --rm --network "$NETWORK" -v "$REPO_ROOT:$REPO_ROOT:ro" -v "${STAGE:-/tmp}:${STAGE:-/tmp}:ro" \
    -e KC_SERVER_URL=http://kc-server:7380 -e "KC_AS=$principal" \
    --entrypoint /usr/local/bin/kc "$CLI_IMAGE" "$@"
}

kc() {
  echo "+ kc $*" >&2
  kc_as admin "$@"
}

mysql_root() {
  docker exec -i "$MYSQL_CONTAINER" mysql -uroot -pshop-walk shop 2>/dev/null
}

require_stack() {
  docker network inspect "$NETWORK" >/dev/null 2>&1 || { echo "FAIL: $NETWORK 不存在，先 make deploy-local-up" >&2; exit 1; }
  docker image inspect "$CLI_IMAGE" >/dev/null 2>&1 || { echo "FAIL: 缺少 $CLI_IMAGE" >&2; exit 1; }
}

require_mysql() {
  docker inspect "$MYSQL_CONTAINER" >/dev/null 2>&1 || { echo "FAIL: $MYSQL_CONTAINER 不在运行；consumer 世界由 goto 起墙外 compose，supplier 世界由本脚本起" >&2; exit 1; }
}

mysql_accounts() {
  require_mysql
  mysql_root < "$ZK/mysql/accounts.sql"
}

cmd_mysql() {
  require_mysql
  case "${1:-}" in
    seed)
      printf 'SET FOREIGN_KEY_CHECKS=0;\nDELETE FROM sales_mart; DELETE FROM order_items; DELETE FROM orders;\nDELETE FROM products; DELETE FROM customers; DELETE FROM categories;\nSET FOREIGN_KEY_CHECKS=1;\n' | mysql_root
      mysql_root < "$SEED_DIR/01-seed.sql"
      ;;
    stale-mart|real-drop)
      mysql_root < "$ZK/mysql/world-base.sql"
      mysql_root < "$ZK/mysql/world-$1.sql"
      ;;
    *) echo "usage: $0 mysql seed|stale-mart|real-drop" >&2; exit 2 ;;
  esac
  echo "mysql world: $1"
  echo 'SELECT COUNT(*) AS orders, (SELECT MAX(order_date) FROM sales_mart) AS mart_last FROM orders;' | mysql_root
}

cmd_consumer() {
  require_stack
  STAGE="$(mktemp -d)"
  trap 'rm -rf "$STAGE"' EXIT
  mkdir -p "$STAGE/indexes/sales-search"
  for member in table-name metric-text metric-unit; do
    cp "$INDEX_SRC/$member.yaml" "$STAGE/indexes/sales-search/"
  done
  chmod -R a+rX "$STAGE"

  kc create --name zk-index --store lakefs
  kc attach --repo zk-index
  kc writer commit --command-id zk-index-initial --repo zk-index --dir "$STAGE"

  kc dataset define --dataset qinghe-analysis --revision 1 \
    --source table-meta=refs/heads/main@tables@tables \
    --source table-meta=refs/heads/main@columns@columns \
    --source table-meta=refs/heads/main@databases@databases \
    --source table-meta=refs/heads/main@database-schemas@database-schemas \
    --source table-meta=refs/heads/main@data-platform-instances@data-platform-instances \
    --source table-meta=refs/heads/main@data-jobs@data-jobs \
    --source table-meta=refs/heads/main@relations-table-meta@relations \
    --source sales-semantic=refs/heads/main@semantic-models@semantic-models \
    --source sales-semantic=refs/heads/main@metrics@metrics \
    --source sales-semantic=refs/heads/main@relations-sales-semantic@relations \
    --source zk-index=refs/heads/main@indexes@indexes

  # 消费身份无权的对照 Dataset，供 zk-env-find-usable 检查“不列无权的 Dataset”。
  kc dataset define --dataset qinghe-ops --revision 1 \
    --source table-meta=refs/heads/main@data-jobs@data-jobs

  kc index prepare --dataset qinghe-analysis --revision 1

  kc grant add --dataset qinghe-analysis --principal "$CONSUMER" --action dataset.resolve,file.read
  kc grant add --dataset qinghe-analysis --principal "$CONSUMER" --action feedback.write
  kc grant add --principal "$CONSUMER" --action inventory.read

  mysql_accounts
  echo "consumer world ready: principal $CONSUMER, dataset qinghe-analysis"
}

cmd_supplier() {
  require_stack
  KC_REPOSITORY=table-meta KC_AS=admin "$REPO_ROOT/scripts/deploy/accessors.sh" up
  local read_write="object.read,object.schema.read,object.relations,object.provenance,object.history.read,writer.preview,writer.commit,writer.branch,writer.merge,writer.receipt.read"
  for repo in table-meta sales-semantic; do
    kc grant add --repo "$repo" --principal "$SUPPLIER" --action "$read_write"
    kc grant add --repo "$repo" --dataset "$SHARED_DATASET" --ref refs/heads/main --principal "$SUPPLIER" --action repository.dataset.publish
  done
  kc grant add --dataset "$SHARED_DATASET" --principal "$SUPPLIER" --action dataset.manage,dataset.resolve,file.read,admin.grants.manage
  kc grant add --principal "$SUPPLIER" --action inventory.read
  kc grant add --principal "$CONSUMER" --action inventory.read
  mysql_accounts
  echo "supplier world ready: principal $SUPPLIER, delivery dataset $SHARED_DATASET, colleague $CONSUMER"
}

cmd_supplier_finish() {
  require_stack
  kc index prepare --dataset "$SHARED_DATASET"
}

cmd_check() {
  require_stack
  local dataset="${1:-qinghe-analysis}"
  echo "+ ($CONSUMER) kc show" >&2
  kc_as "$CONSUMER" show
  echo "+ ($CONSUMER) kc search --dataset $dataset --query merchandise" >&2
  kc_as "$CONSUMER" search --dataset "$dataset" --query merchandise || echo "WARN: Dataset 检索未就绪；试跑会把它当观察记录，不是夹具通过" >&2
  require_mysql
  echo 'SELECT COUNT(*) FROM shop.orders;' | docker exec -i "$MYSQL_CONTAINER" mysql -N -ukc_zk_reader -pzk-read 2>/dev/null
}

case "${1:-}" in
  consumer) cmd_consumer ;;
  supplier) cmd_supplier ;;
  supplier-finish) cmd_supplier_finish ;;
  mysql) shift; cmd_mysql "$@" ;;
  check) shift; cmd_check "$@" ;;
  *) echo "usage: $0 consumer|supplier|supplier-finish|mysql <world>|check [dataset]" >&2; exit 2 ;;
esac
