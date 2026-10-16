# FINAL-ACCEPTANCE-01 · 插件与 Dataset 修订重构的整体验收

## 目标

在同一候选上证明按新设计的这一轮重构整体达成：插件只作解释器、LiveAccess 声明与读取、索引声明与能力
清单、检索空间按 Dataset 修订、使用记录、Dataset 修订生命周期、系统出版物同步、目录重组。交付验收报告。

## 依据

- [插件](../../doc/design/plugins.md)、[发现](../../doc/design/discovery.md)、[LiveAccess](../../doc/design/liveaccess.md)、[Dataset](../../doc/design/dataset.md)。
- [测试与验证](../../doc/engineering/testing.md)、[架构不变量](../../doc/engineering/architecture-invariants.md)。

## 验收

- 门禁：`make check-docs`、`make quality`、`make test`、`make test-contracts`、`make test-all`、`make deploy-local-scenes` 在同一候选上全部退出 0，无 skip、无削弱断言。
- live 用例：`make test-all` 中依赖外部环境的 live 用例（2026-10-16 L0 门禁登记 6 项：Luna rerank、两项 State Docker/runtime、Taihu、真实 lakeFS bulk、真实 Snapshot publication）在验收环境实际执行。验收开始前在本文件列出各用例所需的外部环境与入口；环境不到位即整体验收不通过，不以 skip 或“独立入口未验证”代替。
- 规格：`QH/_specs/accept-01` 到 `accept-13` 与 `RA/_specs/liveaccess.yaml` 每条都有执行证据并为绿（`QH`、`RA` 的含义见 `INDEX-SEARCH-SPACE-01`）。
- 旧用例：各实现任务“替换的旧用例”一节里的条目全部处理完；`accept-retire-*` 三条结构检查为绿。
- 发现：真实部署上，Agent 从 `kc help` 出发先在库存里找到可消费的 Dataset，再在其中按关键词、过滤和关系搜到对象，结果只含有权内容；按仓搜索被拒绝。可复用 `ZK-TRIAL-02` 的方法与记录格式，但须在本候选上重跑。
- 访问：外部值在原位置出现并带观察依据，逐值失败；同一对象 ID 在部署范围和 Dataset 范围都能读到。
- 产品书：`DEBT-PRODUCT-CLAIMS-01` 审计表中依赖本轮实现的承诺（自动跟随、按 Dataset 修订生效的索引声明、库存按消费权列出、只有共享外部值进索引、成员使用汇总）在本候选上按产品书原预期复核；不以删去承诺代替完成设计目标。
- 场景缺口：`.data/scenes/_views.yaml` 中 discovery、liveaccess、dataset 登记的缺口逐条关闭或写明仍缺什么。
- `test-catalog.md` 与 `architecture-invariants.md` 中随本轮改写的行都指向实际存在的测试。

## 前置

- 实现：`DATASET-REVISION-01`、`DATASET-RETENTION-01`、`DATASET-STORAGE-01`、`INDEX-USAGE-01`、`PUBLICATION-SYNC-01`、`ROUTE-MANIFEST-01`、`ARCH-LAYOUT-01`、`DEBT-INVENTORY-CONSUMPTION-01`。
- 验收执行：`ACCEPT-HARNESS-01`。“规格”一条要求 accept-01 至 accept-13 每条都有执行证据，执行器须先为全部规格接上 probe，且真实测试部署与共享搜索容量健康。
- 试跑：`ZK-TRIAL-02`。

## 不做

不在验收轮顺手改行为；发现问题按三出口处理。
