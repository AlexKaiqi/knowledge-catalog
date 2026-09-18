SELECT
  o.order_id,
  o.order_status,
  o.order_date,
  CAST(SUM(i.qty * i.unit_price * (1 - i.discount)) AS DECIMAL(15, 2)) AS gmv
FROM shop.orders o
JOIN shop.order_items i ON i.order_id = o.order_id
WHERE o.order_status IN ('PAID', 'SHIPPED')
GROUP BY o.order_id, o.order_status, o.order_date
ORDER BY o.order_id;
