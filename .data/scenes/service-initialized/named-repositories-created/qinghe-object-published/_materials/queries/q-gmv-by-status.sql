SELECT
  o.order_status,
  CAST(SUM(i.qty * i.unit_price * (1 - i.discount)) AS DECIMAL(15, 2)) AS gmv,
  COUNT(*) AS item_count
FROM shop.orders o
JOIN shop.order_items i ON i.order_id = o.order_id
GROUP BY o.order_status
ORDER BY o.order_status;
