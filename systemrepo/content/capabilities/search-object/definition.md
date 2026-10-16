---
entity: kc/capability/search-object
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/search-object/definition.md
---
# 搜索对象

先从服务库存选择可消费的 Dataset，再在它的固定发布修订内按选入的索引声明寻找候选，最后按同一修订和来源版本回读正文。搜索、关系与遍历都要求 Dataset 范围，消费权不产生来源仓权限。

相关说明：[Schema](../../concepts/schema/definition.md)、[派生投影](../../mechanisms/projection/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
