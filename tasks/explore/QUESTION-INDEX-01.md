# QUESTION-INDEX-01 · 索引接口耗时与检索形状是否可解释

## 要回答的问题

`operations access-spec describe` 稳定 30 秒超时是确定性缺陷还是依赖抖动；检索结果的排序依据能否
从返回形状判断；`completeness` 与 `continuation` 并存时语义是否唯一。

## 已知证据

`kc operations access-spec describe --dataset qqbrowser` 连续三次均为 `real 30.01`；同一时刻
`kc operations projection describe --dataset qqbrowser` 为 1.26/1.17/1.17 秒。两者同源只读接口
差 25 倍以上，30.01 的重复精度指向固定超时线而非负载。

`kc search --dataset qinghe-analysis --query 茶` 的 hit 键为 `commit`、`evidence`、`objectId`、
`repository`、`version`，无 `score`；`evidence[].localRank` 为 1。顶层 `completeness: complete`、
无 `continuation`。

## 判定标准

- 超时项：连续 5 次采样若中位数贴近同一阈值，关闭为"确定性缺陷"并定位到具体超时来源；若随负载波动，
  关闭为"抖动"，需给出波动区间。
- 排序项：若设计以 rank 而非 score 表达强弱，关闭为"设计取舍 + 文档缺口"，不改返回形状；若设计要求
  可比分数，关闭为"缺陷"。
- 语义项：以设计原文为准判定 `complete` 指"结果完备"还是"本次未截断"；设计未定义时关闭为"设计缺口"，
  不由实现自行选定。

## 前提

无。

## 不涉及

不改排序算法、不扩张返回形状、不缩短既有超时阈值；不触碰 `INDEX-PHYSICAL-01`、
`INDEX-SEARCH-SPACE-01` 正在修改的文件。

## 结论

待填。
