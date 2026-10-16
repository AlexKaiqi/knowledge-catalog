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
