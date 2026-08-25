# snapshot/

⓪ Snapshot authority：字面 path/blob/tree、不可变 commit、ref、expected-old CAS、merge 与 archive。

`Store` 是 Catalog 成员唯一必须满足的接口；它不认识 `object_id`、Aspect、Schema、Binding 或检索。`TreeStore` 是可选的字面路径能力，`TreeChangeSet` 只携带路径与字节。

`Advanced` 只报告 `{store, from, to}`。上层若需要知识变化集合，必须在②解释固定的两个 commit，不能把 `ObjectID` 塞回⓪事件。

| 实现 | 介质 |
|---|---|
| `snapshot/filegit.FileGitRepository` | 本机 Git |
| `snapshot/dolt.DoltRepository` | Dolt |
| `snapshot/gitea.Repository` | Gitea Git 对象 API + 分支 CAS |
