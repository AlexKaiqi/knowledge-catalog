---
entity: kc/concept/address
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/address/definition.md
---
# 对象地址

完整访问引用由对象 URN 与内容 locator 表达。URN 指认来源 Repository 与 object_id，JSON Pointer 从对象内容根选择节点，不指向文件路径或交付信封；版本及 Repo、Dataset 访问语境作为读取依据另行固定。READ、PUT、REMOVE 接受同一种引用，调用方不必拆成仓、对象、切面、成员参数。

例如，`urn:kc:object:kr%3A%2F%2Fscene%2Fobject:table%2Forders#/columns/order_id` 定位一个列 Member；再加 `/constraints/nullable` 可读取其中的数据。空 locator 指向对象根，`/` 选择空名称字段；键名中的 `/` 与 `~` 分别用 `~1` 与 `~0` 转义。不存在的节点与合法空值分别报告。

现行 Go Address 保留内部维护坐标，由对象标识与可选切面名、成员键表达；种类包括 Entity、Aspect、Relation、Member，历史文件也可能使用 Record。服务依据对象表示将统一引用映射为完整维护单元，不能按 locator 深度猜写入粒度。

整体值、完整 Aspect、Member 可以完整替换或删除；对象根 REMOVE 删除该对象全部维护单元。深字段可读并不承诺字段 PATCH，Members 集合根也不自动变成全部 Member 的删除。维护地址与 Dataset 交付路径都不改变对象身份。

相关说明：[对象](../object/definition.md)、[切面](../aspect/definition.md)、[成员](../member/definition.md)、[关系](../relation/definition.md)。
