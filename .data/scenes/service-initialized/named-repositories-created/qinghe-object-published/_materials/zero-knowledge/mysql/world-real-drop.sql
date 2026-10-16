-- 世界 A：真实下降。在 world-base.sql 之后执行。
-- 客户 2（Chen，Hangzhou，WEB）第二周只在 09-07、09-08 下单；其余客户两周不变。
-- 同时删除对应的 sales_mart 行（Hangzhou 每天只有 Chen 一行），两个 GMV 口径保持一致。
SET NAMES utf8mb4;

DELETE i FROM order_items i
JOIN orders o ON o.order_id = i.order_id
WHERE o.customer_id = 2 AND o.order_date >= '2026-09-09';

DELETE FROM orders WHERE customer_id = 2 AND order_date >= '2026-09-09';

DELETE FROM sales_mart WHERE city = 'Hangzhou' AND order_date >= '2026-09-09';
