---
entity: kc/capability/permissions
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/permissions/usage.md
---
# 查询与管理授权：使用与核对

先区分 登记空间、仓和 Dataset 资源，再选择动作。仅查询本人可用能力时使用现行查询入口；管理动作需要对应权。授权回执保留规则身份，撤权后旧 pin 不提供绕过。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
