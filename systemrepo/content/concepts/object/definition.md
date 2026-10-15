---
entity: kc/concept/object
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/object/definition.md
---
# 对象

具有独立身份、可以被引用和版本化维护的对象载体。对象可以描述一张表、一个指标、一项能力或一份规则。身份由来源 Repository 与 object_id 确定；不同仓的同名对象仍是不同对象，文件路径或交付布局变化不改变身份。

对象可以作为完整值维护，也可以分成多个切面。关系与 Schema 也具有对象身份。现行合同保留 Entity 作为普通对象整体值的 Address 名称，以及 entity 作为 Schema 类型名字段；产品解释统一称对象。

对象 URN 编码来源仓与 object_id，URN 后的 JSON Pointer 从对象内容根定位数据。空 locator 读取同一固定版本组装的整个对象；`/columns/order_id` 选择稳定键标识的 Member，`/columns/order_id/constraints/nullable` 继续选择该 Member 内的数据。版本依据与访问语境另行固定，Dataset 采用的来源版本不能由调用方覆盖；Dataset 消费范围不取代来源对象身份，也不授予源仓写权。

例如，以下入口读取订单列的 nullable 字段：

```text
kc read 'urn:kc:object:table/orders#/columns/order_id/constraints/nullable'
```

READ、PUT、REMOVE 共用对象 URN 与 locator。Writer 创建、替换或删除所指的完整维护单元；替换不会合并旧字段。整体值、完整 Aspect、Member 是维护粒度，深字段读取不产生任意字段 PATCH，按 Aspects 或 Members 组合的对象不能用组装结果整体覆盖。

对象不等于文件。定义可以描述含义、边界和组成，引用分散的内部对象、固定文件或领域外部标识；definition 是推荐组织约定，不强制统一资源集合结构。插件可以按 Schema 为对象增加访问能力，基本对象发现与读写不以插件为前提。

相关说明：[切面](../aspect/definition.md)、[关系](../relation/definition.md)、[对象地址](../address/definition.md)、[Schema](../schema/definition.md)。
