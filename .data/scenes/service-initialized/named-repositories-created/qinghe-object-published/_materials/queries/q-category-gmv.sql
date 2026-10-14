SELECT
  cat.name,
  CAST(SUM(i.qty * i.unit_price * (1 - i.discount)) AS DECIMAL(15, 2)) AS gmv,
  COUNT(*) AS item_count
FROM shop.order_items i
JOIN shop.products p ON p.product_id = i.product_id
JOIN shop.categories cat ON cat.category_id = p.category_id
GROUP BY cat.name
ORDER BY cat.name;
