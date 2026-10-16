---
entity: kc/capability/resource-access
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/resource-access/definition.md
---
# 访问动态资源

读取方面或成员上的取值来源，按它引用的 ResourceDescriptor 向外部系统取得值。读取对象时，已安装的 [LiveAccess](../../object-interfaces/live-access/definition.yaml) 插件透明取值，失败按单元说明；已声明的资源操作沿 `kc invoke` 入口另行授权。

相关说明：[动态资源访问](../../mechanisms/dynamic-access/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
