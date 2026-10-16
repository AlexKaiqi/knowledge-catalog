-- 两套零知识世界的公共基线。由 operator 以 root 在基础种子之上执行，替换订单、明细与 sales_mart；
-- 之后再执行 world-real-drop.sql 或 world-stale-mart.sql 之一。
-- 2026-08-31 至 2026-09-13 两个整周，四位客户每天各下一单，全部 PAID，两周订单完全相同。
-- 每位客户固定买一种商品：Lin、Chen 买 Longjing（Chen 买两份），Wu 买 Gaiwan，Zhao 买 Tieguanyin。
-- 每位客户每天只有一行明细。Hangzhou 只有 Chen、Shenzhen 只有 Wu，sales_mart 的一行对应他们的一天；
-- Shanghai 有 Lin 与 Zhao，两人都买 Tea，在 sales_mart 里每天合成一行。
-- sales_mart 按 refresh_sales_mart 的逻辑完整重建。
SET NAMES utf8mb4;

DELETE FROM sales_mart;
DELETE FROM order_items;
DELETE FROM orders;

INSERT INTO orders (order_id, customer_id, order_status, order_date, pay_amount, remark)
SELECT
  1000 + d.n * 10 + c.customer_id,
  c.customer_id,
  'PAID',
  DATE_ADD('2026-08-31', INTERVAL d.n DAY),
  CASE c.customer_id WHEN 1 THEN 88.00 WHEN 2 THEN 176.00 WHEN 3 THEN 45.00 ELSE 66.00 END,
  'zk'
FROM (
  SELECT 0 AS n UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4
  UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9
  UNION ALL SELECT 10 UNION ALL SELECT 11 UNION ALL SELECT 12 UNION ALL SELECT 13
) d
CROSS JOIN customers c;

INSERT INTO order_items (order_id, line_no, product_id, qty, unit_price, discount)
SELECT
  o.order_id,
  1,
  CASE o.customer_id WHEN 1 THEN 1 WHEN 2 THEN 1 WHEN 3 THEN 4 ELSE 2 END,
  CASE o.customer_id WHEN 2 THEN 2.00 ELSE 1.00 END,
  CASE o.customer_id WHEN 1 THEN 88.00 WHEN 2 THEN 88.00 WHEN 3 THEN 45.00 ELSE 66.00 END,
  0.00
FROM orders o;

INSERT INTO sales_mart (
  order_date, order_status, city, category_name, gmv, order_count, item_count
)
SELECT
  o.order_date,
  o.order_status,
  c.city,
  cat.name,
  SUM(i.qty * i.unit_price * (1 - i.discount)),
  COUNT(DISTINCT o.order_id),
  COUNT(*)
FROM orders o
JOIN order_items i ON i.order_id = o.order_id
JOIN customers c ON c.customer_id = o.customer_id
JOIN products p ON p.product_id = i.product_id
JOIN categories cat ON cat.category_id = p.category_id
GROUP BY o.order_date, o.order_status, c.city, cat.name;
