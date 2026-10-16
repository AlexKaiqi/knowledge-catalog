---
entity: kc/capability/explore-relations
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/explore-relations/usage.md
---
# 沿关系探索：使用与核对

先选定可消费的 Dataset 及其发布修订，起点使用对象身份。只沿该修订范围内的关系与对象扩展，保持同一修订并检查越界和截断。关系不会把范围外端点选进 Dataset，预算耗尽不意味着图已完整。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
