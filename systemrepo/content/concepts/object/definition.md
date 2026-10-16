---
entity: kc/concept/object
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/object/definition.md
---
# 对象

具有独立身份、可以被引用和版本化维护的对象载体。对象可以描述一张表、一个指标、一项能力或一份规则。身份只由带命名空间的 object_id 确定；同一 object_id 在不同仓仍是同一个对象，文件路径、来源仓或交付布局变化不改变身份。

对象可以作为完整值维护，也可以分成多个切面；不同仓可以各自维护同一对象的不相交切面。在同一读取范围中，同一个切面来自两个仓会冲突，不按内容是否相同选择或合并。关系与 Schema 也具有对象身份；两份相同 Schema 不会因为放在不同交付目录就变成不同对象。现行合同保留 Entity 作为普通对象整体值的 Address 名称，以及 entity 作为 Schema 类型名字段；产品解释统一称对象。

对象 URN 只编码 object_id，URN 后的 JSON Pointer 从对象内容根定位数据。空 locator 读取当前范围内按固定版本依据组装的整个对象；`/columns/order_id` 选择稳定键标识的 Member，`/columns/order_id/constraints/nullable` 继续选择该 Member 内的数据。读取结果保留各切面的来源仓与 commit，跨仓对象的依据是版本向量。Repo、Dataset 访问范围另行指定，Dataset 采用的来源版本不能由调用方覆盖；Dataset 消费权也不授予源仓写权。

例如，以下入口读取订单列的 nullable 字段：

```text
kc read 'urn:kc:object:table/orders#/columns/order_id/constraints/nullable'
```

READ、PUT、REMOVE 共用对象 URN 与 locator。Writer 创建、替换或删除所指的完整维护单元；替换不会合并旧字段。整体值、完整 Aspect、Member 是维护粒度，深字段读取不产生任意字段 PATCH，按 Aspects 或 Members 组合的对象不能用组装结果整体覆盖。

对象不等于文件。定义可以描述含义、边界和组成，引用分散的内部对象、固定文件或领域外部标识；definition 是推荐组织约定，不强制统一资源集合结构。插件按已发布的方面或成员声明增加能力，Schema 只描述结构；基础精确读取与维护不以插件为前提。

相关说明：[切面](../aspect/definition.md)、[关系](../relation/definition.md)、[对象地址](../address/definition.md)、[Schema](../schema/definition.md)。
