# snapshot/

⓪ Snapshot authority：字面 path/blob/tree、不可变 commit、ref、expected-old CAS、merge 与 archive。

`Store` 是 Catalog 成员唯一必须满足的接口；它不认识 `object_id`、Aspect、Schema、Binding 或检索。`TreeReader` 是固定 commit 的可选字面路径读能力；`TreeStore` 在其上增加 raw path 写，`TreeChangeSet` 只携带路径与字节。拆分后二者不得用一个模糊的“tree capability”互相代替。另有若干可选加速能力（`DirectoryReader`、`HistoryStore`、`ChangeStore`、`BulkTreeIngester`），经同型 type assertion 与 `XxxOf()` 发现；`BulkTreeIngester` 只换 `TreeChangeSet` 的对象传输原语（批量写通路），CAS 与产出内容与 `TreeStore.ApplyTreeCommit` 完全一致，未提供该能力的介质对显式请求确定性失败关闭。

`commandlog/` 提供跨写面的 command-id replay/conflict ledger；`treewriter/` 是基于 `TreeStore` 的字面路径写服务，负责 CAS 与 Advanced 通知。两者都不解释知识正文。Knowledge PUT/REMOVE 由 `knowledge/writer/` 编排。

`Advanced` 只报告 `{store, from, to}`。上层若需要知识变化集合，必须在②解释固定的两个 commit，不能把 `ObjectID` 塞回⓪事件。

| 实现 | 介质 |
|---|---|
| `snapshot/gitea.Repository` | Gitea Git 对象 API + 分支 CAS |
| `snapshot/lakefs.Repository` | lakeFS Graveler + S3 兼容对象存储；原生锁分支与 hard-reset 条件发布 |

`datasetfs/` 消费上层已经解析好的 Workspace pin，并通过开源
`go-fuse/v2` 把若干 `TreeReader` 目录投影到 Linux 宿主。它不是新的
Snapshot adapter：不持有 ref、不产生 commit，也不扩展 raw path 写能力。

## Snapshot Store 产品模块

底层 `Store`/文件树接口保持现有窄合同。文件 Dataset、对应访问控制和完整控制台在本目录集中组织：

- `dataset/`：文件定义、固定来源组合、发布记录、历史、退役与路径路由；不解释知识对象。
- `access/`：主体、动作与资源范围的通用规则及显式授权管理；不拥有 KC 默认政策或外部身份目录。
- `service/`：独立 Dataset/权限 HTTP 与原始文件交付，只消费上述能力及存储/身份端口。
- `console/`：完整原 lakeFS 前端及新增 Dataset/权限页面，独立静态 handler 与构建资产。

上层 Catalog 公开路径只留兼容入口；KC Server/Invocation 提供外部业务政策、知识语义视图、hook 和证据的适配，不持有另一份同能力实现。目录归属与逻辑协议角色分开：HTTP/presentation 子包不成为底层 `snapshot.Store` 的方法或新的协议层。
