# DEBT-INDEX-CONTRACT-GATES-01 · 收口检索迁移的统一检查接缝

## 债

- 位置：`internal/arch/typed_executor_test.go`、`doc/engineering/test-catalog.md:507`、`scripts/coupling/baseline.json` 与 `discovery/` 依赖图。
- 信号：架构守卫仍要求旧 `SearchExecutor`；验证库存引用两项已不存在的 ProjectionNotify 测试；quality 报新 `discovery/space` 单元、传播成本 14.09% 超过基线 14.01%，以及 `discovery/datasets` 不稳定度从 0.50 到 0.57。
- 判定：都处于 `INDEX-SEARCH-SPACE-01` 已拥有的搜索入口替换、旧用例迁移与拆包范围，不属于文件下载增量。
- 处置：立即并入同一进行中任务，保留原风险及必要反例；按最终结构解释并复核依赖图，不以放宽断言或简单重写基线换绿。
- 触发条件：`INDEX-SEARCH-SPACE-01` 的统一验收前。

## 目标

检索迁移后的真实执行器、用例库存与质量门禁一致，统一检查不再停在旧合同接缝。

## 依据

- [工程质量闭环](../../doc/engineering/quality.md)。
- [测试与验证](../../doc/engineering/testing.md)。

## 验收

- typed 守卫及其反例继续阻止绕过应用执行器，检查当前 Dataset 搜索入口。
- 被替换测试的风险仍有正式用例，文档引用与验证库存通过。
- 依赖图增量经复核，`make quality`、`make check-docs` 及受影响合同通过。

## 前置

随 `INDEX-SEARCH-SPACE-01` 收口。

## 不做

不恢复已退役的仓搜索，不新增并行实现线，不改文件下载行为。

## 关闭

已并入 `INDEX-SEARCH-SPACE-01`；关闭表示归并，不表示上述门禁已经通过。
