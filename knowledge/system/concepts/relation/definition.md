---
entity: kc/concept/relation
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/relation/definition.md
---
# 关系

通过带角色的引用连接至少两个端点的知识对象，具有自己的身份、类型、方向、来源和版本。

写入只修改关系所在仓。端点可跨仓，引用不授予读权；消费时按关系存储仓和本次范围查询，目标是否可读另行判断。

相关说明：[对象](../object/definition.md)、[知识仓](../repository/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
