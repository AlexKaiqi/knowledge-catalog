# snapshot/

⓪ Snapshot authority：字面 path/blob/tree、不可变 commit、ref、expected-old CAS、merge 与 archive。

`Store` 是 Dataset 服务 成员唯一必须满足的接口；它不认识 `object_id`、Aspect、Schema、Binding 或检索。`TreeReader` 是固定 commit 的可选字面路径读能力；`TreeStore` 在其上增加 raw path 写，`TreeChangeSet` 只携带路径与字节。拆分后二者不得用一个模糊的“tree capability”互相代替。另有若干可选加速能力（`DirectoryReader`、`HistoryStore`、`ChangeStore`、`BulkTreeIngester`），经同型 type assertion 与 `XxxOf()` 发现；`BulkTreeIngester` 只换 `TreeChangeSet` 的对象传输原语（批量写通路），CAS 与产出内容与 `TreeStore.ApplyTreeCommit` 完全一致，未提供该能力的介质对显式请求确定性失败关闭。lakeFS 实现的批量写向隐藏 wip 分支的固定路径写入，因此幂等：传输错误、5xx 与 429 做有限次指数退避重试（每次重发完整正文），其它 4xx 不重试并带回 lakeFS 响应体；已观察的 5xx 来自 lakeFS 容器到对象存储的出站临时端口耗尽。

`command_contract.go` 提供跨写面的 CommandLedger/CommandStore；`write_contract.go` 提供基于 TreeStore 的 RawWriter，负责 CAS 与 Advanced 通知。构造返回接口，具体实现未导出。两者都不解释对象正文。Object PUT/REMOVE 由 `object/writer/` 编排。

`ProbeEmptyTree` 只检查固定 commit 的字面根目录，每页最多请求一个条目；任何条目（包括隐藏路径）
都代表非空。空页须沿有进展的游标读到同一 generation 的耗尽页才能证明为空。没有可选
`DirectoryReader` 时返回未知，不降级成全树 `ListFiles`；读取错误和无效分页证据原样失败关闭。

命令在进入写入前耐久预留，同 ID 同 digest 重放已保存回执，异 digest 拒绝。写入错误只有经
`WriteNotApplied` 显式证明目标未发布才释放预留；未知结果保留 `PENDING`，重开 ledger 也不
重新执行，须由 `ResolvePending` 或 `AbandonPending` 显式处置。该纯错误包装保留原错误码，
不按 HTTP 状态或临时错误推断发布结果。`WriteObserver` 接收真实结果，已有领域错误
原样保留；成功后若留证失败，同时返回记录错误与原 `APPLIED` / `REPLAYED` 回执，命令账
仍为 `APPLIED`。调用方可据回执核对已成立的提交；重试重放原回执，不重新提交。

lakeFS 的发布协调锁不会因重开或经过某段时间自动释放。持锁错误给出目标和候选坐标；
操作员先停止该权威的全部写入者，再调用 `RecoverLakeFSPublication`，传入完整分支 ref
及刚核对的目标、候选 commit。陈旧观察拒绝，释放失败原样报告；目标一致且锁已不存在时
重复恢复成功。此操作只释放协调残留，不推进 ref、不解析内容、不清除命令占位。
恢复完成后仍须独立核对未知写入结果并 `ResolvePending` 或 `AbandonPending`；底座没有
fencing token，因此必须维持停写直到恢复结束，不能把该函数作为在线抢锁接口。

`Advanced` 只报告 `{store, from, to}`。上层若需要对象变化集合，必须在②解释固定的两个 commit，不能把 `ObjectID` 塞回⓪事件。

生产后端只有 lakeFS，由 `OpenLakeFS`、`CreateManagedLakeFS`、`OpenManagedLakeFS` 返回 `Store`；私有实现在 `internal/lakefsimpl`。Gitea/filegit 存储接线和旧注册薄壳删除，身份认证不属于存储 adapter。

lakeFS 返回的 Store 还可经可选接口读写原生仓属性：
`ReadRepositoryMetadata() (map[string]string, error)` 读取当前属性，
`WriteRepositoryMetadata(map[string]string) error` 只增加或更新所供键，保留其他键。
属性保存在原生仓权威，不创建 Snapshot commit，不是固定历史版本的属性，
也不扩张基础 `Store` 接口。空属性是成功读取的空 map；认证、权限、
传输或不合法响应以错误返回，不当作属性缺失。实际读写仍受原生凭据与上层任务授权约束。

`datasetfs/` 消费上层已经解析好的 Workspace pin，并通过开源
`go-fuse/v2` 把若干 `TreeReader` 目录投影到 Linux 宿主。它不是新的
Snapshot adapter：不持有 ref、不产生 commit，也不扩展 raw path 写能力。

## Snapshot Store 产品模块

底层 `Store`/文件树接口保持现有窄合同。文件 Dataset、对应访问控制和完整控制台在本目录集中组织：

- `dataset/`：文件定义、固定来源组合、发布记录、历史、退役与路径路由；不解释对象。
- `access/`：主体、动作与资源范围的通用规则及显式授权管理；不拥有 KC 默认政策或外部身份目录。
- `service/`：独立 Dataset/权限 HTTP 与原始文件交付，只消费上述能力及存储/身份端口。
- `console/`：完整原 lakeFS 前端及新增 Dataset/权限页面，独立静态 handler 与构建资产。
- `client/`：本模块的 typed HTTP SDK；复用调用方注入的已认证运输，不保存身份或会话。
- `cli/`：可组合的文件资源命令、输入与帮助；通过 SDK 请求服务，由 `kc` 装配到统一入口。

旧 Dataset 服务 转发入口已移除；KC Server/Invocation 提供外部业务政策、对象语义视图、hook 和证据的适配，不持有另一份同能力实现。目录归属与逻辑协议角色分开：HTTP/presentation 子包不成为底层 `snapshot.Store` 的方法或新的协议层。

公开 import 闭集是 `kc/snapshot` 与 `kc/snapshot/dataset`、`access`、`service`、`console`、`client`、`cli`。一级包各有 contract.go、实现、测试和 README；没有任意深层公共 API。Dataset 依赖存储合同，Service 装配文件产品，底层不依赖 Dataset/Access/HTTP，领域核心与服务不反向依赖 CLI/SDK。外部 hook 和 evidence 通过窄回调接入，不暴露内部 journal。

客户端入口同归模块，命令仍按用户任务组织为 Dataset、文件与授权等既有路径。`kc` 保留认证上下文、全局入口、部署及跨模块流程；托管仓供给、连接恢复与初始发权属于应用管理，不因接口路径包含 snapshot 就成为底层存储合同。CLI 不读取 KC Home、应用 Invocation 或对象实现，发布、当前授权和文件范围仍由唯一服务执行。

根包使用私有 storecontract 共享纯文件坐标 DTO，避免根构造器与 lakeFS 实现的 Go import 循环；公开别名仅指向这些纯 DTO，不开放具体客户端或实现。

`HistoryStore.CommitHistory` 返回包含给定固定提交的 first-parent 链，最新在前；正数 limit
是调用者的总上限，通用提供方仍可能返回短批次，调用者从批次最后一个提交继续探测，
请求至少两个提交却仅得到该起点时才证明到达根提交。lakeFS 实现跟随物理分页以满足请求总量，
后续页失败或游标无进展时返回错误，不以部分列表冒充完整结果。
