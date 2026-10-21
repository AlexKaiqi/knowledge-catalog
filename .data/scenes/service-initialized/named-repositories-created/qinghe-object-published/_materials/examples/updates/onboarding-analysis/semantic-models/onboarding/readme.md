---
entity: example/qinghe-analysis
aspect: readme
schema_ref: schema/core/readme/v1
path_hint: semantic-models/onboarding/readme.md
---
# 从销售资料判断对象、成员和关系

本导读随 `qinghe-analysis` r3 交付。需要已认证身份与该 Dataset 的消费权。
先固定修订；本文所有样例在 r3 可读，来源仓本身不公开。

```sh
kc dataset show qinghe-analysis --revision 3
kc read 'urn:kc:object:example/qinghe-analysis#/readme' --dataset qinghe-analysis --revision 3
kc files read --dataset qinghe-analysis --revision 3 --path columns/orders.pay_amount.properties.yaml
kc files read --dataset qinghe-analysis --revision 3 --path semantic-models/shop.sales/dimensions/category.yaml
kc files read --dataset qinghe-analysis --revision 3 --path semantic-models/shop.sales/measures/gmv.yaml
kc files read --dataset qinghe-analysis --revision 3 --path relations-table-meta/orders-contains-columns.yaml
```

## 从需求到选择

原始材料是表结构、字段说明、语义模型与指标口径。以下给出可核对的选择依据，
不假称记录了原作者的设计过程，也不把数仓模型推广为所有领域的必选结构。

| 需求 | 本例表达 | 改动与替代方式 |
|---|---|---|
| 列需要被多个模型引用，并参与表的包含关系 | `column/shop.orders.pay_amount` 有独立身份 | 列属性独立修订；只需表内清单时也可建 columns 成员 |
| 类别维度与 GMV 度量属于指定语义模型 | 同一模型的 dimensions、measures 成员 | 按键更新一个定义，其他成员保持；跨模型独立管理的指标另建对象 |
| 业务商品与物理表不是同一件事 | 两个对象，由 recorded-in 关系连接 | 重建物理表不会改变商品身份 |
| 一起维护“表包含哪些列” | 一条多端 contains 关系 | 每条包含关联需要独立演进时可拆成多条关系 |

关系端点的 `role: member` 是业务角色，不是对象协议的 Member。
当前可执行关系端点连接对象 ID；关系指向对象内部成员的能力仍待实现。

## 做一次可解释的修订

先写下一个变更：“只修正 GMV 度量的说明，类别维度保持”。读取两个成员的值和维护依据，
在自己的练习仓复用匹配的 Schema 与模型资料，然后按系统的修订指南更新完整成员。
不要把修改深字段理解为字段级 PATCH，也不要用集合根替换所有成员。

```sh
kc read 'urn:kc:object:semantic-model/shop.sales#/measures/gmv' --dataset qinghe-analysis --revision 3
kc read 'urn:kc:object:semantic-model/shop.sales#/dimensions/category' --dataset qinghe-analysis --revision 3
kc read 'urn:kc:object:kc/guide/revise-an-aspect#/guide' --repo kr://kc/system
```

验收时对比更新前后的两个单元：GMV 的完整值发生预期变化，category 值未变；两者身份未变。
需要向同事交付时，另行发布 Dataset 并验证消费权。引用列对象不自动把列加入交付。
本例的表统计来自 LiveAccess，读取失败应保留逐值错误；理解模型可以只读取上面的静态单元。

## 学会后换一份资料

给自己的字段清单标出：哪些只是普通字段，哪些是 member，哪些应成为独立对象；
为每种选择写一个真实的修改任务和一个不采用替代方案的理由。再检查关系端点、
Schema、取值描述在消费范围中的可解释性与可读性。

找不到文件先核对 Dataset、修订和路径；不能改读来源仓或 latest 来掩盖范围错误。
建模方法入口：`kc/guide/model-from-materials`；首次发布入口：`kc/guide/first-publication`。
