---
entity: kc/capability/resource-access
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/resource-access/definition.md
---
# 访问动态资源

读取访问声明，向独立 runtime 请求动态值。当前状态与已声明操作通过 [LiveAccess](../../object-interfaces/live-access/definition.yaml) 和 [对象能力](../object-interfaces/definition.md) 统一组织，历史访问入口继续兼容。

相关说明：[动态资源访问](../../mechanisms/dynamic-access/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
