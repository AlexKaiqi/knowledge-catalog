# Discovery 的投影消费合同

本包保存查询编排与提供方共同消费的合同。声明式索引的实际构造、编译、执行和维护
分别归 [`declarativeindex/projection`](../../declarativeindex/projection/README.md) 与
[`declarativeindex/control`](../../declarativeindex/control/README.md)，本包不持有运行实例或转发实现。

`Retriever` 只返回 `CandidatePage`，候选携带来源仓、固定版本和对象身份，不能交付正文。
`ProjectionMaintainer` 管理可重建的物理投影；`Engine` 组合这两种能力。
`StreamingProjectionMaintainer` / `RebuildSession` 的分批构建只有 Commit 才发布，Abort 保留
之前的就绪代。`ContextRetriever`、`ContextMetaLoader` 是有界执行端口，
`ExpressionProber` 和 `SemanticWindowRetriever` 必须分别证明组合和语义窗口能力。
登记成功不证明任何查询已经支持或就绪。

`ResourceMaintainer` 是可选的后台物理恢复端口，返回已确认删除、仍受未决保护的资源及有界
扫描是否尚有后续。它不授予按物理资源选择查询的能力；提供方负责持久准入与删除隔离，
维护方负责周期调用和报告未决保护，查询或只读元数据路径不调用它。资源归属未知不能当空集合。

`ResourcePreparer` 是提供方签发固定就绪代票据的可选端口。`PreparedResource` 的准备保护有期限，
准入持久保存上层的不透明发布证据；准入后不再因准备期限结束而释放。`Confirm` 记录已知接受，
`Cancel` 只释放从未准入的准备，未知准入或接受须经权威恢复。`Close` 只释放本地句柄。
签发须核对完整 `Meta`、实际介质就绪和确权记录，不能按名称接管旧资源；与物理写入和删除使用同一排他。

`CompiledDoc` 是完整对象的提供方文档，Aspect / Member 仍是维护单元。
`ProjectionCell` 使用完整字段身份及类型槽位，适用而缺值与 Schema 不适用分别表示。
`Meta`、`Capability`、`Guarantee` 是物理依据和能力证据；`IndexDescriptor`、`IndexSync`、
`StateSync`、`ProjectionTarget` 是实际贡献的诊断、维护结果与进度，工作账不代替权威版本。
这些形状的真实定义只有本包一份，具体实现直接消费。

`WithSearchBudget` 为一次组合查询建立共享的候选数、后端页数及截止时间，嵌套调用保留
同一预算。默认 10,000 候选、100 页、30 秒；零值采用默认值。
`ReserveSearchPage` 在同一上下文内并发预留并归还未用容量，`SearchContextStatus` 与
`SearchBudgetExhausted` 区分预算期限和调用方取消。上下文 key、计数器和期限原因唯一。

`WithCandidateScope` 在正文读取前约束固定来源范围；`FilterCandidateScope` 先拒绝错误
来源或版本，再按实际范围过滤并保留分页进度。`CandidateScopeContains` 检查单个身份，
`CandidateScopeDigest` 为续页提供同一范围摘要。它们共享同一个上下文范围，不保存索引。

`WithCandidateRecheck` 携带已经授权、固定 Dataset 范围的消费方回读函数。
`RecheckCandidate` 在观察候选占用结果名额之前调用它；提供方再用同一声明解释新观察并
补判条件。淘汰后继续取候选，沿用同一个页数、候选数和时间预算，不让后置过滤悄悄把
产品页缩短。该接缝不刷新共享索引，不把调用者身份放入后台维护配置。

`ChangeNotice` / `ParseChangeNotice` / `ValidateChangeNotice` 只接受定位和版本提示，拒绝
正文或实际值。`ObservationSource` 从固定声明取得已经验证的逻辑值；后台实际身份、
控制调度和原子发布由具体贡献负责，不放入通用查询合同。

`PublicationResourceMaintainer` 以发布权威 resolver 对账未决准入，返回确认、解除和未知保护；
这些是引用变化，不代表删除物理语料。提供方在权威 IO 之后以资源 CAS 复查同一票据与原证据，
并发确认的引用不能被旧判定删除。活动准入不做恢复围栏；原有限期限结束后才进入权威恢复，
到期本身仍不给释放资格。
`ResourceCoordinator` 是显式实例 namespace 的后台资源权威，构造不访问后端，扫描不依赖已打开
查询空间或来源需求。其本地生命周期由插件根拥有；查询与只读 reconcile 不运行恢复。

`PublicationBinder` 用已接受的不透明准入证据打开确切生成代的只读 `Engine`，不按物理名称选择。
绑定在 Dataset CAS 之前已经持久就绪；查询不创建、修复或确认绑定。已知采用身份的绑定/保护/介质
缺失时明确失败，不回退到活动指针。返回视图保留原元数据、计数及固定续页依据，拒绝维护写入。

托管 `PublicationBinder` 为返回的语料只读视图取得独立的持久查询保护，与原发布引用分开。
视图受本次请求和提供方上限约束；关闭解除本查询引用。过期请求不能继续读取元数据或语料，
进程丢失后的期限对账不确认或释放未知发布；单次查询保护不充当续页或观察代保留。

`PublicationContinuationBinder` 从不透明原发布证据及提供方续页恢复其受保护生成代；
不依赖当前映射，不重新准备。期限、范围或耐久引用缺失时失败。提供方续页保护与单次
请求保护分开，接续成功不延长原宽限；实际 Dataset 保留和观察代期限须另外核对。

`ContinuationSealStore` 将第一次准备与已有游标加载分开；加载不能创建或替换未知签名材料。
返回的 `ContinuationSealer` 只认证不透明游标正文，可在请求中本地封装/核验，不打开查询代或
取得权限。语料续页的期限和引用仍由 `PublicationContinuationBinder` 与实际资源权威验证。

`ObservationBinder` 为一个已发布观察取得只读生成代视图及独立有界请求租约。空提供方游标只解析一次
当前观察；已有游标必须恢复原生成代，并同时核对原续页租约和被替换观察的原截止时间，不能回退最新值。
关闭仅释放本请求租约。已取得保护的请求可在其有界期限内继续读取，观察到期后拒绝新续页准入。
该端口不授予 Dataset/来源权限，不判断 Dataset 的检索保留、退役或清除；调用方须先完成相应判定。

具名 Dataset 的观察由上层实际权威选择 `ObservationReference`，提供方只消费不透明证据，
不解释 Dataset 协议。当前证据经 `PublicationBinder` 打开；旧观察续页携带权威固定的原截止时间，
经 `PublicationContinuationDeadlineBinder` 的 `BindPublicationContinuationUntil` 在同一资源 CAS 准入中
检查截止，不能仅在查找证据时检查。它不续延原生游标期限，也不缩短已经准入请求的独立有界期限。
不支持该端口的提供方不能声称兑现旧观察截止；查询不能退回最新 ROOT。

`PublicationReferenceMaintainer.ReconcilePublicationReferences` 单独对账已接受引用。调用方的
只读权威判定须证明该精确身份已不可逆地失去资格，才能返回 false；未知返回错误并保留。
提供方在权威 IO 之后用资源 CAS 重核原 ID、阶段、期限和证据，只释放指定接受引用，
不取消独立查询或续页租约。该步骤不代表物理删除，也不以经过的时间代替消费权威。

`ResourceRecoveryReport.Protected` 记录已知活动 ROOT 或有效声明的正常保护；`Retained` 表示归属、
读者覆盖或请求完成未知。`Deleted` 只记录已确认删除或同一已完成删除的幂等恢复，不能把正常保护
当成未知错误，也不能把未知资格记成已回收。


`ComputationMaintainer` 在显式运行目录内维护本实例的派生计算副本，独立于已打开查询引擎和当前模型。
宿主提供目录，提供方核验实际归属并持久续扫；固定查询视图须拒绝维护。返回报告中 `Protected` 包括
仍在缓存策略原期限内的副本或有效计算票据，`Retained` 表示未知；`Deleted` 只记已确认清空的物理分区。
计算缓存中的复制值退出不解除或延长查询、发布或证据引用，尚有效的语料必须拥有所需值。

`PublicationRepairer` 由维护调用方提供当前固定的目标元数据，区别“介质可读”和“解释/物理合同兼容”。
健康且兼容时不构建；不兼容或已确认资源丢失时准备新资源，复核准备结果满足目标后才进入实际资格与
映射 CAS。未知归属或读取暂不可用仍保留，不以重建掩盖未知。接受引用对账还须识别已明确换代的物理
映射；同一逻辑修订仍保留，不代表其旧物理票据仍被采用，独立查询与续页保护仍单独存续。
