# QUESTION-OBJECT-01 · 对象发现形状差异是分工还是漂移

## 要回答的问题

`revision`、`dataset`、`hits`/`nodes` 在三处返回里的位置差异属于设计分工还是形状漂移；
`dataset show` 把顶层 `revision` 与 `items[].commit` 放同层是否会被误读为同源版本；
零知识路径（只给业务词）能否不查文档抵达目标对象。

## 已知证据

`revision`：`dataset show` 顶层数字、`search` 嵌于 `dataset: {id, revision}`、`files list` 顶层数字。
`dataset`：`search` 为对象、`files list` 为字符串。`relations` 返回 `hits`，`traverse` 返回
`nodes`/`edges`。

`traverse --max-hops` 上限 1-8 已在 help 中声明（实测输出 `--max-hops 为 1-8`），"无上限说明"的
原判断不成立。

## 判定标准

- 每处差异能对上设计条款，关闭为"设计分工 + 文档缺口"；对不上，关闭为"形状漂移"，按契约缺陷另开任务。
- 顶层 `revision` 与 `items[].commit` 混放导致实测中产生误读，关闭为"确认缺陷"；未产生误读，
  关闭为"可接受"并登记文档提示缺口。
- 零知识路径记录为抵达目标所需的命令序列与人工判断次数；若必须读 README 才能继续，关闭为"发现缺口"。

## 前提

无。

## 不涉及

不为视觉统一改既有返回形状、不新增查询 DSL、不把 `traverse` 改成枚举路径。

## 结论

待填。
