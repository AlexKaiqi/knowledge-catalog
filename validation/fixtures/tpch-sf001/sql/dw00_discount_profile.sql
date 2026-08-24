SELECT
    count(*)::BIGINT AS row_count,
    count(DISTINCT l_discount)::BIGINT AS ndv,
    min(l_discount)::DECIMAL(4,2) AS min_value,
    max(l_discount)::DECIMAL(4,2) AS max_value,
    round(avg(l_discount), 8)::DECIMAL(10,8) AS avg_value
FROM lineitem;
