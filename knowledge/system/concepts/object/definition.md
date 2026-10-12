---
entity: kc/concept/object
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/object/definition.md
---
# 对象

具有独立身份、可以被引用和版本化维护的知识载体。对象可以描述一张表、一个指标、一项能力或一份规则。身份由仓与 object_id 确定，文件路径只说明位置。

对象可以作为完整值维护，也可以分成多个切面。关系与 Schema 也具有对象身份。现行合同保留 Entity 作为普通对象整体值的 Address 名称，以及 entity 作为 Schema 类型名字段；产品解释统一称对象。

对象不等于文件。定义可以描述含义、边界和组成，引用分散的内部对象、固定文件或领域外部标识；definition 是推荐组织约定，不强制统一资源集合结构。对象按需要实现共同能力，基本知识读写不以 Interface 为前提。

相关说明：[对象 Interface](../object-interface/definition.md)、[切面](../aspect/definition.md)、[关系](../relation/definition.md)、[知识地址](../address/definition.md)、[Schema](../schema/definition.md)。
