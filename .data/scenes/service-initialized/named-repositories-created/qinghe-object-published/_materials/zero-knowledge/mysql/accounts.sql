-- 零知识用例的 MySQL 账号。由 operator 以 root 执行，不属于 Agent 可见材料。
-- kc_zk_reader：只读整个 shop 库，交给 Agent 的默认连接。
-- kc_zk_limited：只读底表，不能读 sales_mart，供 zk-task-layered-denial 区分“哪一层没有权限”。
-- 两个账号都没有 EVENT 权限，SHOW EVENTS 看不到作业定义，作业信息只能从 KC 获得。
SET NAMES utf8mb4;

CREATE USER IF NOT EXISTS 'kc_zk_reader'@'%' IDENTIFIED BY 'zk-read';
GRANT SELECT ON shop.* TO 'kc_zk_reader'@'%';

CREATE USER IF NOT EXISTS 'kc_zk_limited'@'%' IDENTIFIED BY 'zk-limited';
GRANT SELECT ON shop.orders TO 'kc_zk_limited'@'%';
GRANT SELECT ON shop.order_items TO 'kc_zk_limited'@'%';
GRANT SELECT ON shop.customers TO 'kc_zk_limited'@'%';
GRANT SELECT ON shop.products TO 'kc_zk_limited'@'%';
GRANT SELECT ON shop.categories TO 'kc_zk_limited'@'%';
