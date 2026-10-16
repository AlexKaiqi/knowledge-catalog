# DEBT-ARCH-LOCAL-DUMPS-01 · 明确架构扫描与本机导入材料的边界

## 债

- 位置：`internal/arch/object_identity_shape_test.go` 的工作区遍历与 `.data/realcase/handoff/merged/` 本机材料。
- 信号：`FILE-ACCESS-01` 的完整架构复验扫描到被 Git 忽略、未入库的导入文件，持续报告旧关系端点形状；该用例尚未结束时已产生大量相同诊断。
- 判定：本机导入材料与受维护协议材料的检查边界未明确；不是本轮文件下载增量。不能靠放宽关系断言、全局忽略 `.data/` 或把原始材料改写成产品事实解决。
- 处置：明确受维护文件与本机导入缓存的边界，复用仓内文件选择规则，并为应检查与不应检查的材料补充反例；真实导入发布另行核对其协议形状。
- 触发条件：下次完整架构验收，或将本机导入材料接入正式发布前。

## 目标

架构守卫完整检查源码、设计和协议场景材料，本机未入库材料不改变同一源码快照的检查范围。

## 依据

- [核心概念](../../doc/concepts.md)。
- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 受维护材料中的错误对象引用和关系端点仍被拒绝；Git 忽略的本机导入材料有明确、可测试的处理边界。
- 完整架构检查能结束，原始材料的范围与检查结果分别说明。
- 受影响合同、`make check-docs`、`make quality` 通过，不放宽原断言。

## 前置

无。

## 不做

不迁移真实导入业务材料，不把忽略本机缓存记作材料通过协议验收。

## 发现证据

`.validation/runs/20261016T033305Z-2335a4a55e39/` 保留原诊断。该宽运行在明确的 typed 守卫失败后，因继续扫描本机材料而主动中止；最终为 interrupted，且 sourceChanged=true，不能记为完整架构通过或完整失败计数。

## 交付

2026-10-16 按本条自身验收收口：架构扫描复用 tracked 与新 non-ignored 文件清单，受维护的场景、设计与源码继续检查；本机忽略的导入材料不进入源码守卫。`TestArchitectureSourceInventoryKeepsFixturesAndNewFiles` 同时覆盖应纳入与应排除的材料，身份形状断言未放宽。

- 已验证：完整架构 676 pass / 0 fail / 0 skip，包含来源清单及身份形状守卫，见原始完整报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）。该完整报告的 11 项外部 live skip 不属于本条架构范围。
- 来源核对：本条清单与身份守卫在上述通过来源及后续候选间未变；后续 quality（含 check-docs）通过，见候选质量报告（`.validation/runs/20261016-mvp-unified-050129Z/dataset-quality-report.json`）。本次依据范围与相关字节对照见复核记录（`.validation/runs/20261016-mvp-evidence-audit/review.json`），不宣称新候选整套合同已通过。
- 边界：本机原始导入材料没有因此取得协议验收；本轮未迁移业务材料、未提交。任务记录及状态变更后统一检查文档与质量，结果另存本轮收口证据。
