# REVIEW-10 · 是否提供可迁移的 Canonical 导出？

**场景与现状：** 设计承诺「历史可迁移」「迁移不改语义」，但公开面没有「导出全部知识（含多版本与来源）」的动作；维护扫描不产出可移植产物。

**选项与建议：** 定义 Canonical 导出/导入：一个能表达「对象身份 + Address + 值 + schema 引用 + 来源 + 快照坐标」的流式格式，导入经 Writer 重建。明确「导入后身份不变、commit 标识可以变」。

**最低验收：** 在两个不同 provider 之间往返后，同一基线的对象身份与摘要全一致；往返过程不依赖某个 provider 的私有形状。

依据：[规模设计](../../doc/engineering/testing.md) §13、[能力合同](../../doc/architecture.md)、[维护扫描](../../object/maintenance)。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。前提是多个 Snapshot provider 之间往返，存储只剩 lakeFS 后不再成立；出现真实迁移需求时再立项。
