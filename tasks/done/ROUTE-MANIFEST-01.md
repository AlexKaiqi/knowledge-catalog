# ROUTE-MANIFEST-01 · 公开 HTTP 接口只有一份封闭清单

## 目标

公开 HTTP 接口只有一份封闭清单，由 server 持有；server 实际注册、清单与金样三者一致，任一处漂移即红。
UI 子树根的补斜杠跳转状态码由路由显式固定，不随 Go 工具链版本变化。

## 依据

- [架构](../../doc/architecture.md)、[路线](../../doc/roadmap.md) 的目录重组表（HTTP 路由表只保留一份）。
- [架构不变量](../../doc/engineering/architecture-invariants.md)。

## 验收

- 删除 `httpsurface`，清单只留在 server；`validation-inventory` 改读 server 导出的金样文件；守卫为“server 实际注册 == 清单 == 金样”，故意违反时变红。
- `cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言用 Go 1.23.6 与当前工具链各跑一次，都通过且断言未放宽。
- `make check-docs`、`make quality`、`make test`、`make test-contracts` 通过。

## 前置

`INDEX-SEARCH-SPACE-01`：`server/routes.go`、`httpsurface/` 当前有在途改动，按仓搜索退役也会删改路由。

## 不做

除 UI 跳转状态码固定外不改公开行为；不搬目录、不拆 `home/`（`ARCH-LAYOUT-01`）。

## 并入

2026-10-16 从 `ARCH-LAYOUT-01` 拆出，带入其原“并入”一节的 `DEBT-UI-REDIRECT-STATUS-01`（原债文本在 `done/`）：

- 位置：`cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言。
- 信号：断言期待 307；Go 1.23.6 标准 ServeMux 返回 301，Go 1.26.0 返回 307，独立 ServeMux 同样复现。项目最低
  Go 版本为 1.23，该断言在最低版本上是红的。
- 处置：在路由里显式固定状态码，不以修改断言处理。
- 剩余验收：见上方“验收”第二条。
