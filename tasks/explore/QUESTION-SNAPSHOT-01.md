# QUESTION-SNAPSHOT-01 · 存储权威的失败与重试语义是否如 README 所述

## 要回答的问题

`BulkTreeIngester` 的批量写在 CAS 与产出内容上是否与 `TreeStore.ApplyTreeCommit` 完全一致；
`ProbeEmptyTree` 在没有 `DirectoryReader` 时是否真返回未知；未决写结果是否只有 `WriteNotApplied`
能释放预留。

## 已知证据

[snapshot/README.md](../../snapshot/README.md) 声明：批量写"CAS 与产出内容与 `TreeStore.ApplyTreeCommit`
完全一致"；`ProbeEmptyTree` "没有可选 `DirectoryReader` 时返回未知，不降级成全树 `ListFiles`"；
"写入错误只有经 `WriteNotApplied` 显式证明目标未发布才释放预留"。三项目前都只有文字声明，
未见对照用例证据。

## 判定标准

- 相同 `TreeChangeSet` 下两条写入路径产出的 commit 与 CAS 行为一致，关闭为"合同成立"；有差异，
  关闭为"确认缺陷"。
- 无 `DirectoryReader` 时返回未知而非列举，关闭为"合同成立"；分页未耗尽即判空，关闭为"确认缺陷"。
- 三态（`APPLIED`/`PENDING`/`ABANDONED`）在真实后端失败注入下各出现一次且预留释放条件正确，
  关闭为"合同成立"。
- 任何一处"按 HTTP 状态或临时错误推断发布结果"的代码路径，关闭为"确认缺陷"并定位。

## 前提

写侧取证限定在专用测试仓，不在生产仓执行破坏性写。

## 不涉及

不新增后端适配器、不恢复已退役的存储接线；不改动 `READ-BASIS-01` 等对象版本任务正在修改的文件。

## 结论

待填。
