---
entity: kc/capability/extensions
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/extensions/usage.md
---
# 配置 Hook：使用与核对

核对动作、时机与范围后登记扩展。前置与后置有不同失败含义：前置拒绝阻止该动作，后置投递失败不取消已接受发布或合并。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
