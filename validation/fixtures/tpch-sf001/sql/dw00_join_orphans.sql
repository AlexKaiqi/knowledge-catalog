SELECT relation, orphan_count
FROM (
    SELECT 'orders.customer' AS relation, count(*)::BIGINT AS orphan_count
    FROM orders o LEFT JOIN customer c ON o.o_custkey = c.c_custkey
    WHERE c.c_custkey IS NULL
    UNION ALL
    SELECT 'lineitem.orders', count(*)::BIGINT
    FROM lineitem l LEFT JOIN orders o ON l.l_orderkey = o.o_orderkey
    WHERE o.o_orderkey IS NULL
    UNION ALL
    SELECT 'lineitem.supplier', count(*)::BIGINT
    FROM lineitem l LEFT JOIN supplier s ON l.l_suppkey = s.s_suppkey
    WHERE s.s_suppkey IS NULL
    UNION ALL
    SELECT 'customer.nation', count(*)::BIGINT
    FROM customer c LEFT JOIN nation n ON c.c_nationkey = n.n_nationkey
    WHERE n.n_nationkey IS NULL
    UNION ALL
    SELECT 'supplier.nation', count(*)::BIGINT
    FROM supplier s LEFT JOIN nation n ON s.s_nationkey = n.n_nationkey
    WHERE n.n_nationkey IS NULL
    UNION ALL
    SELECT 'nation.region', count(*)::BIGINT
    FROM nation n LEFT JOIN region r ON n.n_regionkey = r.r_regionkey
    WHERE r.r_regionkey IS NULL
)
ORDER BY relation;
