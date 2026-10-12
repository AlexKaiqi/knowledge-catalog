---
entity: kc/capability/projections
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/projections/usage.md
---
# 维护查询投影：使用与核对

先诊断当前依据、覆盖与就绪状态，再选维护动作。同步不重写知识；动态 notice 不推进仓 HEAD，提示不能代替持续对账与恢复。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
