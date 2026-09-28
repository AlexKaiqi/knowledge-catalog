# snapshot/

⓪ Snapshot authority：字面 path/blob/tree、不可变 commit、ref、expected-old CAS、merge 与 archive。

`Store` 是 Catalog 成员唯一必须满足的接口；它不认识 `object_id`、Aspect、Schema、Binding 或检索。`TreeReader` 是固定 commit 的可选字面路径读能力；`TreeStore` 在其上增加 raw path 写，`TreeChangeSet` 只携带路径与字节。拆分后二者不得用一个模糊的“tree capability”互相代替。另有若干可选加速能力（`DirectoryReader`、`HistoryStore`、`ChangeStore`、`BulkTreeIngester`），经同型 type assertion 与 `XxxOf()` 发现；`BulkTreeIngester` 只换 `TreeChangeSet` 的对象传输原语（批量写通路），CAS 与产出内容与 `TreeStore.ApplyTreeCommit` 完全一致，未提供该能力的介质对显式请求确定性失败关闭。

`commandlog/` 提供跨写面的 command-id replay/conflict ledger；`treewriter/` 是基于 `TreeStore` 的字面路径写服务，负责 CAS 与 Advanced 通知。两者都不解释知识正文。Knowledge PUT/REMOVE 由 `knowledge/writer/` 编排。

`Advanced` 只报告 `{store, from, to}`。上层若需要知识变化集合，必须在②解释固定的两个 commit，不能把 `ObjectID` 塞回⓪事件。

| 实现 | 介质 |
|---|---|
| `snapshot/gitea.Repository` | Gitea Git 对象 API + 分支 CAS |
| `snapshot/lakefs.Repository` | lakeFS Graveler + S3 兼容对象存储；要求 atomic assign 扩展 |

`datasetfs/` 消费上层已经解析好的 Workspace pin，并通过开源
`go-fuse/v2` 把若干 `TreeReader` 目录投影到 Linux 宿主。它不是新的
Snapshot adapter：不持有 ref、不产生 commit，也不扩展 raw path 写能力。
