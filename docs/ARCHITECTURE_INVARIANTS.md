# 架构不变量

日期：2026-08-28

本文是核心架构约束的唯一验收索引。设计背景仍由 `KNOWLEDGE_CATALOG_DESIGN.md`、
`LAYERS.md`、`STORE_ADAPTERS.md` 等文档解释；一项实现是否合格，以本表列出的可证伪属性和
自动化证据为准。只有文字、没有反例测试的规则不算固化。

## 1. 验收模型

每条不变量必须同时说明：稳定决策、禁止观察、自动化证据。证据分为：

- structural：import、类型所有权、调用方向和数据形状；
- contract：所有 provider 运行同一行为合同；
- poison：不允许的调用一旦发生立即失败；
- metamorphic：只改变一个架构轴，协议观察保持等价；
- failure：缺能力、basis 冲突和派生不一致必须稳定失败。

## 2. P0 不变量与证据

| ID | 可证伪属性 | 禁止观察 | 主要自动化证据 |
|---|---|---|---|
| A-01 | 只改变 Repository provider 绑定，不改变 Reader、Writer、Catalog 和命令语义 | 非装配根/自身实现 import 具体 authority；provider 特有业务分支 | `TestConcreteAuthorityImportsAreConfined` `TestGenericAuthorityAssemblyHasNoProviderBranches` `TestAuthoritySelectionChangesOnlyRepositoryProfileField`；Dolt/Gitea RepositoryContract |
| L-01 | 每个生产包显式属于一层，只能沿 allowlist 依赖 | 新包自动落入 app；Catalog 获得 ObjectID；Snapshot 获得 Aspect | `TestEveryProductionPackageHasAnAllowedLayer`；focused transitive guards |
| I-01 | KnowledgeRef/ObjectID 不随物理路径变化 | 路径移动产生新对象身份 | `TestT1PathMove` |
| V-01 | 一次请求只使用开始时冻结的 Repository→Commit | hydrate 跟随更新后的 HEAD/latest；continuation 混入新 basis | `TestSearchAtNeverFollowsHeadAfterBasisIsFixed` `TestRelationsContinuationBindsQueryBasisAndGeneration` `TestOpenedWorkspacePinDoesNotMoveWithLaterCommit` |
| W-01 | 一个 Writer 请求只有一个 Snapshot Repository target，代数只有 PUT/REMOVE | Workspace/dynamic runtime 成为 target；跨仓原子写；PATCH/APPEND | `TestT3Atomicity` |
| W-02 | 写入保持 CAS 与命令幂等 | stale expected 成功；同 command ID 异 digest 覆盖 | `TestT2CommitCAS` `TestT4CommandIdempotency` |
| C-01 | Canonical 只从固定 authority basis 解释；hydrate 义务不因调用方信封是否含全文而取消 | 公开返回 Candidate 或 OpenSearch `_source`；用 stored fields 充当知识 | `TestSearchHydratesCandidatePageThroughKnowledgeBatchReader` `TestRelationsUsesExactRetrieverBeforePoisonAuthorityReadMany` |
| P-01 | Projection 可删除、可重建且不回滚 Canonical commit | 投影失败回滚 Snapshot；消费请求同步 build；一次性 Open 启动投影 worker | `TestT8ProjectionLocateHydrateBasisLagAndRebuild` `TestConsumerPathsDoNotMaintainProjectionOrScanAuthority` `TestProjectionWorkerStartsOnlyFromServeFacade` |
| R-01 | SEARCH/RELATIONS 只从 exact-basis Retriever 取得候选页，再回读当前页 | authority relation/filter scan；先收集全量候选；错误 basis hydrate | `TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate` `TestRelationsPagesCandidatesAndRechecksFalsePositives` |
| R-02 | 无 READY exact-basis provider 时失败关闭 | 无索引扫描 authority；BUILDING 当作空结果 | `TestRelationsRequiresReadyProviderBeforeAuthority` `TestLocalProfileHasNoSearchProjection` |
| REL-01 | Relation 是独立 N 元对象；endpoint 是同仓结构化 KnowledgeRef | legacy string endpoint；Dolt endpoint 倒排成为查询口 | `TestRelationEnvelopeValidatedBeforeCommit` `TestProjectionCompilerEmitsRelationCore` |
| WS-01 | ResolvedWorkspace 只冻结 `{repository → commit}`，结果不复制、不覆盖 | Catalog DTO 出现 ObjectID/Aspect；按 scope 静默覆盖 | `TestT11FederatedReadDoesNotOverride` `TestOpenedWorkspacePinDoesNotMoveWithLaterCommit` |
| WS-02 | Workspace 配方不发权，pin 不锁未来权限 | 成为成员即获得 READ；旧 pin 绕过撤权 | `TestWorkspaceAuthorizationCoverageIsHonest` `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess` |
| AUTH-01 | 命名知识集与 `--repo` SEARCH 以 pin/仓为候选；无 `knowledge.read` 时交付链屏蔽正文，不标 `partial`、不从 SearchView 抹仓 | 用缺少读权裁候选；把未授权 Canonical 原文交给调用方；因缺读权报 `partial` | `TestWorkspaceAuthorizationCoverageIsHonest` `TestRepoSearchDeliveryStripsUnauthorizedBody` `TestMetricPermissionScenes` `TestCatalogReadDiscoversWithoutKnowledgeRead` `TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead` |
| AUTH-02 | `workspace.consume` 不放行 `knowledge.*`；命名知识集 SEARCH 另要 `knowledge.search` | consume 隐含 `knowledge.read`/`knowledge.search`；catalog.read 跳过命名知识集 consume | `TestWorkspaceConsumeDoesNotImplyKnowledgeActions` `TestAuthorizeWorkspaceKnowledgeSeparatesConsumeFromSearch` |
| AUTH-03 | 交付链输入是已 hydrate 的知识 ID；按序改写可见正文；无 `knowledge.read` 只清空正文；不得改 ID/Address | 改 Candidate 身份后仍返回；后续 stage 看到未屏蔽正文；把交付写进 `retrieval/` / `index/` | `TestEmptyChainReturnsHydratedBody` `TestRepositoryReadStripsUnauthorizedBodyAndKeepsID` `TestChainRejectsIdentityMutation` `TestChainRunsLaterStagesOnStrippedEnvelope` `TestLaterStageMayRewriteVisibleBody` `TestFromValueRoundTripWritesOnlyBody` |
| D-01 | Binding declaration basis 与 observation basis 分开 | 动态值冒充 commit 内容；Stream 隐式数组化 | `TestStateBindingHydratesConsumerReadAndKeepsBothBases` `TestOrdinaryReadRejectsStreamBinding` |
| CA-01 | 底座不跨请求缓存语义对象 | Reader/Snapshot 持有 ObjectID→KnowledgeValue | `TestLowerLayersDoNotDeclareSemanticObjectCaches` `TestKnowledgeServiceBatchHydratesOneTreeWithoutCrossRequestObjectCache` |
| S-01 | Schema 只声明逻辑访问语义 | provider、mapping、stored、summary、key 进入 Schema | `TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens` |
| API-01 | CLI 与 HTTP 调同一应用 executor，但 transport 注册相互独立 | HTTP 调 CLI parser/dispatcher；两个入口实现不同业务规则 | `TestRelationRepositoryWorkspaceAndHTTPUseOneExactBasisExecutor` `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` |
| E-01 | 协议失败在 provider/surface 间保持稳定错误码 | 不可用被报告为不存在；basis 冲突被静默忽略 | `TestProtocolErrorJSON` `TestSearchRejectsCandidateMissingFromFixedAuthorityBasis` |
| O-01 | access/trace/hitmap 是证据或派生统计，不是知识与授权权威 | 访问次数写回知识；hitmap 改变权限或 Canonical | `TestAccessExtractionDoesNotInterpretKnowledgePayloadAsEvidence` `TestFileStoreTraceAndVersionedHitmap` `TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation` |
| IX-01 | SEARCH/RELATIONS/history/audit 请求都有服务端硬页上限，零值只表示默认页 | limit=0 返回全仓；调用方用超大 limit 绕过分页 | `TestSearchLimitIsBoundedAndDefaulted` `TestRelationsRejectsInvalidLimit` `TestPageLimitTreatsZeroAsDefaultAndRejectsOversized` `TestCatalogRepoReadFlow` `TestCatalogAuditIsGitLog` `TestAgentDelegatedAccessTraceFeedbackAndHitmap` `TestKnowledgeResolveAndObjectLogOverHTTP` `TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent` `TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation` `TestHTTPAccessLogQueryFiltersAndPages` `TestKnowledgeObjectRequestOmitsZeroLimit` `TestPagedKnowledgeRequestsOmitZeroLimit` |
| IX-02 | Projection 物理拓扑属于 physicalDigest，过亿档不得落回单主分片隐式默认 | shard/replica/refresh 改变但旧投影仍被判定兼容 | `TestOpenSearchProjectionScaleSettingsAffectPhysicalDigest` |
| IX-03 | 暖 generation rebuild 在原子 Publish 前持续服务旧 READY generation | rebuild 全程持写锁；把 READY 控制面改成 BUILDING | `TestOpenSearchWarmRebuildKeepsReadyGenerationQueryable` |
| IX-04 | 稳态增量成本随变更批次而不是总索引量增长 | 每次 Apply 执行全索引 `_count` 或强制 `_refresh` | `TestOpenSearchIncrementalApplyAvoidsGlobalCountAndForcedRefresh` |
| IX-05 | ad-hoc 历史 pin 的 Engine 生命周期绑定单次读取 | 每个历史 commit 永久进入进程缓存 | `TestHistoricalReadMissEngineIsReleased` |

## 3. 变更规则

1. 新增架构能力必须声明影响的 invariant ID；若没有影响，应明确说明。
2. 修改 invariant 必须同时修改设计解释、反例测试和本表证据。
3. 删除或重命名证据测试前，必须先为对应 invariant 提供替代证据。
4. 例外必须精确到 package/API、说明原因和删除条件；“暂时允许”不是例外合同。
5. `make test-boundary` 负责 structural；component/provider conformance 负责语义；CLI/HTTP E2E 只证明公开 surface，不替代前两类。

## 4. 已选定、尚未进入本表

调用方信封是否含全文由交付链首段决定，不改写 `C-01`。命名知识集与 `--repo` SEARCH 的搜宽读严已由 `AUTH-01` / `AUTH-02` 固化；链的独立层与身份冻结已由 `AUTH-03` 固化。

下列由 [`PERMISSIONS.md`](PERMISSIONS.md) 选定，参考实现尚未提供对应表面，因此不是固化不变量：

- Catalog 范围 SEARCH 语法糖（`kc knowledge search --catalog` / `discoveryWorkspaceId`）：准入是该 Catalog 的 `catalog.read`，不另要 discovery Workspace 的 `workspace.consume`，也不用按仓 `knowledge.search` 裁候选。
