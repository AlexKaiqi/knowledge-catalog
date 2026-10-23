# DISCOVER-USAGE-01 · 使用记录：按成员公开数据，不给建议

## 目标

一次检索用到哪些声明成员、各自担什么角色（过滤还是打分）、哪些条件因未声明被拒，都记成可归因的数据；
回读能关联到产生它的那次查询；采纳反馈只能针对自己经搜索得到的回读，重复提交只计一次。维护者读到按成员
汇总的候选、回读、采纳计数与缺失需求，以及索引体积和维护延迟。这些只是数据，不写任何分支，不据以改声明、
退役修订或延长保留。

## 依据

- [发现](../../../doc/design/discovery.md)：使用记录、U9。
- [治理](../../../doc/design/governance.md)：反馈与工作分支。
- [服务](../../../doc/design/service.md)：证据规则，使用记录不进入检索、不成为授权规则。

## 验收

- [accept-10-usage](../../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-10-usage.yaml)
  四条规格全绿：`accept-search-usage-chain`、`accept-usage-data-only`、`accept-usage-feedback-boundaries`、
  `accept-usage-filter-vs-score`。
- 汇总不扩大谁能看到谁的访问：样本只给访问引用，无权主体读不到他人的原访问内容。
- 过滤成员与打分成员分开计数，不混在同一口径里。
- `DATASET-LIFECYCLE-01` 的 `accept-revision-usage-no-auto-retire` 中“使用数据逐修订列出访问时间与空间”
  一条随本条具备断言条件；本条不实现保留或退役判定。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`：空间语义定稿后，归因才有一致的检索范围。
- 与 `INDEX-PUBLISH-DECOUPLE-01` 无冲突：后者改发布时序与对外词汇，本条只加记录与汇总。

## 不做

不按使用记录自动改声明、自动退役修订或延长保留；不做意图分类或理解；不做回放与发现包（见同族
`discovery/` 下的后续任务）；不新增私有特权读取路径。

## 由来

2026-10-23 原 `INDEX-USAGE-01` 被删除，文件名带 `INDEX` 但与索引重构主线无关。删除后留下三处缺口：
[路线](../../../doc/roadmap.md) 近期第 6 项“按成员公开使用数据”失去承接、
[accept-10-usage](../../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-10-usage.yaml)
四条规格与 [accept.py](../../../.data/scenes/accept.py) 的归因指向不存在的任务、`FINAL-ACCEPTANCE-01` 把
“规格每条有执行证据”判为无法满足。

2026-10-23 按原文（git `c219b744`）恢复为 `DISCOVER-USAGE-01`，归入发现域，不再挂在索引主线名下。
它同时是 `todo/discovery/` 族里五条后续任务的母体：检索轨迹与意图、找 Dataset 的归因、数据画像、
按记录回放、可选发现包都要先有这份记录。
