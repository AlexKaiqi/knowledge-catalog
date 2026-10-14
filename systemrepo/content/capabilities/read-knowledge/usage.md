---
entity: kc/capability/read-knowledge
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/read-knowledge/usage.md
---
# 精确读取与复核：使用与核对

指定唯一仓或明确 Dataset 范围；需要复核时保留 commit。读取结果说明保存值与来源，缺切面或解释依赖应按实际范围判断，不能自动补读清单外内容。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
