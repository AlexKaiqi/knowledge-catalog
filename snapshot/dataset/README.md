# snapshot/dataset

本包拥有 Snapshot Store 的文件组合、Dataset 发布、固定范围、路径交付与登记历史。
一套服务只有一个 `Service` 和一份登记权威，多 Repository、多 Dataset 共享这份权威。
没有可创建、选择、切换或归档的 Catalog 资源。

底层 `snapshot.Store` 提供单仓版本、文件树和条件推进；本包只处理来源仓、固定 commit、
文件路径和交付路径，不解释知识对象、Aspect、Schema 或索引。来源标识是
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
`RetireKnowledgeSet` 改变当前 Dataset 的消费政策；来源仓归档仍由 Snapshot 仓生命周期
决定。本包没有整份登记空间的归档动作。

## 发布与消费

`PrepareKnowledgeSet` 冻结候选来源；`PublishKnowledgeSet` 原子接受发布记录并推进当前
版本；`DefineKnowledgeSet` 是这两步的纯文件组合入口。新 revision 必须大于当前版，
不得覆盖已接受版本或倒退。源分支前进不会改变已发布 Dataset，知识投影准备由上层负责。

`KnowledgeSet.Metadata` 是可选的字符串键值 map，与文件选择一起固定在发布 revision 中。
调用方可以在 `PrepareKnowledgeSet` 返回的候选上设置 metadata，再交给 `PublishKnowledgeSet`；
原有 `DefineKnowledgeSet` / `PrepareKnowledgeSet` 签名保持不变。metadata 原样进入当前定义、
不可变发布历史、库存摘要、管理详情与 `.kc-dataset.yaml` 便携配方。`description` 可以作为
普通自定义 key 使用，不是必填字段，也不引入 Schema 或嵌套值。省略 metadata 的旧记录继续
恢复；修改或清除 metadata 需要发布新的 revision。map 在状态、视图与返回值之间独立复制。
metadata 不读取、识别或处理 README，不增加任何来源或文件范围。
公开定义与配方保持纯 DTO。`DecodeJSON` 严格检查它们及嵌套定义中的 metadata；需要保留
自身 JSON 数值精度和字节预算的传输层可以另行调用 `ValidateMetadataJSON(raw, target)`。
该验证端口只沿目标 Go 类型识别 Dataset / 配方，`any` 与 `json.RawMessage` 正文保持不透明。

`DatasetVersion` 返回已接受历史记录。`ReplayPublished` 从可信发布记录恢复 pin，再
核对客户端坐标与文件清单；客户端摘要不能授权自己。`ReplayDefinition` 仅用于另行授权
的临时配方，不能借命名 Dataset grant 扩大范围。成员准入、退役和当前权限继续约束旧 pin。
空清单不是整仓别名，父目录跳转与未选入文件不能扩大范围。

`ReadView` 固定本次已接受状态，拥有独立状态映射、审计上下文和身份印记；不重新加载
远端，不订阅推进事件，不写权威。`Log(LogQuery)` 读取登记 Snapshot 历史，可以按 Dataset
文件筛选；这是登记历史，不是来源仓正文历史。

## 持久格式与恢复

登记权威是独立 Snapshot，普通 YAML 文件不使用知识文件信封：

| 文件 | 内容 |
|---|---|
| `registry.yaml` | 实际 `authorityId` 与唯一、非空 `repositories` 名单 |
| `dataset-<token>.yaml` | 当前完整 Dataset，包含命名、正整数 revision 与冻结来源 |
| `release-<setId SHA256>-v<revision>.yaml` | 不可变已接受发布记录，不保存当前退役政策 |

`Registry.Save` 在读取的 expected-old 上执行 Snapshot CAS；拒绝或持久化失败不改变
可见状态与 HEAD，陈旧句柄不能覆盖新状态。并发读取或启动不会初始化、迁移或重建权威。
缺失、错误类型、未知字段、重复键、null、多个 YAML 文档、身份或文件名不匹配均失败。

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
本包不依赖 Knowledge、索引、Home、Invocation、Server 或具体 Snapshot adapter。
包内夹具验证不等于真实 lakeFS 部署验收。
