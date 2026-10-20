# 架构不变量

本文是核心架构约束的验收索引：把设计 owner 的决策映射为可证伪属性、禁止观察和验证入口。
[架构总览](../architecture.md)、[对象读写](../design/object.md) 等 owner 决定应然边界；
本表与公开合同必须符合设计，不能用当前测试反向缩小设计。只有文字、没有反例测试的规则不算固化。

表内 Test 名表示声明的验证入口，不表示本次已通过。方法、补例规范和同次运行证据由
`test-catalog.md` §0.2 拥有；`make validation-inventory` 可定位具名 Test 的文件/行号。
实际结论要结合对应 run 的源码指纹、scope、成功/失败/跳过事件及原始产物。

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
| A-01 | 只改变 Repository provider 绑定，不改变 Reader、Writer、Dataset 登记服务 和命令语义 | 非装配根/自身实现 import 具体 authority；provider 特有业务分支 | `TestConcreteAuthorityImportsAreConfined` `TestGenericAuthorityAssemblyHasNoProviderBranches` `TestAuthoritySelectionChangesOnlyRepositoryProfileField` `TestArchitectureGuardIncludesProductionScripts` `TestFixtureDeploymentUsesOnlyDeclaredHomeSeams`；Gitea/LakeFS RepositoryContract；`TestLakeFSMatchesTreeProviderByOperationStep` |
| L-01 | 每个生产包显式属于一层，只能沿 allowlist 依赖 | 新包自动落入 app；Dataset 登记服务 获得 ObjectID；Snapshot 获得 Aspect | `TestEveryProductionPackageHasAnAllowedLayer` `TestDatasetSpaceHasAnExplicitDiscoveryLayerAndNoRuntimeDependencies`；focused transitive guards |
| I-01 | ObjectRef/ObjectID 不随物理路径变化 | 路径移动产生新对象身份 | `TestT1PathMove` |
| V-01 | 一次请求只使用开始时冻结的 Repository→Commit | hydrate 跟随更新后的 HEAD/latest；continuation 混入新 basis | `TestSearchAtNeverFollowsHeadAfterBasisIsFixed` `TestRelationsContinuationBindsQueryBasisAndGeneration` `TestOpenedDatasetPinDoesNotMoveWithLaterCommit` `TestReadManyLoadsOnlyRequestedObjectLocatorsAndUnits` `TestSingleObjectReadDecodeBytesAreIndependentOfRepositorySize` |
| W-01 | 一个 Writer 请求只有一个 Snapshot Repository target，代数只有 PUT/REMOVE | Workspace/dynamic runtime 成为 target；跨仓原子写；PATCH/APPEND | `TestT3Atomicity` |
| W-02 | 写入保持 CAS 与命令幂等 | stale expected 成功；同 command ID 异 digest 覆盖 | `TestT2CommitCAS` `TestT4CommandIdempotency` `TestCommandLogRecoversReservationBeforeCommit` `TestCommandLogRecoversCommitBeforeReceipt` `TestCommandLogReadFailureCannotReapplyCommand` `TestCommandLogRetentionIsBoundedAndKeepsPending` |
| C-01 | Canonical 只从固定 authority basis 解释；hydrate 义务不因调用方信封是否含全文而取消 | 公开返回 Candidate 或 OpenSearch `_source`；用 stored fields 充当对象 | `TestSearchHydratesCandidatePageThroughObjectBatchReader` `TestRelationsUsesExactRetrieverBeforePoisonAuthorityReadMany` |
| P-01 | Projection 可删除、可重建且不回滚 Canonical commit | 投影失败回滚 Snapshot；消费请求同步 build；一次性 Open 启动投影 worker | `TestT8ProjectionLocateHydrateBasisLagAndRebuild` `TestConsumerPathsDoNotMaintainProjectionOrScanAuthority` `TestProjectionWorkerStartsOnlyFromServeFacade` |
| R-01 | SEARCH/RELATIONS 只从 exact-basis Retriever 取得候选页，再回读当前页 | authority relation/filter scan；先收集全量候选；错误 basis hydrate | `TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate` `TestRelationsPagesCandidatesAndRechecksFalsePositives` |
| R-02 | 无 READY exact-basis provider 时失败关闭 | 无索引扫描 authority；BUILDING 当作空结果 | `TestRelationsRequiresReadyProviderBeforeAuthority` `TestLocalProfileHasNoSearchProjection` |
| REL-01 | Relation 是独立 N 元对象；endpoint 是可跨仓的结构化 ObjectRef，写入只影响关系存储仓 | 引用触发端点仓写入或隐式授权；只查端点仓而漏掉 Dataset 中的关系仓；endpoint 倒排表成为查询口 | `TestRelationEnvelopeValidatedBeforeCommit` `TestCrossRepositoryRelationStillRejectsMalformedEnvelope` `TestDatasetRelationsQueryAllMembersAndBindContinuationToScope` `TestProjectionCompilerEmitsRelationCore` |
| KS-01 | ResolvedDataset 冻结某 Dataset 版本的文件清单（从而导出 `{repository → commit}`），结果不复制、不覆盖 |Dataset 登记服务 DTO 出现 ObjectID/Aspect；按 scope 静默覆盖；已发布版本跟随 live 分支 | `TestT11FederatedReadDoesNotOverride` `TestOpenedDatasetPinDoesNotMoveWithLaterCommit` `TestOpenDatasetDoesNotFollowLaterCommitUntilRepublish` `TestDefineDatasetFailsClosedWithoutAttachedSnapshot` `TestDatasetPathAllowedEmptyItemsDeny` `TestEmptyDatasetItemsDoNotAdmitWholeRepository` |
| KS-02 |Dataset 配方不发仓权，pin 不锁未来权限；`file.read`×Dataset 与仓 `object.read` 互不蕴含 | 成为成员即获得仓级 READ；旧 pin 绕过撤权 | `TestDatasetAuthorizationCoverageIsHonest` `TestUserJourneyObjectGrantDoesNotAuthorizeAccess` `TestDatasetFileReadDoesNotImplyRepoObject` |
| AUTH-01 | 搜索只有命名 Dataset 的发布版本；消费授权与选入文件范围先于候选查询，声明也只在选入 Dataset 后生效 | 按仓/隐式全局/临时定义搜索；无消费权时访问查询后端；搜出清单外对象或使用未选入声明 | `TestDatasetSearchAuthorizesBeforeResolution` `TestDatasetSearchRejectsMissingConsumptionBeforeQuery` `TestDatasetDiscoveryHTTPRejectsRetiredScopesBeforeRuntime` `TestDiscoveryCLIRejectsImplicitAndTemporarySearchScopesBeforeTransport` `TestMetricPermissionScenes` `TestDatasetSearchExcludesPathsOutsidePublishedList` |
| AUTH-02 | Dataset`file.read` 足以搜索及解释清单内文件；不放行仓 `object.*`；库存或仓读权不能替代 Dataset 消费权，撤权后续页仍拒绝 | Dataset 消费权隐含来源整仓读写；inventory.read 或仓读权跳过 Dataset 消费；续页令牌冻结授权 | `TestDatasetAuthorizationCoverageIsHonest` `TestDatasetFileReadDoesNotImplyRepoObject` `TestDatasetSearchRejectsMissingConsumptionBeforeQuery` `TestDatasetDeliveryRejectsReadsAfterGrantRevocation` `TestHTTPWorkspaceSearchKeepsDatasetFileReadBody` |
| AUTH-03 | 交付链输入是已 hydrate 的对象 ID；按序改写可见正文；无 `object.read` 只清空正文；不得改 ID/Address | 改 Candidate 身份后仍返回；后续 stage 看到未屏蔽正文；把交付写进 `discovery/` / `discovery/projection/` | `TestEmptyChainReturnsHydratedBody` `TestRepositoryReadStripsUnauthorizedBodyAndKeepsID` `TestChainRejectsIdentityMutation` `TestChainRunsLaterStagesOnStrippedEnvelope` `TestLaterStageMayRewriteVisibleBody` `TestFromValueRoundTripWritesOnlyBody` |
| AUTHN-01 | 认证可替换：具体认证器只在 `identity/` 子包，`server/`、`snapshot/service/` 只经注册表与 `identity.Authenticate` 消费；存储访问只用服务凭证 | `identity/` 外实现认证器或自写测试替身；按认证方式名字分支；装配根以外 import 具体认证器；调用方证明转发给存储 | `TestAuthenticatorsAreConfinedToIdentity` `TestAuthenticatorGuardRejectsLeaks` `TestAuthenticatorDoublesComeFromIdentitytest` `TestRegisteredAuthenticatorReplacesProviderWithoutServerChange` `TestVerifiedCallerProofIsNeverForwardedToStorage`；`TestForbiddenDependencies/identity/builtin` |
| D-01 | Binding declaration basis 与 observation basis 分开 | 动态值冒充 commit 内容；Stream 隐式数组化 | `TestStateBindingHydratesConsumerReadAndKeepsBothBases` `TestOrdinaryReadRejectsStreamBinding` `TestProjectionCompilerSkipsUnobservedBindingContent` `TestDescriptorInUnselectedRepositoryResolvesAtFrozenDependency` |
| CA-01 | 底层 Snapshot、Dataset 组合与 Reader 不拥有跨请求对象缓存的具体状态；上层缓存经 Object hydrate 端口装配 | Reader/Snapshot 持有 ObjectID→ObjectValue 或 import 具体缓存；未注入端口时暗中缓存完整对象 | `TestLowerLayersDoNotDeclareSemanticObjectCaches` `TestObjectsBatchHydratesOneTreeWithoutCrossRequestObjectCache` |
| CA-02 | 缓存只复用固定 Repository、commit 与完整对象或 Address 的 Snapshot 读取，保持完整元数据及副本隔离 | 同 ID 跨仓/跨版本命中；Address 坐标丢失；调用方修改污染缓存；错误 basis 成为成功条目 | `TestCacheUsesRepositoryVersionAndBatchedMisses` `TestCacheClonesAllMutableSnapshotData` `TestCacheAddressKeysRetainEveryCoordinate` `TestCacheFailsOnSourceErrorsAndWrongBasis` `TestSearchReusesHydrationCacheAcrossQueriesAndRejectsWrongBasis` `TestReaderAndWorkspaceUseInjectedHydrationAtFixedBasis` |
| CA-03 | 正文缓存、预热与单次回源均有界；miss 子集批量回源，旧 pin 不随预热推进 | 缓存无限增长；冷热预热全仓物化；命中仍回源；新 commit 覆盖旧版本内容；消费 miss 启动维护扫描 | `TestCacheLRUEvictionByteBudgetAndLifecycle` `TestColdWarmIsOptInAndOnlyOneBoundedPage` `TestWarmRefreshesHotObjectsWithoutDiscardingOldPin` `TestCacheConcurrentReadsAndWarmAreIsolated` `TestRelationsReuseSameBasisHydrationCache` `TestConsumerReadSchemaSearchAndRelationsNeverCallMaintenanceScanner` |
| PC-01 | 注册的 Snapshot 消费者独立跟踪目标与恢复依据，从 published HEAD 对账并恢复可丢状态；静态 Snapshot 与动态 observation lane 不混 basis | 某消费者失败/阻塞阻断其它 worker；共用应用进度；旧任务完成覆盖新目标；耐久账 READY 阻止介质恢复；动态 notice 移动 HEAD 或静态 basis | `TestSnapshotConsumersHaveIndependentDurableProgressAndRetry` `TestSnapshotConsumerWorkersRecoverLostNotificationsWithoutBlockingEachOther` `TestSnapshotConsumerLateCompletionCannotOverwriteNewDesiredCommit` `TestSnapshotConsumerRevisionsDoNotShareProgress` `TestSnapshotConsumerRegistrationIsUniqueAndFrozenAtStart` `TestIngestionControllerKeepsStaticAspectAndDynamicRecipeLanesSeparate` |
| S-01 | Schema 只描述结构：取值声明只在单元头 `valueSource` 引用取值描述，检索能力只在采用索引声明 Schema 的 `index` 方面成员里声明；Schema 与系统发布的 Schema 不带 `origin`/`binding`/`access` | Schema 级 `origin`/`binding`；先查 Schema 再回落实例的解析分支；Schema 字段上的 `access`；按字段类型推默认检索声明 | `TestValueDeclarationResolvesOnlyFromUnitValueSource` `TestValueDeclarationGuardRejectsSchemaFallbacks` `TestPublishedSchemasDeclareNoSchemaLevelOrigin` `TestWriterRejectsSchemaOrigin` `TestSchemaFieldAccessIsRejected` `TestDescribeSchemaListsStructureWithoutAccess` `TestIndexDeclarationMembersAreTheOnlyRetrievalSource` `TestIndexDeclarationCompilerIgnoresOrdinaryIndexNamedObjects` |
| API-01 | CLI 与 HTTP 调同一应用 executor，但 transport 注册相互独立 | HTTP 调 CLI parser/dispatcher；两个入口实现不同业务规则 | `TestRelationDatasetCLIAndHTTPUseOneExactBasisExecutor` `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` `TestCLIAndHTTPUseSameTypedApplicationExecutor` `TestApplicationCoreHasNoTransportOrProviderImports` `TestApplicationRequestsUseOwnedIdentifierTypes` |
| E-01 | 协议失败在 provider/surface 间保持稳定错误码 | 不可用被报告为不存在；basis 冲突被静默忽略 | `TestProtocolErrorJSON` `TestSearchRejectsCandidateMissingFromFixedAuthorityBasis` `TestBaseAuthorityWithoutObjectCapabilitiesFailsClosed` `TestMissingProviderCapabilitiesFailWithoutPanic` `TestProviderNeutralUnitAlgebraHasNoFileStorageShape` |
| O-01 | access/trace/hitmap 是证据或派生统计，不是对象与授权权威 | 访问次数写回对象；hitmap 改变权限或 Canonical | `TestAccessExtractionDoesNotInterpretObjectPayloadAsEvidence` `TestFileStoreTraceAndVersionedHitmap` `TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation` |
| IX-01 | SEARCH/RELATIONS/history/audit 请求都有服务端硬页上限，零值只表示默认页 | limit=0 返回全仓；调用方用超大 limit 绕过分页 | `TestSearchLimitIsBoundedAndDefaulted` `TestRelationsRejectsInvalidLimit` `TestPageLimitTreatsZeroAsDefaultAndRejectsOversized` `TestCatalogRepoReadFlow` `TestCatalogAuditReadsSnapshotHistory` `TestAgentDelegatedAccessTraceFeedbackAndHitmap` `TestObjectResolveAndObjectLogOverHTTP` `TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent` `TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation` `TestHTTPAccessLogQueryFiltersAndPages` `TestObjectRequestOmitsZeroLimit` `TestPagedObjectRequestsOmitZeroLimit` |
| IX-02 | Projection 物理拓扑属于 physicalDigest，过亿档不得落回单主分片隐式默认 | shard/replica/refresh 改变但旧投影仍被判定兼容 | `TestOpenSearchProjectionScaleSettingsAffectPhysicalDigest` |
| IX-03 | 暖 generation rebuild 在原子 Publish 前持续服务旧 READY generation | rebuild 全程持写锁；把 READY 控制面改成 BUILDING | `TestOpenSearchWarmRebuildKeepsReadyGenerationQueryable` |
| IX-04 | 稳态增量成本随变更批次而不是总索引量增长 | 每次 Apply 执行全索引 `_count` 或强制 `_refresh` | `TestOpenSearchIncrementalApplyAvoidsGlobalCountAndForcedRefresh` |
| IX-05 | ad-hoc 历史 pin 的 Engine 生命周期绑定单次读取 | 每个历史 commit 永久进入进程缓存 | `TestHistoricalReadMissEngineIsReleased` |
| PL-01 | 插件实例只由部署配置安装；一个能力清单名、一个 LiveAccess runtime 只对应一个实例；共享实例按来源分别维护 | Repo metadata、Dataset 或场景夹具选择实例；清单名或 runtime 歧义时猜一个实例启动；未装插件时取值不报缺能力或固定读取受阻；追平一个来源顺带执行其它来源维护 | `TestRepositoryPluginSelectionIsRetiredFromCodeAndScenes` `TestRepositoryPluginSelectionGuardRejectsReintroduction` `TestDeploymentRefusesRetiredPerRepositoryPluginSelection` `TestDeploymentRefusesAmbiguousOrUnknownPluginInstances` `TestDeploymentRefusesOverlappingAccessRuntimes` `TestDeploymentWithoutPluginInstancesExplainsMissingCapability` `TestIndexPluginsUncompiledEntryInstallsNoCapabilityAndKeepsFixedReads` `TestIndexPluginsSyncOneSourceDoesNotRunAnotherSourcesMaintenance` |

### 2.1 交叉索引

编号分三套，**不另造第四套**。本表负责验证映射；遇到冲突先回到 `doc/` 下的设计篇
核对并修复映射，不以现有代码、测试通过或产品验收编号选择更宽松的解释。

`K-01`…`K-28` 是设计推导层的语义结论编号，定义见本文 §2.2 的 K 表（原系统设计 §9.3，随替换并入本文），不是 `test-catalog.md` 旅程用例号。`ADR-*` 列保留原系统设计决策记录的历史编号：对应决策内容已由 [架构总览](../architecture.md)、[对象读写](../design/object.md) 等篇的选定/否决正文承接，原文只存在于 git 历史，不再另指现行文档。空格表示没有单条 ADR 对应该不变量。未进交叉表的 `K-*` 仍只在 §2.2 的 K 表，不要为它们补造架构 ID。交叉表只导航，不是第二份证据登记。

| 架构 | 设计 | ADR | MVP |
|---|---|---|---|
| `A-01` | `K-23` | ADR-018 | P1 |
| `L-01` | — | ADR-001 | S5 |
| `I-01` | `K-04` | ADR-007 | P2 |
| `V-01` | `K-04`, `K-10`, `K-11` | ADR-008 | C2 |
| `W-01` | `K-01`, `K-21`, `K-22` | ADR-002, ADR-004, ADR-010 | P4 |
| `W-02` | `K-06`, `K-18` | ADR-003 | P3 |
| `C-01` | `K-05`, `K-12`, `K-25` | ADR-013 | C4, C6 |
| `P-01` | `K-19` | ADR-013 | S4 |
| `R-01` | `K-19`, `K-25` | ADR-013 | C4 |
| `R-02` | `K-27` | ADR-013 | C5 |
| `REL-01` | — | ADR-014 | — |
| `KS-01` | `K-03`, `K-10`, `K-12`, `K-13` | ADR-008, ADR-009 | C3 |
| `KS-02` | `K-20` | ADR-010 | C7 |
| `AUTH-01` | `K-25` | — | C4 |
| `AUTH-02` | — | — | C7 |
| `AUTH-03` | `K-25` | — | C4 |
| `D-01` | `K-17`, `K-28` | ADR-017, ADR-022 | — |
| `CA-01` | — | — | — |
| `CA-02` / `CA-03` | — | — | — |
| `PC-01` | — | — | — |
| `S-01` | `K-26` | ADR-023 | P5 |
| `API-01` | — | — | S1 |
| `E-01` | `K-27` | — | C5 |
| `O-01` | — | — | — |
| `IX-01` … `IX-05` | — | — | — |
| `PL-01` | — | — | — |

| 设计 | 架构 | 备注 |
|---|---|---|
| `K-01` | `W-01` | |
| `K-02` | `A-01` | 仓独立身份；本表只固化 adapter 替换不改语义 |
| `K-03` | `KS-01` | |
| `K-04` | `I-01`, `V-01` | |
| `K-05` | `C-01`, `W-02` | |
| `K-06` | `W-02` | |
| `K-07` | — | 合并语义；未单列。证据在[治理](../design/governance.md)与 `cli/hook_merge_test.go` |
| `K-08` | — | 同上 |
| `K-09` | — | 同上 |
| `K-10` | `KS-01`, `V-01` | |
| `K-11` | `V-01` | |
| `K-12` | `KS-01`, `C-01` | |
| `K-13` | `KS-01` | |
| `K-14` | — | fork/vendor 升级；未单列 |
| `K-15` | — | 同上 |
| `K-16` | — | 同上 |
| `K-17` | `D-01`, `W-01` | |
| `K-18` | `W-02` | P3 |
| `K-19` | `P-01`, `R-01`, `R-02` | C4, C5 |
| `K-20` | `KS-02` | C7 |
| `K-21` | `W-01` | |
| `K-22` | `W-01` | P4 |
| `K-23` | `A-01` | P1, P2 |
| `K-24` | — | ADR-020；未单列 |
| `K-25` | `C-01`, `AUTH-01`, `AUTH-03` | C4 |
| `K-26` | `S-01` | P5 |
| `K-27` | `R-02`, `E-01` | C5 |
| `K-28` | `D-01` | |

### 2.2 设计推导编号 K-01..K-28

K 表保留设计推导层的语义结论（原系统设计 §9.3，替换后并入本文）；规范性的可证伪属性、
禁止观察和自动化证据以上方 P0 表为准。两处冲突时必须先修复冲突，不能选择对当前实现
更宽松的一份解释。`K-*` 不是 `test-catalog.md` 的旅程用例号。

| # | 不变量 |
|---|---|
| `K-01` | 每个 Writer 命令只有一个 Snapshot target；Workspace 和动态运行值都不是 target |
| `K-02` | 每个 Repository 有独立身份、ACL、Version 图、Ref 和生命周期 |
| `K-03` | public/group/personal 是治理 Scope，不是目录优先级 |
| `K-04` | ObjectRef 不依赖路径；PinnedObjectRef 固定 Version |
| `K-05` | Version 内 Canonical 与已接受 Ref 不可原地修改 |
| `K-06` | Ref/对象更新必须带前置条件，禁止静默 LWW |
| `K-07` | 工作分支上的写入不改变默认分支，也不通知目录；只有合并推进默认分支 |
| `K-08` | 合并绑定精确源 commit 与目标期望 commit；源分支移动或目标分叉都拒绝 |
| `K-09` | 合并前在源 commit 上校验 Schema 与 locator；失败不推进目标 |
| `K-10` | ResolvedDataset 是 Repository→Commit Map；命令内不可变 |
| `K-11` | 跨命令可跟已发布 selector；命令内不得跟随 latest |
| `K-12` | 联合结果保留 Repository、Version、Object、Scope 和 Provenance |
| `K-13` | 多来源并存，不按 Scope 静默覆盖 |
| `K-14` | 普通对象引用升级不修改引用方 Repository，也不跨 Repository merge |
| `K-15` | Fork 创建新 ObjectRef；只有 Fork sync 做三方比较 |
| `K-16` | Vendor 保留精确来源 pin；本地编辑必须转 Fork |
| `K-17` | 动态 State/Stream 不因可访问而成为 Canonical；沉淀必须显式 COMMIT |
| `K-18` | 同幂等键同 digest 返回原 Receipt；异 digest 冲突 |
| `K-19` | Projection 非 Canonical，必须声明 basis、coverage 和 lag |
| `K-20` | Workspace pin 锁数据不锁未来权限；授权按请求求值 |
| `K-21` | 内容写入、建分支与合并都经 Writer，不直写 Backend/Ref |
| `K-22` | 不构造跨 Repository 的虚假单一事务 |
| `K-23` | Adapter 迁移不得改变身份、版本和读写协议语义 |
| `K-24` | Repository 领域生命周期终点是 ARCHIVE；物理删除由保留/合规流程处理 |
| `K-25` | Candidate 不作为对象结果；SEARCH 命中必须在计划固定的 SearchView/basis 上 hydrate Canonical，不得用 Candidate 或 stored fields 充当对象。调用方信封是否含全文由权限体系交付链首段决定，不取消本条 |
| `K-26` | 索引声明不包含插件实例、引擎地址、原始 mapping、脚本、分片或对象身份的替代定义；这些属于部署配置或插件实现（反例 `TestIndexDeclarationWriterRejectsInvalidInputsAndShapeAtomically`） |
| `K-27` | 不能证明无漏项的 provider/plan 必须返回 partial；不得用 score、缓存命中或 invalidation 推断完整性 |
| `K-28` | 外部单元（binding 单元）的消费结果必须同时标识声明（单元与取值描述的组合摘要，取值描述按冻结版本解释：选入仓用服务版的提交，未选入的持有仓用发布时冻结的依赖版本）与逐单元 observation basis；依赖版本不进入可见范围，持有仓前进不改变解释；VFS/commit 不得冒充冻结动态值，Stream 不得隐式数组化 |

## 3. 变更规则

1. 新增架构能力必须声明影响的 invariant ID；若没有影响，应明确说明。
2. 修改 invariant 必须同时修改设计解释、反例测试和本表证据。
3. 删除或重命名证据测试前，必须先为对应 invariant 提供替代证据。
4. 例外必须精确到 package/API、说明原因和删除条件；“暂时允许”不是例外合同。
5. `make test-boundary` 负责 structural；component/provider conformance 负责语义；CLI/HTTP E2E 只证明公开 surface，不替代前两类。

## 4. 已选定、尚未进入本表

调用方信封是否含全文由交付链首段决定，不改写 `C-01`。Dataset 搜索按整体消费权与固定交付范围准入，范围内的结果不逐对象裁剪；仓直接读取仍按仓权限判断。链的独立层与身份冻结由 `AUTH-03` 固化。

搜索范围按[发现](../design/discovery.md)选定的“只按 Dataset”实现：索引声明选进 Dataset 才生效，插件只作
解释器、不按仓或 Dataset 绑定。`AUTH-01` / `AUTH-02` 已随旧入口退役改写；原授权与范围风险由 Dataset 消费
拒绝、未选入对象与声明、正式 CLI/HTTP 旧范围反例接住，契约是否通过看本次运行证据。

交付链首段之后的隐私化 / 脱敏 **未选定**（[权限体系](../design/permissions.md) Non-Goal）：不是本表不变量，不是 MVP 待做项，禁止实现，也不得写成当前入口或已挂链段。

插件声明移出 Schema 已在[插件](../design/plugins.md)、[发现](../design/discovery.md#索引声明)与
[LiveAccess](../design/liveaccess.md#声明)选定：索引声明是仓里的 `index` 方面，外部取值只在方面或成员上
引用取值描述，观察依据按地址挂在值旁边。`K-28` 已随 binding 单元与跨仓依赖版本改写。`S-01` 与 `K-26` 已随
索引声明迁入 `index` 方面改写：Schema 不再带字段 `access`，Writer 拒绝旧写法；检索能力只来自索引声明成员，
声明不含插件实例、物理载荷或身份替代定义，由结构守卫与 Writer 反例证明。
