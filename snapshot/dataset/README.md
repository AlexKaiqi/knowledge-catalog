# snapshot/dataset

本包拥有 Snapshot Store 的文件组合、Dataset 发布、固定范围、路径交付与登记历史。
一套服务只有一个 `Service` 和一份登记权威，多 Repository、多 Dataset 共享这份权威。
没有可创建、选择、切换或归档的 Catalog 资源。

底层 `snapshot.Store` 提供单仓版本、文件树和条件推进；本包只处理来源仓、固定 commit、
文件路径和交付路径，不解释对象、Aspect、Schema 或索引。来源标识是
`RepositoryID + CommitID + File/SubPath`；`Target/Path` 只决定交付位置。

## 公开入口与状态

`NewService(sources, registry)` 装配唯一文件组合服务。部署先显式调用
`CreateSnapshotRegistry(store, ref)` 创建登记权威；启动和恢复使用
`OpenSnapshotRegistry(store, ref)`。空 ref 使用 `snapshot.DefaultRef`。
`Registry.AuthorityID()` 和 `State.AuthorityID` 均取实际 Snapshot 的 `store.ID()`；
它们用于核对恢复权威和审计依据，不是请求中可选择的组合空间。

`State` 保存来源成员、Dataset 当前定义与已接受版本。来源连接配置、凭据、权限、
文件正文和临时 pin 不进入这份状态。来源连接、登记、发布、授权分别成立：创建或连接
一个仓不会自动授予消费权，登记也不把整仓加入每个 Dataset。

`RegisterRepository` / `UnregisterRepository` 改变成员准入。
`RetireDataset` 改变当前 Dataset 的消费政策；来源仓归档仍由 Snapshot 仓生命周期
决定。本包没有整份登记空间的归档动作。

## 发布与消费

`ParseSourceMappings([]string)` 与 `ParseFileMappings([]string)` 解析便携来源坐标，供
argv 或其它输入 adapter 使用，不读取 flags、环境或 Home。目录形式为
`repository[=selector][@path[@subPath]]`；单文件形式为
`repository[=selector]@file@target`。省略 selector 使用 `snapshot.DefaultRef`；省略目录
`@path` 与显式根映射 `@` 保持区别。解析不冻结版本、登记成员或授权，发布仍走下面的服务。

`PrepareDataset` 冻结候选来源；`PublishDataset` 原子接受发布记录并推进当前
版本；`DefineDataset` 是这两步的纯文件组合入口。新 revision 必须大于当前版，
不得覆盖已接受版本或倒退。源分支前进不会改变已发布 Dataset，对象投影准备由上层负责。
准备与发布均核对逐文件选择的冻结源文件可读性，并按目标路径核对与目录来源的真实内容
冲突；失败不推进当前版本、发布历史或登记 HEAD。冲突探测只读取可能相撞的路径及其
祖先，并用 `DirectoryReader` 的单项页检查目标下是否存在内容，不扫描来源 prefix。
需要目录探测但来源缺该能力时明确拒绝，不能用 `ListFiles` 模拟。
根 fallback 与显式 prefix 并存时，还用根来源固定 commit 的 `TreeReader.ReadFile`
点查显式目录目标及其祖先，直到已选根子树边界；真实文件不能占用声明目录位置。
该检查不枚举任何 prefix，也不读取显式挂载的文件；缺少点读能力失败关闭。
未声明交付目录的 `Path: nil` 来源保持原有坐标语义。

`RouteDatasetFile(items, deliveredPath)` 从已固定清单编译单文件来源坐标：精确单文件条目
优先，其余路径按显式 prefix 或根 fallback 路由。返回既有 `DatasetItem` 的副本；prefix
结果保留原映射字段，并把映射后的原仓路径放在 `File`。无匹配返回
`OBJECT_REF_UNRESOLVED`，空或逃逸路径返回 `USAGE_INVALID`。该纯 helper 不读取正文
或证明文件存在。显式单文件占用根 fallback 的精确目标；源祖先文件或目标下的源内容
仍不能造成文件与目录冲突。`ReadVirtualFileAt` 复用该编译并核对固定 commit；旧虚拟读取
入口的未归属路径继续返回 `USAGE_INVALID`。
`CheckDatasetFileConflict(items, deliveredPath, source)` 共享发布时的路径/目录判定，供
消费者对旧清单失败关闭；它只检查该显式文件目标，不枚举 prefix。调用方须先授权
冲突依据来源，再提供取得 Store 的回调。

`Dataset.Dependencies` 是可选的 `{repository → commit}` 引用依赖，由上层准备在 `PrepareDataset`
返回的候选上填写，与来源一起固定在发布 revision 中。它只用来解释选入单元引用的、不在选入仓里的
对象（如取值描述），不进入文件清单、成员与读权。`PublishDataset` 校验坐标非空、依赖仓已挂载、
提交存在；依赖仓也是来源时提交必须与冻结来源一致。有依赖时 PinID 把依赖纳入计算，无依赖时
保持历史值；回放与 `ResolvedDataset` 都带出依赖。

`Dataset.Metadata` 是可选的字符串键值 map，与文件选择一起固定在发布 revision 中。
调用方可以在 `PrepareDataset` 返回的候选上设置 metadata，再交给 `PublishDataset`；
原有 `DefineDataset` / `PrepareDataset` 签名保持不变。metadata 原样进入当前定义、
不可变发布历史、库存摘要、管理详情与 `.kc-dataset.yaml` 便携配方。`description` 可以作为
普通自定义 key 使用，不是必填字段，也不引入 Schema 或嵌套值。省略 metadata 的旧记录继续
恢复；修改或清除 metadata 需要发布新的 revision。map 在状态、视图与返回值之间独立复制。
metadata 不读取、识别或处理 README，不增加任何来源或文件范围。
公开定义与配方保持纯 DTO。`DecodeJSON` 严格检查它们及嵌套定义中的 metadata；需要保留
自身 JSON 数值精度和字节预算的传输层可以另行调用 `ValidateMetadataJSON(raw, target)`。
该验证端口只沿目标 Go 类型识别 Dataset / 配方，`any` 与 `json.RawMessage` 正文保持不透明。

`DatasetVersion` 返回已接受历史记录。`ReplayedDefinition(svc, setID, revision, sources, metadata)`
判定一次发布请求是否只是已接受修订的重试：该修订已存在、请求来源（忽略服务端冻结的
commit）与 metadata 完全相同时返回已接受定义，调用方以 `disposition: REPLAYED` 回答且不再
准备或写入；首次接受回答 `APPLIED`。同修订号但配方不同仍由 `PublishDataset` 拒绝为
`NON_FAST_FORWARD`。本地 `dataset define` 与 HTTP 发布都在授权之后调用它。
请求显式提供 commit 时必须与该已接受来源提交一致；省略 commit 的分支重试不随当前 HEAD 重新冻结。
`ReplayPublished` 从可信发布记录恢复 pin，再
核对客户端坐标与文件清单；客户端摘要不能授权自己。`ReplayDefinition` 仅用于另行授权
的临时配方，不能借命名 Dataset grant 扩大范围。成员准入、退役和当前权限继续约束旧 pin。
空清单不是整仓别名，父目录跳转与未选入文件不能扩大范围。
`ReplayPublished` 只复核当前成员、来源生命周期与固定 commit，不读取文件正文。
完整 `CheckResolved` 另外核对逐文件可用性；其调用方须先完成消费授权，不能把该正文
检查塞进文件授权之前的 pin 恢复或目录请求。

`ReadView` 固定本次已接受状态，拥有独立状态映射、审计上下文和身份印记；不重新加载
远端，不订阅推进事件，不写权威。`Log(LogQuery)` 返回 `(History, error)`，读取登记 Snapshot
历史，可以按 Dataset 文件筛选；这是登记历史，不是来源仓正文历史。缺失历史能力、历史
读取和可用提交元数据回读的错误明确返回，不能当作成功的空历史；不提供元数据能力时
仍保留真实提交坐标。

## 持久格式与恢复

登记权威是独立 Snapshot，普通 YAML 文件不使用对象文件信封：

| 文件 | 内容 |
|---|---|
| `registry.yaml` | 实际 `authorityId` 与唯一、非空 `repositories` 名单 |
| `dataset-<token>.yaml` | 当前完整 Dataset，包含命名、正整数 revision 与冻结来源 |
| `release-<setId SHA256>-v<revision>.yaml` | 不可变已接受发布记录，不保存当前退役政策 |

`Registry.Save` 在读取的 expected-old 上执行 Snapshot CAS；提交前已证明未发布的拒绝
不改变可见状态与 HEAD，陈旧句柄不能覆盖新状态。发布后的应答丢失可能已经推进 HEAD，
旧句柄不会伪造接受，后续需核对或重开实际权威。并发读取或启动不会初始化、迁移或重建权威。
`SetObserver` 配置的 observer 接收登记持久化的实际操作结果：原操作失败时，无论回调返回
nil 或其它错误，都原样保留原失败及错误码；原操作成功后，记录失败明确返回给调用方。
记录错误不回滚已接受的成员或 Dataset，也不证明登记 HEAD 未推进。`PublishDataset`
在返回错误时可以不返回定义；调用方应核对 `DumpState` 或重开实际权威的已接受记录，
再决定是否重试，不能把 observer 错误解释为未发布。
缺失、错误类型、未知字段、重复键、null、多个 YAML 文档、身份或文件名不匹配均失败。
显式创建仅接受全树为空且 `HistoryStore` 证明当前 commit 是无父初始根的 authority；
已有历史、外来文件或无法证明初始根均拒绝。已接受登记的 YAML 全部丢失仍是损坏，
不能凭当前空树重建空账。

旧唯一登记账的 `catalog.yaml` 和旧 `repository-*.yaml` 仅为恢复兼容读取。旧 metadata 的
非空 `id` 必须匹配实际 Snapshot 的 `store.ID()`，但不成为请求选择参数；已有 Dataset 版本、
来源坐标与审计历史保留。打开不会写入，幂等操作也不迁移；第一次成功的明确状态变更
写出 `registry.yaml` 并移除旧 metadata/成员文件，旧 Snapshot 历史仍可读。
旧 `id` 不匹配实际 Snapshot 时返回 `PRECONDITION_FAILED`，明确要求显式迁移；打开与
创建都不能采用这份权威或推进 HEAD。新旧 metadata 并存、旧账 `archived: true`、
缺 metadata 或损坏状态明确拒绝，不能当作
首次创建。已有只存当前版的旧账仅保留该已接受版本，不补造更早历史。

## 验证

回归覆盖文件发布与交付布局、固定来源、范围校验、当前退役、历史回放、只读视图、
权威身份、失败保持、并发 CAS 和只读旧账恢复。上层依赖仅进入公开根包/一级能力；
本包不依赖 Object、索引、Home、Invocation、Server 或具体 Snapshot adapter。
包内夹具验证不等于真实 lakeFS 部署验收。

## 按已接受发布选择消费范围

命名历史消费复用 `DatasetVersion` 与 `ReplayPublished`：
版本记录先受当前退役政策约束，仅从已接受记录的冻结来源建立候选 pin，再按已接受记录和当前
成员、来源状态复核。调用方不从管理详情自行拼装 pin，也不通过临时定义借用命名
Dataset grant。该路径没有扩大 `Service` 接口；传输层的发布版选择归相邻 service。
