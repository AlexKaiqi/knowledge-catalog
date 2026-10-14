---
entity: kc/concept/object-interface
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/object-interface/definition.md
---
# 对象 Interface

根据对象处理逻辑的共性，定义对象实现要求、可用操作和结果保证。对象按合同提供实现信息，系统通过独立扩展的处理器完成公共处理逻辑，消费者通过统一入口使用能力。

对象可实现多个 Interface，同一能力可有多个明确选择的实现。实现声明是普通知识单元，推荐按集合型切面的成员分别维护。声明定义结构，处理器检查实际语义。处理器支持按实现结构判断，字段查询声明变化不改变这种支持；完整知识版本和声明依据仍保留。文件扩展名、目录和对象分类不推导能力。

Interface 合同与实现 Schema 作为知识发布，处理器由部署装配。新增扩展无需向核心分派添加领域类型分支，也无需运行时追加只读系统仓。普通 Schema 引用保持同仓同版本解释，跨仓能力引用采用明确固定依据，并受消费范围约束。

相关说明：[对象](../object/definition.md)、[切面](../aspect/definition.md)、[Schema](../schema/definition.md)、[对象能力分派](../../mechanisms/object-interface-dispatch/definition.md)、[使用能力](../../guides/use-object-interface/guide.md)。
