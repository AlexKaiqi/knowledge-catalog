# DISCOVER-PROFILE-01 · Dataset 修订的数据画像

## 目标

调用者（人或 Agent）在写索引声明或决定怎样搜索之前，能读到一个 Dataset 修订按 Schema 字段的数据画像：
采用该 Schema 的对象数、字段覆盖率与缺值比例、基数估计、数值与日期范围、文本长度分布、外部单元中
`shared` 与 `per-caller` 的占比。画像绑定 Dataset 修订，作为独立的只读能力，不随搜索返回；值是估计时
明确标为估计并给出依据。

## 依据

- [发现](../../doc/design/discovery.md)：不做什么（分面与总数做成独立能力）、能力清单、进度表“未到”里的分面与总数。
  需在该篇补写画像的范围与完整性说明。
- [Affordance](../../doc/design/affordance.md)：画像是数据侧的说明，与能力清单并列。

## 验收

- `discovery.md` 补写后有对应用例，场景树规格全绿。
- 同一 Dataset 修订的画像可重复得到；修订推进后旧修订的画像仍按旧修订给出。
- 不能借画像读到逐对象的值；对没有消费权的 Dataset 拒绝。
- 画像计算受预算约束，超预算明确报告未完成，不给出冒充完整的数字。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

`INDEX-SEARCH-SPACE-01`。

## 不做

不按画像自动生成声明；不做逐查询的分面；不返回样本值原文。
