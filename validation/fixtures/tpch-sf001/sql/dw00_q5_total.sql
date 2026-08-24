SELECT cast(round(sum(l.l_extendedprice * (1 - l.l_discount)), 4) AS DECIMAL(18,4)) AS q5_total
FROM customer c
JOIN orders o ON c.c_custkey = o.o_custkey
JOIN lineitem l ON l.l_orderkey = o.o_orderkey
JOIN supplier s ON l.l_suppkey = s.s_suppkey
JOIN nation n ON s.s_nationkey = n.n_nationkey AND c.c_nationkey = s.s_nationkey
JOIN region r ON n.n_regionkey = r.r_regionkey
WHERE r.r_name = 'ASIA'
  AND o.o_orderdate >= DATE '1994-01-01'
  AND o.o_orderdate < DATE '1995-01-01';
