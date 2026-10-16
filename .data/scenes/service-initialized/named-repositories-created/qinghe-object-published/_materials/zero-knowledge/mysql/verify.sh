#!/bin/sh
# 校验零知识 MySQL 世界：在已加载基础种子的 shop 库上依次应用账号与两套世界，打印封存答案所需的数字。
# 用法：verify.sh <mysql 客户端前缀>，例如 verify.sh "docker exec -i kc-zk-verify mysql -uroot -pv"
set -eu
M="$1"
D=$(dirname "$0")

weekly='SELECT CASE WHEN order_date < "2026-09-07" THEN "w1" ELSE "w2" END AS wk'

run() { echo "-- $1"; echo "$2" | $M -N shop; }

$M shop < "$D/accounts.sql"

for world in real-drop stale-mart; do
  echo "== $world"
  $M shop < "$D/world-base.sql"
  $M shop < "$D/world-$world.sql"
  run "shop.gmv by week" "$weekly, SUM(i.qty*i.unit_price*(1-i.discount)) FROM orders o JOIN order_items i USING(order_id) GROUP BY wk ORDER BY wk;"
  run "shop.sales.gmv by week" "$weekly, SUM(gmv) FROM sales_mart GROUP BY wk ORDER BY wk;"
  run "shop.gmv by week and city" "$weekly, c.city, SUM(i.qty*i.unit_price*(1-i.discount)) FROM orders o JOIN order_items i USING(order_id) JOIN customers c USING(customer_id) GROUP BY wk, c.city ORDER BY c.city, wk;"
  run "shop.gmv by week and category" "$weekly, cat.name, SUM(i.qty*i.unit_price*(1-i.discount)) FROM orders o JOIN order_items i USING(order_id) JOIN products p USING(product_id) JOIN categories cat USING(category_id) GROUP BY wk, cat.name ORDER BY cat.name, wk;"
  run "sales_mart last date" "SELECT MAX(order_date) FROM sales_mart;"
done

echo "== limited account"
echo 'SELECT COUNT(*) FROM shop.sales_mart;' | $M -N -ukc_zk_limited -pzk-limited 2>&1 | grep -o 'ERROR 1142[^:]*' || true
echo 'SELECT COUNT(*) FROM shop.orders;' | $M -N -ukc_zk_limited -pzk-limited 2>/dev/null
echo "== reader cannot see events"
echo 'SHOW EVENTS FROM shop;' | $M -N -ukc_zk_reader -pzk-read 2>&1 | grep -o 'ERROR [0-9]*' || echo "no error"
