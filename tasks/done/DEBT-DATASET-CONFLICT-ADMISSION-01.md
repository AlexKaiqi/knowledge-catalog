# DEBT-DATASET-CONFLICT-ADMISSION-01 · 核查文件交付接受前的对象冲突检查

## 债

- 位置：Dataset 接受发布路径与对象层冲突准备合同，`snapshot/dataset`、`discovery/datasets` 及其应用装配。
- 信号：2026-10-15 独立 Snapshot/Object 试跑中，双仓根挂载将两个 Schema 身份各选入两次，Dataset 定义仍接受；之后同事身份按 Dataset 读取这些 Schema 返回对象冲突。受测世界没有配置检索索引。
- 判定：Agent 已读到冲突规则和文件选择办法，其组合失误不是信息缺失。但设计要求发布时拒绝同一对象同一方面的跨仓重复，需要核查不依赖检索索引的发布路径是否落实；不能据此改窄设计或让 Reader 静默去重。
- 处置：从固定受测证据建立最小重复 Schema 发布用例，核查当前装配与并行改造后的行为；缺少守卫则在对象层的既有发布准备边界补齐，保持普通文件 Dataset 无对象模型也可交付。
- 触发条件：下一次调整 Dataset 发布准备或扩大无索引对象交付的验收范围前。

## 目标

对象冲突在设计要求的发布阶段被清楚拒绝，不能只在消费者读取时首次发现；与路径冲突、普通文件交付和检索索引准备分别成立。

## 依据

- [Dataset](../../doc/design/dataset.md)、[对象](../../doc/design/object.md)。
- [测试与验证](../../doc/engineering/testing.md)。

## 验收

- 无检索索引的双仓重复 Schema 用例给出预期发布拒绝；不相交方面组合、单份 Schema 选择及纯文件交付保持有效。
- 已接受旧修订不被候选失败替换，不削弱 Reader 的冲突检查。
- 相关合同、`make check-docs`、`make quality` 通过。

## 前置

`ZK-TRIAL-01` 的 `20261015-public-examples-pi-supplier/operator/supplier-readback/summary.json` 与原始发布轨迹。
当前并行索引迁移尚未全绿，本任务未声称已对最新源码重现。

## 不做

不新增自动去重、对象级权限或检索依赖，不为 Agent 漏用公开规则补专用业务流程。

## 关闭

2026-10-16 并入 `DATASET-REVISION-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
