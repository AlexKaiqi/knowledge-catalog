# INDEX-USAGE-01 · 使用记录：按成员公开数据，不给建议

## 目标

查询证据记录参与的声明成员与角色、因未声明被拒的条件；回读关联查询；反馈只能针对自己经搜索得到的
回读，重复只计一次。按成员汇总候选、回读、采纳次数，以及索引体积和维护延迟，作为数据公开给 Dataset
维护者。插件不提建议，不写任何分支。

## 依据

- [发现](../../doc/design/discovery.md)：使用记录、U9。
- [治理](../../doc/design/governance.md)。

## 验收

- [accept-10-usage](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-10-usage.yaml) 全绿。
- 汇总不扩大谁能看到谁的访问：样本只给引用。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

`INDEX-SEARCH-SPACE-01`。原重构阶段 4d 的记录部分没有实现，由本条一并完成。

## 不做

不按使用记录自动改声明、自动退役修订或延长保留。
