---
entity: kc/mechanism/governance
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/governance/definition.md
---
# 工作分支与条件合并

需要评审的单仓变更先写到工作分支，由业务完成评审后，再把审阅过的分支 commit 条件合并到目标引用。

## 工作分支

Writer 可以把提交写到指定分支。分支写入复用同一套 Schema、来源、并发与幂等校验，但不推进 main，也不通知索引、Dataset 或维护；一次变更仍只修改一个仓。

## 条件合并

合并只做快进：分支必须是目标的后代。调用方可以固定审阅时看到的分支 commit 与目标 commit，任一移动都拒绝，需要在新基点上重新评审。合并前按目标的 Schema 重新校验分支里的每个改动对象，并要求改动全部是经 Writer 写入的对象单元。只有推进 main 时才通知下游。

## 审批归业务

KC 不保存评审、预览或外部套件结果。审批由业务系统负责，通过前置 writer.merge Hook 接入：前置 Hook 拒绝即否决合并；后置 Hook 投递失败不回滚已接受的合并。

相关说明：[Writer](../../interfaces/writer/definition.md)、[版本](../../concepts/version/definition.md)、[出站扩展](../hook/definition.md)。
