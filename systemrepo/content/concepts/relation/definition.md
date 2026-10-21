---
entity: kc/concept/relation
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/relation/definition.md
---
# 关系

通过带角色的引用连接至少两个端点的对象，具有自己的身份、类型、方向、来源和版本。

写入只修改关系所在仓。端点可跨仓，引用不授予读权；消费时按关系存储仓和本次范围查询，目标是否可读另行判断。

## 什么时候做成关系

关联本身有业务含义、需要自己的身份与版本、会被单独引用或单独演进时，做成关系对象。只是对象内部的一个指针（例如某字段记录的外部标识），留在正文里即可，不必建关系。

关系与"同一对象的另一个方面"解决的是不同问题：关系连接两个相互独立的身份，方面是同一个身份的组成部分。跨团队维护、生命周期各自独立的两个事物，用关系；同一事物不同团队各写一部分，用方面。

相关说明：[对象](../object/definition.md)、[仓](../repository/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
