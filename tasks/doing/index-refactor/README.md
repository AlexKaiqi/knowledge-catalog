# 索引重构（在途）

索引重构主线现在三条都在 `doing/`：`INDEX-PUBLISH-DECOUPLE-01`、`INDEX-PHYSICAL-01`、
`INDEX-SEARCH-SPACE-01`。它们不属于 `todo/index-refactor/` 里的任何一个实施批次，处置方式如下。

- 三条任务的物理布局与检索空间语义结论都保留，被替换的只有“准备先于接受”这个时序前提。
- `INDEX-PUBLISH-DECOUPLE-01` 是主线：发布先接受、构建后台追、进度与就绪可见、对外只有一套 index 词。
  它已把时序前提反转并落进设计篇，另两条只需在自己的事实记录里登记，不重做整项。
- `INDEX-PHYSICAL-01` 只在时序相关文件集上与 `INDEX-PUBLISH-DECOUPLE-01` 的实现串行，其余范围
  （资源生命周期与历史代回收、成员与向量产物复用、配额与清除、真实部署规模验收）继续独立推进。
- `INDEX-SEARCH-SPACE-01` 的四处判定随 `INDEX-PUBLISH-DECOUPLE-01` 的设计决定 11 改变，改动登记在
  该任务自己的事实记录里。

完整批次顺序、判据与边界写在 [todo/index-refactor/README.md](../../todo/index-refactor/README.md)，
那份是主线的唯一说明，这里不复制，避免两处说法分叉。
