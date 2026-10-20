---
entity: kc/concept/member
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/member/definition.md
---
# 成员

集合型切面中具有稳定 member_key 的独立维护单元。

成员键表达集合内的身份，展示顺序不构成身份。集合型 Schema 约束成员值，Reader 可以组装整个集合。

## 什么时候做成成员

判断标准是集合元素是否需要独立维护：需要按稳定键单独读写、单独更新、单独标失效或单独统计归因的，做成成员；只是随切面一起读写的普通字段，留在切面内即可。把每个字段都提升为成员会让维护粒度变碎，而把需要单独更新的内容埋在普通字段里，则只能整体替换整个切面。

写入粒度跟着这个划分走：成员可以完整替换或删除，深字段可读但不承诺字段级 PATCH，集合根也不自动变成全部成员的删除。

相关说明：[切面](../aspect/definition.md)、[对象地址](../address/definition.md)、[Schema](../schema/definition.md)。
