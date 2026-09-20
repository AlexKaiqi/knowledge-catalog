SELECT
  city,
  category_name,
  CAST(SUM(gmv) AS DECIMAL(15, 2)) AS gmv,
  SUM(order_count) AS order_count
FROM shop.sales_mart
GROUP BY city, category_name
ORDER BY city, category_name;
