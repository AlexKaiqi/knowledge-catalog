---
entity: kc/capability/object-interfaces
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/object-interfaces/definition.md
---
# 使用对象能力

对象按 Interface 实现共同能力，消费者选择对象、实现与操作，由系统按固定合同分派到已装配处理器。

当前内置 LiveAccess 提供当前状态读取与已声明操作调用。历史 ResourceDescriptor 操作入口保持兼容。其他能力通过独立扩展接入，不能仅凭文件类型或能力说明认定已部署。

相关说明：[对象 Interface](../../concepts/object-interface/definition.md)、[对象能力分派](../../mechanisms/object-interface-dispatch/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
