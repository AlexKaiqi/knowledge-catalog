# DEBT-SCHEMA-LIST-COST-01 · Dataset 的 schema list 先做完整 describe 再取摘要

## 债

- 位置：`invocation/schema_dataset.go` 的 `browseDatasetSchemas`，它先调 `describeDatasetSchemas(cx, serving, "")`，再从完整报告里取摘要行。
- 信号：2026-10-16 补齐 `schema list` 摘要字段（单仓补 `aspect`，Dataset 补 `description`）时读到：为了列一页，要把每个成员的全部 Schema 合同读出并规范化，编译索引声明，还要逐成员调 `CheckSearchProjectionAt` 查就绪；就绪结果在 list 里直接丢弃，续页每页都把这些重做一遍。
- 判定：小问题，可能不算问题（2026-10-16 用户判定）。结果正确，帮助文本也写明了成本随成员和 Schema 数量增长。多出的耗时是每个成员一次就绪检查：取该 commit 的检索引擎句柄、读一次 Meta、核对投影状态，不发查询、不取候选、不建投影；本机是毫秒级，随成员数线性增长，续页每页重复一次。合同读取与声明编译是 list 本来就要做的，不算多出的部分。
- 处置：list 不做就绪检查；用到的 `entity/aspect/description/fields/uses` 只取自合同解析和索引声明编译，不走投影。是否还能更进一步（按页只读本页的合同）由 Dataset 设计裁定。
- 触发条件：Dataset 成员或 Schema 数量导致 `schema list --dataset` 明显变慢，或下次触碰 `browseDatasetSchemas` 时。

## 目标

`schema list --dataset` 的成本不高于必要的合同解析与声明编译，不触碰检索投影。

## 依据

- [检索与发现](../../doc/design/discovery.md)。
- [Dataset](../../doc/design/dataset.md)。

## 验收

- 回归测试：在没有就绪投影（或投影检查会失败）的 Dataset 上，`schema list --dataset` 仍返回完整的摘要行，且不调用投影就绪检查。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

无。

## 不做

不改变 list 行的公开形状，不改续页游标与修订绑定。

## 交付（2026-10-16）

### 实现范围

- `invocation/schema_dataset.go` 拆出 `describeDatasetSchemasWithReadiness(cx, serving, objectID, readiness)`：`schema describe` 保留逐成员 `CheckSearchProjectionAt` 就绪报告；`browseDatasetSchemas`（即 `schema list --dataset` 与续页）改传 `readiness=false`，不再发起就绪检查。合同读取、`AccessSpecAt` 声明编译与 `entity/aspect/description/fields/uses` 摘要来源不变，公开形状与续页游标不变。
- 新增回归 `invocation/schema_list_readiness_test.go` 的 `TestSchemaListDoesNotAskIndexReadiness`：就绪检查必失败的计数插件上，list 首页与续页零次就绪调用且摘要行完整；describe 仍逐成员报告失败的就绪状态。

### 验证范围

- 先红后绿：临时还原旧实现（list 直接调 describe）时新回归以“1 calls readiness”失败，恢复后通过。
- `make check-docs` 通过；`make quality`（含耦合基线 13.97% 持平）通过；`make check-validation` 通过；`make test` 全绿；受影响 `go test ./invocation/ ./object/...` 11 包全绿；真实 OpenSearch 上的 `TestDatasetSearchFollowsExplicitRevisionPublication` 经独占临时实例补跑通过（此前无 `KC_TEST_OPENSEARCH_URL` 时的失败为环境缺失，非本改动引入）。
- 无 skip，无断言放宽。
