SELECT CASE
  WHEN src.gmv = mart.gmv THEN 'MATCH'
  ELSE 'DRIFT'
END
FROM (
  SELECT CAST(SUM(qty * unit_price * (1 - discount)) AS DECIMAL(15, 2)) AS gmv
  FROM shop.order_items
) src
CROSS JOIN (
  SELECT CAST(SUM(gmv) AS DECIMAL(15, 2)) AS gmv
  FROM shop.sales_mart
) mart;
