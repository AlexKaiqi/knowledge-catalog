SET NAMES utf8mb4;

INSERT INTO categories VALUES
  (1, 'Tea'),
  (2, 'Snack'),
  (3, 'Ware');

INSERT INTO customers VALUES
  (1, 'Lin', 'Shanghai', 'APP'),
  (2, 'Chen', 'Hangzhou', 'WEB'),
  (3, 'Wu', 'Shenzhen', 'APP'),
  (4, 'Zhao', 'Shanghai', 'WEB');

INSERT INTO products VALUES
  (1, 1, 'Longjing 50g', 88.00),
  (2, 1, 'Tieguanyin 50g', 66.00),
  (3, 2, 'Sesame candy', 12.00),
  (4, 3, 'Gaiwan', 45.00);

INSERT INTO orders VALUES
  (1, 1, 'PAID', '2026-08-27', 167.20, 'gift box'),
  (2, 1, 'SHIPPED', '2026-08-28', 12.00, 'ok'),
  (3, 2, 'PAID', '2026-08-27', 66.00, 'ok'),
  (4, 2, 'UNPAID', '2026-08-29', 45.00, 'wait pay'),
  (5, 3, 'PAID', '2026-08-30', 57.00, 'ok'),
  (6, 4, 'CANCELLED', '2026-09-01', 88.00, 'changed mind');

INSERT INTO order_items VALUES
  (1, 1, 1, 1.00, 88.00, 0.10),
  (1, 2, 4, 2.00, 45.00, 0.00),
  (2, 1, 3, 1.00, 12.00, 0.00),
  (3, 1, 2, 1.00, 66.00, 0.00),
  (4, 1, 4, 1.00, 45.00, 0.00),
  (5, 1, 3, 1.00, 12.00, 0.00),
  (5, 2, 4, 1.00, 45.00, 0.00),
  (6, 1, 1, 1.00, 88.00, 0.00);

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
