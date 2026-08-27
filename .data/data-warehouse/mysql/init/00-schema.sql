SET NAMES utf8mb4;

CREATE TABLE region (
  r_regionkey INTEGER NOT NULL,
  r_name CHAR(25) NOT NULL,
  r_comment VARCHAR(152) NOT NULL,
  PRIMARY KEY (r_regionkey)
) ENGINE=InnoDB COMMENT='TPC-H regions';

CREATE TABLE nation (
  n_nationkey INTEGER NOT NULL,
  n_name CHAR(25) NOT NULL,
  n_regionkey INTEGER NOT NULL,
  n_comment VARCHAR(152) NOT NULL,
  PRIMARY KEY (n_nationkey),
  KEY idx_nation_region (n_regionkey),
  CONSTRAINT fk_nation_region FOREIGN KEY (n_regionkey) REFERENCES region (r_regionkey)
) ENGINE=InnoDB COMMENT='TPC-H nations';

CREATE TABLE supplier (
  s_suppkey INTEGER NOT NULL,
  s_name CHAR(25) NOT NULL,
  s_address VARCHAR(40) NOT NULL,
  s_nationkey INTEGER NOT NULL,
  s_phone CHAR(15) NOT NULL,
  s_acctbal DECIMAL(15,2) NOT NULL,
  s_comment VARCHAR(101) NOT NULL,
  PRIMARY KEY (s_suppkey),
  KEY idx_supplier_nation (s_nationkey),
  CONSTRAINT fk_supplier_nation FOREIGN KEY (s_nationkey) REFERENCES nation (n_nationkey)
) ENGINE=InnoDB COMMENT='TPC-H suppliers';

CREATE TABLE customer (
  c_custkey INTEGER NOT NULL,
  c_name VARCHAR(25) NOT NULL,
  c_address VARCHAR(40) NOT NULL,
  c_nationkey INTEGER NOT NULL,
  c_phone CHAR(15) NOT NULL,
  c_acctbal DECIMAL(15,2) NOT NULL,
  c_mktsegment CHAR(10) NOT NULL,
  c_comment VARCHAR(117) NOT NULL,
  PRIMARY KEY (c_custkey),
  KEY idx_customer_nation (c_nationkey),
  CONSTRAINT fk_customer_nation FOREIGN KEY (c_nationkey) REFERENCES nation (n_nationkey)
) ENGINE=InnoDB COMMENT='TPC-H customers';

CREATE TABLE part (
  p_partkey INTEGER NOT NULL,
  p_name VARCHAR(55) NOT NULL,
  p_mfgr CHAR(25) NOT NULL,
  p_brand CHAR(10) NOT NULL,
  p_type VARCHAR(25) NOT NULL,
  p_size INTEGER NOT NULL,
  p_container CHAR(10) NOT NULL,
  p_retailprice DECIMAL(15,2) NOT NULL,
  p_comment VARCHAR(23) NOT NULL,
  PRIMARY KEY (p_partkey)
) ENGINE=InnoDB COMMENT='TPC-H parts';

CREATE TABLE partsupp (
  ps_partkey INTEGER NOT NULL,
  ps_suppkey INTEGER NOT NULL,
  ps_availqty INTEGER NOT NULL,
  ps_supplycost DECIMAL(15,2) NOT NULL,
  ps_comment VARCHAR(199) NOT NULL,
  PRIMARY KEY (ps_partkey, ps_suppkey),
  KEY idx_partsupp_supplier (ps_suppkey),
  CONSTRAINT fk_partsupp_part FOREIGN KEY (ps_partkey) REFERENCES part (p_partkey),
  CONSTRAINT fk_partsupp_supplier FOREIGN KEY (ps_suppkey) REFERENCES supplier (s_suppkey)
) ENGINE=InnoDB COMMENT='TPC-H part supplier relationships';

CREATE TABLE orders (
  o_orderkey INTEGER NOT NULL,
  o_custkey INTEGER NOT NULL,
  o_orderstatus CHAR(1) NOT NULL,
  o_totalprice DECIMAL(15,2) NOT NULL,
  o_orderdate DATE NOT NULL,
  o_orderpriority CHAR(15) NOT NULL,
  o_clerk CHAR(15) NOT NULL,
  o_shippriority INTEGER NOT NULL,
  o_comment VARCHAR(79) NOT NULL,
  PRIMARY KEY (o_orderkey),
  KEY idx_orders_customer (o_custkey),
  CONSTRAINT fk_orders_customer FOREIGN KEY (o_custkey) REFERENCES customer (c_custkey)
) ENGINE=InnoDB COMMENT='TPC-H orders';

CREATE TABLE lineitem (
  l_orderkey INTEGER NOT NULL,
  l_partkey INTEGER NOT NULL,
  l_suppkey INTEGER NOT NULL,
  l_linenumber INTEGER NOT NULL,
  l_quantity DECIMAL(15,2) NOT NULL,
  l_extendedprice DECIMAL(15,2) NOT NULL,
  l_discount DECIMAL(15,2) NOT NULL COMMENT 'Discount fraction in the range 0.00 through 0.10',
  l_tax DECIMAL(15,2) NOT NULL,
  l_returnflag CHAR(1) NOT NULL,
  l_linestatus CHAR(1) NOT NULL,
  l_shipdate DATE NOT NULL,
  l_commitdate DATE NOT NULL,
  l_receiptdate DATE NOT NULL,
  l_shipinstruct CHAR(25) NOT NULL,
  l_shipmode CHAR(10) NOT NULL,
  l_comment VARCHAR(44) NOT NULL,
  PRIMARY KEY (l_orderkey, l_linenumber),
  KEY idx_lineitem_supplier (l_suppkey),
  KEY idx_lineitem_part_supplier (l_partkey, l_suppkey),
  CONSTRAINT fk_lineitem_order FOREIGN KEY (l_orderkey) REFERENCES orders (o_orderkey),
  CONSTRAINT fk_lineitem_supplier FOREIGN KEY (l_suppkey) REFERENCES supplier (s_suppkey),
  CONSTRAINT fk_lineitem_partsupp FOREIGN KEY (l_partkey, l_suppkey) REFERENCES partsupp (ps_partkey, ps_suppkey)
) ENGINE=InnoDB COMMENT='TPC-H order line items';

CREATE EVENT inspect_urgent_orders
  ON SCHEDULE EVERY 1 DAY
  ON COMPLETION PRESERVE
  DISABLE
  COMMENT 'Read-only fixture job used to validate DataJob metadata collection'
  DO SELECT COUNT(*) FROM orders WHERE o_orderpriority = '1-URGENT';
