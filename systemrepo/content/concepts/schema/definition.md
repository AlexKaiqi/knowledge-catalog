---
entity: kc/concept/schema
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/schema/definition.md
---
# Schema

作为对象发布的结构声明，约束值、字段逻辑类型、必填性及集合组织。Schema 只描述结构；索引声明和外部取值分别由独立声明提供，不写进 Schema。

schema_ref 按对象 ID 在固定版本语境中解析，也可以明确引用仓和版本。Schema 与实例可以同批发布；结构兼容修改继续解释已有实例，删除被引用声明被拒绝。修改索引成员不改变值结构，但需要另行核对检索能力与就绪。

副本必须与系统出版物一致的内置协议 Schema 只有 Meta Schema `schema/meta/schema-definition/v1`
与索引声明 Schema `schema/core/index-declaration/v1`。它们定义服务采用的校验与声明合同，不能在业务仓原地演进。
其余系统出版的 Schema，包括 `schema/core/readme/v1` 与 `schema/core/relation/v1`，按普通 Schema
的兼容规则维护；“由系统仓发布”本身不表示副本不可修改。

插件按声明的形状认领：索引插件解释采用索引声明 Schema 的成员，LiveAccess 解释方面或成员上的取值来源。结构合规不代替处理语义检查，发布声明不证明插件已安装。

## 复用还是新建

按 ID 引用别的仓发布的 Schema 并固定版本即可，业务团队不需要在自己的仓里复制一份。判断能否复用看结构而不是看名字接近：字段构成、必填性、集合组织能否容纳资料的实际形状。只差展示用的宽松约束时，复用的代价通常小于新建一份近似类型。

新建的理由是结构本身不同，或确实需要独立演进并承担迁移责任。为了省一次查找而复制同构的 Schema，会让同一个含义出现多个身份，后续无法统一演进。兼容修改要继续能解释已有实例，不能只验证新写入的那一个；删除被引用的 Schema 会被拒绝。不兼容的修改怎样迁移还没有定论，不要假设它可用。

相关说明：[声明式索引](../index-declaration/definition.md)、[Meta Schema](../meta-schema/definition.md)、[对象](../object/definition.md)、[Writer](../../interfaces/writer/definition.md)。
