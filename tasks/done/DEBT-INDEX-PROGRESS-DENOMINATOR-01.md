# DEBT-INDEX-PROGRESS-DENOMINATOR-01 · 追赶进度的分母没有设计出处

## 债

- 位置：索引层的构建进度上报与 `discovery/projection/engine.go:161-171` 的 `RebuildSession` 分批构建。
- 信号：`1240/87056` 的分母在 `doc/design/discovery.md` 的索引维护一节里没有定义。
- 判定：新增观测能力带来的设计缺口，不是实现问题。若不定口径，`total` 与 `objectCount` 会被当成同一件事。
- 处置：在设计篇的索引维护一节补一句口径。
- 触发条件：进度口开工即处理，与该任务的实现同一次提交；不得先实现后补文档。

## 关闭

2026-10-22 并入 `INDEX-PUBLISH-DECOUPLE-01`，未按本文件单独验收。

并入理由：本条的唯一交付是设计篇里的一句话，而 `INDEX-PUBLISH-DECOUPLE-01` 的验收里本来就要求改
`doc/design/discovery.md` 的索引维护一节（`done`/`total` 口径）。一句话的债挂在不改这份文档的任务上
没有意义；挂在改它的任务上，反而能保证"不得先实现后补文档"这条约束被真正执行。

承接内容与变化：

- 原"处置"那句口径（按对象计、含被声明排除对象、`objectCount` 是可检索文档数、两者不相等也不互相印证）
  已写成 `INDEX-PUBLISH-DECOUPLE-01` 的设计决定 5。
- 原"目标""验收"里的三条（设计篇一句话说清计数单位、明确 `total` 与 `objectCount` 的关系、`indexes:status`
  与 `kc index` 的文档措辞与该定义一致）全部落在该任务的设计篇修订与验收第 14 条上。
- 原"不做"里"不为了消除差异把 `total` 改成可检索文档数"一条保留：那会把排除对象藏在进度之外，
  让追赶看起来永远追不上。
