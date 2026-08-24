SELECT table_name, row_count
FROM (
    SELECT 'customer' AS table_name, count(*)::BIGINT AS row_count FROM customer
    UNION ALL SELECT 'lineitem', count(*)::BIGINT FROM lineitem
    UNION ALL SELECT 'nation', count(*)::BIGINT FROM nation
    UNION ALL SELECT 'orders', count(*)::BIGINT FROM orders
    UNION ALL SELECT 'part', count(*)::BIGINT FROM part
    UNION ALL SELECT 'partsupp', count(*)::BIGINT FROM partsupp
    UNION ALL SELECT 'region', count(*)::BIGINT FROM region
    UNION ALL SELECT 'supplier', count(*)::BIGINT FROM supplier
)
ORDER BY table_name;
