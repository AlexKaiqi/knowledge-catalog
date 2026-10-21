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

## 交付（2026-10-16）

实现范围：

- UI 补斜杠跳转显式固定为 307：`snapshot/service/http.go` 在 `/ui/` 挂载点旁注册精确 pattern `GET /ui`，handler 内 `http.Redirect(..., http.StatusTemporaryRedirect)`，不再依赖 Go ServeMux 的隐式子树跳转（Go 1.23 返回 301、更新工具链返回 307）。对应债务 `DEBT-UI-REDIRECT-STATUS-01` 收口。
- 路由清单唯一留在 server：`server/routes.go` 字面表新增 `GET /ui` 条目（清单 90→91）；`httpsurface` 包退役删除（`registry.go`、`registry_test.go` git rm），仅保留 `httpsurface/README.md` 作退役说明（`done/DATASET-03.md` 历史链接需要）。
- 金样文件取代 `httpsurface.Patterns()`：新建 `server/routes_golden.txt`（91 行 `METHOD /pattern` 字典序）与 `server/routes_golden.go`（`go:embed` + `GoldenRoutes()` 导出）。
- 三方守卫：`server/routes_anchor_test.go` 重写为 Routes() 集合 ↔ 金样双向对账、金样有序且无重复、清单无重复且保持有序、导出形状检查；`scripts/validation-inventory` 改读 `server/routes_golden.txt`（删 `kc/httpsurface` import）；`cli/http_contract_inventory_internal_test.go` 改读金样文件对账；`internal/arch/layers_test.go` 新增 `TestRetiredHTTPSurfaceStaysAbsent`（目录仅允许退役 README + AST `parser.ImportsOnly` 全仓 import 检查），并清理全部 httpsurface 引用。
- 计数与白名单同步：`cli/http_surface_coverage_test.go` 计数 90→91、`allowedHTTPPath` 加 `/ui`；`cli` 证据表 `GET /ui` 增加证据测试。
- 耦合基线显式收紧：`go run ./scripts/coupling -write-baseline`，`scripts/coupling/baseline.json` 移除 httpsurface 单元（units 82→81，propagation 14.31% 持平）。
- 文档同步 9 处：`doc/design/service.md`、`doc/roadmap.md`、`cli/README.md`、`cli/SURFACE.md`、`server/README.md`、`home/README.md`、`invocation/README.md`、`invoke/README.md`、`internal/arch/README.md`。

已验证范围：

- 红证据（先红后绿）：改动前在 go1.23.6 上跑 `cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言，实际返回 301 而断言期待 307，测试红；守卫红验证：故意把金样一条改为 `GET /ui-RED`，`TestRoutesMatchGoldenManifest` 双向报红（金样多余条目与清单缺失条目都被点名），恢复后绿。
- 双工具链：go1.23.6 与当前工具链（go1.26.0）各跑一次 UI 补斜杠断言，均 ok，断言未放宽。
- 门禁：`go build ./...` / `go vet ./...` 全净；`make check-docs` PASS（19 documents）；`make quality` 通过（耦合棘轮 held，81 units）；`make check-validation` 通过（54 CLI commands、91 HTTP routes、2161 Go declarations）；`make test` 通过（lakefs 组 TestProductScenes + TestMetricPermissionScenes 全过，54/54 命令覆盖）；`make test-contracts` 测试全过。
- 瞬态说明：`make test` 与 `make test-contracts` 各报过一次 `make: *** [test] Error 1`，均为并行任务提交改变工作区 git 指纹触发 `validation.py` 的 source-changed 判定（证据无效而非测试失败），go-test.jsonl 零 fail；复跑后 `report.json` 显示 `status passed / sourceChanged False`。
- 说明：本线未执行 commit；全部改动已被提交为 5e280349（refactor(server): retire httpsurface and keep one route table with a golden manifest，24 文件 426+/356-）。

## 不做

除 UI 跳转状态码固定外不改公开行为；不搬目录、不拆 `home/`（`ARCH-LAYOUT-01`）。

## 并入

2026-10-16 从 `ARCH-LAYOUT-01` 拆出，带入其原“并入”一节的 `DEBT-UI-REDIRECT-STATUS-01`（原债文本在 `done/`）：

- 位置：`cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言。
- 信号：断言期待 307；Go 1.23.6 标准 ServeMux 返回 301，Go 1.26.0 返回 307，独立 ServeMux 同样复现。项目最低
  Go 版本为 1.23，该断言在最低版本上是红的。
- 处置：在路由里显式固定状态码，不以修改断言处理。
- 剩余验收：见上方“验收”第二条。
