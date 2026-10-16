---
entity: kc/concept/index-declaration
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/index-declaration/definition.md
---
# 声明式索引

索引声明是采用索引声明 Schema 的 `index` 方面成员，由 Dataset 维护者发布在自己的仓里，选进 Dataset 后才对这个 Dataset 生效。每个成员以 Schema 字段定位符列出输入，声明构建方式、用途和所依据的能力清单版本；输入覆盖选入范围内采用该 Schema 的对象，也可以按对象身份组合不同方面。Schema 本身只描述值结构。Writer 校验声明形状和字段定位符，实例按能力清单逐成员核对支持情况；索引准备与后台就绪由运行环境维护。

用途包括全文、过滤、排序和语义召回，具体支持范围以实例公开清单为准。声明不携带引擎地址、原始 mapping、脚本或分片配置，也不自动授权。修改声明不自动成为值结构不兼容，但会改变逻辑查询合同，需判断投影影响。固定历史版本继续使用当时的结构和声明。

一个 Dataset 的一个发布修订构成检索空间，内容可以来自多个仓。查询按字段定位符表达过滤和排序，全文与语义给出文本，不把成员键当查询参数；结果依据保留参与的声明成员。缺声明、提供方不支持、成员失效和后台未就绪分别报告，不能返回业务空结果或扫描全仓兜底。候选最终按同一修订与来源版本回读对象。

共享观察准备只接受取值描述为 shared 的外部单元；per-caller 值不能进入共享索引。当前含此类单元的观察准备会明确失败，逐声明成员独立标记不支持的隔离能力尚未就绪。能力清单、Schema 或成员变化时重新核对受影响的成员。

相关说明：[Schema](../schema/definition.md)、[派生投影](../../mechanisms/projection/definition.md)、[搜索对象](../../capabilities/search-object/definition.md)。
