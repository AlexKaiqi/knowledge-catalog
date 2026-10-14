---
entity: kc/concept/schema
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/schema/definition.md
---
# Schema

作为对象发布的结构声明，约束值、字段逻辑类型、必填性及集合组织。字段 access 同时声明允许的查询能力，结构校验与查询合同解释分别处理。

普通 schema_ref 在实例所在仓、同一版本语境解析。Schema 与实例可以同批发布；结构兼容修改继续解释已有实例，删除被引用声明被拒绝。系统 Schema 副本与平台出版物一致。只改变访问声明不自动构成值结构不兼容，查询合同与投影影响仍需单独检查。

Schema 是插件的切点：插件按 Schema 声明对对象生效。结构合规不代替处理语义检查，发布声明不证明插件已安装。

相关说明：[声明式索引](../index-declaration/definition.md)、[Meta Schema](../meta-schema/definition.md)、[对象](../object/definition.md)、[Writer](../../interfaces/writer/definition.md)。
