SELECT
    l_discount::DECIMAL(4,2) AS discount,
    count(*)::BIGINT AS row_count
FROM lineitem
GROUP BY l_discount
ORDER BY l_discount;
