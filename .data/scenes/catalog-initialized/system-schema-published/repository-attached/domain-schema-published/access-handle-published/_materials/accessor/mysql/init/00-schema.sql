SET NAMES utf8mb4;

CREATE TABLE categories (
  category_id INTEGER NOT NULL,
  name VARCHAR(32) NOT NULL,
  PRIMARY KEY (category_id)
) ENGINE=InnoDB COMMENT='Qinghe shop product categories';

CREATE TABLE customers (
  customer_id INTEGER NOT NULL,
  name VARCHAR(32) NOT NULL,
  city VARCHAR(32) NOT NULL,
  channel CHAR(8) NOT NULL,
  PRIMARY KEY (customer_id)
) ENGINE=InnoDB COMMENT='Qinghe shop customers';

CREATE TABLE products (
  product_id INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  name VARCHAR(40) NOT NULL,
  list_price DECIMAL(10,2) NOT NULL,
  PRIMARY KEY (product_id),
  KEY idx_products_category (category_id),
  CONSTRAINT fk_products_category FOREIGN KEY (category_id) REFERENCES categories (category_id)
) ENGINE=InnoDB COMMENT='Qinghe shop products';

CREATE TABLE orders (
  order_id INTEGER NOT NULL,
  customer_id INTEGER NOT NULL,
  order_status VARCHAR(16) NOT NULL,
  order_date DATE NOT NULL,
  pay_amount DECIMAL(10,2) NOT NULL,
  remark VARCHAR(64) NOT NULL,
  PRIMARY KEY (order_id),
  KEY idx_orders_customer (customer_id),
  CONSTRAINT fk_orders_customer FOREIGN KEY (customer_id) REFERENCES customers (customer_id)
) ENGINE=InnoDB COMMENT='Qinghe shop orders';

CREATE TABLE order_items (
  order_id INTEGER NOT NULL,
  line_no INTEGER NOT NULL,
  product_id INTEGER NOT NULL,
  qty DECIMAL(10,2) NOT NULL,
  unit_price DECIMAL(10,2) NOT NULL,
  discount DECIMAL(5,2) NOT NULL,
  PRIMARY KEY (order_id, line_no),
  KEY idx_order_items_product (product_id),
  CONSTRAINT fk_order_items_order FOREIGN KEY (order_id) REFERENCES orders (order_id),
  CONSTRAINT fk_order_items_product FOREIGN KEY (product_id) REFERENCES products (product_id)
) ENGINE=InnoDB COMMENT='Qinghe shop order lines';

CREATE TABLE sales_mart (
  order_date DATE NOT NULL,
  order_status VARCHAR(16) NOT NULL,
  city VARCHAR(32) NOT NULL,
  category_name VARCHAR(32) NOT NULL,
  gmv DECIMAL(15,2) NOT NULL,
  order_count INTEGER NOT NULL,
  item_count INTEGER NOT NULL,
  PRIMARY KEY (order_date, order_status, city, category_name)
) ENGINE=InnoDB COMMENT='ETL mart: GMV by day, status, city and category';

CREATE EVENT flag_unpaid_orders
  ON SCHEDULE EVERY 1 DAY
  ON COMPLETION PRESERVE
  DISABLE
  COMMENT 'Read-only fixture job: count unpaid shop orders'
  DO SELECT COUNT(*) FROM orders WHERE order_status = 'UNPAID';

CREATE EVENT refresh_sales_mart
  ON SCHEDULE EVERY 1 DAY
  ON COMPLETION PRESERVE
  DISABLE
  COMMENT 'ETL: rebuild sales_mart from orders, order_items, customers, products and categories'
  DO INSERT INTO sales_mart (
    order_date, order_status, city, category_name, gmv, order_count, item_count
  )
  SELECT
    o.order_date,
    o.order_status,
    c.city,
    cat.name,
    SUM(i.qty * i.unit_price * (1 - i.discount)),
    COUNT(DISTINCT o.order_id),
    COUNT(*)
  FROM orders o
  JOIN order_items i ON i.order_id = o.order_id
  JOIN customers c ON c.customer_id = o.customer_id
  JOIN products p ON p.product_id = i.product_id
  JOIN categories cat ON cat.category_id = p.category_id
  GROUP BY o.order_date, o.order_status, c.city, cat.name;
