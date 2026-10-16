---
entity: kc/concept/schema
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/schema/definition.md
---
# Schema

作为对象发布的结构声明，约束值、字段逻辑类型、必填性及集合组织。Schema 只描述结构；索引声明和外部取值分别由独立声明提供，不写进 Schema。

schema_ref 按对象 ID 在固定版本语境中解析，也可以明确引用仓和版本。Schema 与实例可以同批发布；结构兼容修改继续解释已有实例，删除被引用声明被拒绝。系统 Schema 副本与平台出版物一致。修改索引成员不改变值结构，但需要另行核对检索能力与就绪。

插件按声明的形状认领：索引插件解释采用索引声明 Schema 的成员，LiveAccess 解释方面或成员上的取值来源。结构合规不代替处理语义检查，发布声明不证明插件已安装。

相关说明：[声明式索引](../index-declaration/definition.md)、[Meta Schema](../meta-schema/definition.md)、[对象](../object/definition.md)、[Writer](../../interfaces/writer/definition.md)。
