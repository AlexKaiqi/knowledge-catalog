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
| W-01 | 一个 Writer 请求只有一个 Snapshot Repository target，代数只有 PUT/REMOVE | Workspace/dynamic runtime 成为 target；跨仓原子写；PATCH/APPEND | `TestUserJourneyCrossRepoWriteReportsPartialOutcome` `TestT3Atomicity` |
| W-02 | 写入保持 CAS 与命令幂等 | stale expected 成功；同 command ID 异 digest 覆盖 | `TestT2CommitCAS` `TestT4CommandIdempotency` |
| C-01 | 完整 Canonical 只从固定 authority basis 解释 | 公开返回 Candidate 或 OpenSearch `_source` | `TestSearchHydratesCandidatePageThroughKnowledgeBatchReader` `TestRelationsUsesExactRetrieverBeforePoisonAuthorityReadMany` |
| P-01 | Projection 可删除、可重建且不回滚 Canonical commit | 投影失败回滚 Snapshot；消费请求同步 build | `TestT8ProjectionLocateHydrateBasisLagAndRebuild` `TestConsumerPathsDoNotMaintainProjectionOrScanAuthority` |
| R-01 | SEARCH/RELATIONS 只从 exact-basis Retriever 取得候选页，再回读当前页 | authority relation/filter scan；先收集全量候选；错误 basis hydrate | `TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate` `TestRelationsPagesCandidatesAndRechecksFalsePositives` |
| R-02 | 无 READY exact-basis provider 时失败关闭 | 无索引扫描 authority；BUILDING 当作空结果 | `TestRelationsRequiresReadyProviderBeforeAuthority` `TestLocalProfileHasNoSearchProjection` |
| REL-01 | Relation 是独立 N 元对象；endpoint 是同仓结构化 KnowledgeRef | legacy string endpoint；Dolt endpoint 倒排成为查询口 | `TestRelationEnvelopeValidatedBeforeCommit` `TestProjectionCompilerEmitsRelationCore` |
| WS-01 | ResolvedWorkspace 只冻结 `{repository → commit}`，结果不复制、不覆盖 | Catalog DTO 出现 ObjectID/Aspect；按 scope 静默覆盖 | `TestT11FederatedReadDoesNotOverride` `TestOpenedWorkspacePinDoesNotMoveWithLaterCommit` |
| WS-02 | Workspace 配方不发权，pin 不锁未来权限 | 成为成员即获得 READ；旧 pin 绕过撤权 | `TestWorkspaceAuthorizationCoverageIsHonest` `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess` |
| D-01 | Binding declaration basis 与 observation basis 分开 | 动态值冒充 commit 内容；Stream 隐式数组化 | `TestStateBindingHydratesConsumerReadAndKeepsBothBases` `TestOrdinaryReadRejectsStreamBinding` |
| CA-01 | 底座不跨请求缓存语义对象 | Reader/Snapshot 持有 ObjectID→KnowledgeValue | `TestLowerLayersDoNotDeclareSemanticObjectCaches` `TestKnowledgeServiceBatchHydratesOneTreeWithoutCrossRequestObjectCache` |
| S-01 | Schema 只声明逻辑访问语义 | provider、mapping、stored、summary、key 进入 Schema | `TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens` |
| API-01 | CLI 与 HTTP 调同一应用 executor，但 transport 注册相互独立 | HTTP 调 CLI parser/dispatcher；两个入口实现不同业务规则 | `TestRelationRepositoryWorkspaceAndHTTPUseOneExactBasisExecutor` `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` |
| E-01 | 协议失败在 provider/surface 间保持稳定错误码 | 不可用被报告为不存在；basis 冲突被静默忽略 | `TestProtocolErrorJSON` `TestSearchRejectsCandidateMissingFromFixedAuthorityBasis` |
| O-01 | access/trace/hitmap 是证据或派生统计，不是知识与授权权威 | 访问次数写回知识；hitmap 改变权限或 Canonical | `TestAccessExtractionDoesNotInterpretKnowledgePayloadAsEvidence` `TestFileStoreTraceAndVersionedHitmap` |

## 3. 变更规则

1. 新增架构能力必须声明影响的 invariant ID；若没有影响，应明确说明。
2. 修改 invariant 必须同时修改设计解释、反例测试和本表证据。
3. 删除或重命名证据测试前，必须先为对应 invariant 提供替代证据。
4. 例外必须精确到 package/API、说明原因和删除条件；“暂时允许”不是例外合同。
5. `make test-boundary` 负责 structural；component/provider conformance 负责语义；CLI/HTTP E2E 只证明公开 surface，不替代前两类。
