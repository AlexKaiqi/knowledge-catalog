---
entity: kc/concept/index-declaration
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/index-declaration/definition.md
---
# 声明式索引

Schema 字段上的逻辑访问声明，说明字段允许怎样被查询。Schema 中的字段 access 是唯一书写位置，结构校验与查询合同解释分别处理；物理投影与后台就绪由运行环境维护。

当前逻辑访问能力保持全文、过滤和排序。声明不携带引擎配置，也不自动授权。修改访问能力不自动成为值结构不兼容，但会改变逻辑查询合同，需判断投影影响。固定历史版本继续使用当时的结构和访问解释。

系统先验证并解释声明，再结合提供方能力和就绪情况执行查询。缺声明、提供方不支持和后台未就绪分别报告，不能返回业务空结果或扫描全仓兜底。候选最终按同一依据回读知识。

相关说明：[Schema](../schema/definition.md)、[派生投影](../../mechanisms/projection/definition.md)、[搜索知识](../../capabilities/search-knowledge/definition.md)。
