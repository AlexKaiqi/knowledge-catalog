# snapshot/

⓪ Snapshot authority：字面 path/blob/tree、不可变 commit、ref、expected-old CAS、merge 与 archive。

`Store` 是 Catalog 成员唯一必须满足的接口；它不认识 `object_id`、Aspect、Schema、Binding 或检索。`TreeStore` 是可选的字面路径能力，`TreeChangeSet` 只携带路径与字节。

`commandlog/` 提供跨写面的 command-id replay/conflict ledger；`treewriter/` 是基于 `TreeStore` 的字面路径写服务，负责 CAS 与 Advanced 通知。两者都不解释知识正文。Knowledge PUT/REMOVE 由 `knowledge/writer/` 编排。

`Advanced` 只报告 `{store, from, to}`。上层若需要知识变化集合，必须在②解释固定的两个 commit，不能把 `ObjectID` 塞回⓪事件。

| 实现 | 介质 |
|---|---|
| `snapshot/dolt.DoltRepository` | Dolt |
| `snapshot/gitea.Repository` | Gitea Git 对象 API + 分支 CAS |

`workspacefs/` 消费上层已经解析好的 Workspace pin，并通过开源
`go-fuse/v2` 把若干 `TreeStore` 目录投影到 Linux 宿主。它不是新的
Snapshot adapter：不持有 ref、不产生 commit，也不扩展 `TreeStore`。
