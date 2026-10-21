# httpsurface/（已退役）

本包已由 ROUTE-MANIFEST-01 退役（2026-10-16）：它与 `server/routes.go` 是同一份
封闭路由清单的两份手抄，删除后清单只留在 `server`。当前权威：

- 路由字面表：`server/routes.go`（`Routes()` 导出）。
- 导出金样：`server/routes_golden.txt`（`server.GoldenRoutes()` 嵌入导出）。
- 三方守卫：`server/routes_anchor_test.go`（清单 == 金样）、
  `scripts/validation-inventory`（实际注册 == 金样）、
  `cli/http_contract_inventory_internal_test.go`（实际注册 == 金样 + 证据归属）。
- 退役守卫：`internal/arch` 的 `TestRetiredHTTPSurfaceStaysAbsent` 阻止本包回归。

本 README 仅为历史任务文档的链接可达性保留，不含代码。
