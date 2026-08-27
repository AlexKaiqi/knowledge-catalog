INSERT INTO region VALUES (1, 'AMERICA', 'fixture');
INSERT INTO nation VALUES (1, 'UNITED STATES', 1, 'fixture');
INSERT INTO supplier VALUES (1, 'Supplier#1', 'Address', 1, '1-111-111-1111', 0.00, 'fixture');
INSERT INTO customer VALUES (1, 'Customer#1', 'Address', 1, '1-111-111-1111', 0.00, 'BUILDING', 'fixture');
INSERT INTO part VALUES (1, 'Part#1', 'Manufacturer#1', 'Brand#1', 'TYPE', 1, 'BOX', 10.00, 'fixture');
INSERT INTO partsupp VALUES (1, 1, 100, 5.00, 'fixture');
INSERT INTO orders VALUES (1, 1, 'O', 9.00, '2026-08-27', '1-URGENT', 'Clerk#1', 0, 'fixture');
INSERT INTO lineitem VALUES (
  1, 1, 1, 1, 1.00, 10.00, 0.10, 0.00, 'N', 'O',
  '2026-08-28', '2026-08-27', '2026-08-29', 'DELIVER IN PERSON', 'AIR', 'fixture'
);
