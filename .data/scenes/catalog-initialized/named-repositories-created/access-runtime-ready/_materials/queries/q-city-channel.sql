SELECT
  c.city,
  c.channel,
  COUNT(DISTINCT o.order_id) AS order_count
FROM shop.orders o
JOIN shop.customers c ON c.customer_id = o.customer_id
GROUP BY c.city, c.channel
ORDER BY c.city, c.channel;
