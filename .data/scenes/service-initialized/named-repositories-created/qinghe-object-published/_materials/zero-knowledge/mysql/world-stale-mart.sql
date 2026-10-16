-- 世界 B：假下降。在 world-base.sql 之后执行。
-- 订单不变；sales_mart 只保留到 2026-09-09，底表口径的 GMV 没有下降。
-- 删除只是构造手段；对 Agent 而言，可观察到的只是汇总覆盖不完整，原因（没运行、运行失败、被删）不可区分。
SET NAMES utf8mb4;

DELETE FROM sales_mart WHERE order_date > '2026-09-09';
