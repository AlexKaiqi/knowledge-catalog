---
entity: kc/capability/resource-access
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/resource-access/usage.md
---
# 访问动态资源：使用与核对

先核对方面或成员上的取值来源及它引用的 ResourceDescriptor，确认固定声明版本、源身份要求与具体操作；Schema 只约束取回的值。读取对象会自动取值，插件说明见 `kc plugin liveaccess help`，显式资源操作另查对应叶子 help。源权限独立判断，声明、观察依据与有效期应保留。源拒绝时不能用缓存代替准入。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
