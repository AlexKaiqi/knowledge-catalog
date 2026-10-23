# CLOSEOUT-01 · 对象版本与读写收口

## 目标

五条使用路径在同一冻结候选上一起通过：受影响回归、`make check-docs`、`make quality`、`make test`、
`make test-contracts` 全绿且不新增 skip；真实部署验收与索引物理回收的联合场景各自完成。

本任务只做收口，不补实现。前五条中任一条的剩余实现仍留在原任务。

## 依据

设计篇：[对象版本与读写](../../doc/design/object-versioning.md)，特别是
[接口与实施边界](../../doc/design/object-versioning.md#接口与实施边界)的五行使用路径表。
[工程质量闭环](../../doc/engineering/quality.md)的完成判定。

## 收口口径

三条判定必须分开写，不能互相顶替：

1. 实现的实现范围：代码与公开入口是否接通。
2. 已验证范围：哪些用例在哪个源码指纹上跑绿。
3. 验收结论：冻结候选上是否全绿且无 skip。

前五条各自已给出 1 和 2；本任务只产出 3。批次证据不能当冻结候选验收，`sourceChanged=true` 的运行
不能当冻结候选验收，`passed-with-skips` 不能当无 skip 验收。

## 验收

| 项 | 判定 |
|---|---|
| 冻结候选 | 源码指纹固定后不再修改；每个证据目录的 `sourceChanged=false` |
| 完整混合契约 | `make test-contracts` 全绿，无失败、无新增 skip；既有 live 环境用例（`KC_LIVE_*`，需真实太湖认证、Luna、lakeFS 吞吐或 OpenSearch 容器）单独列出，不计入本任务缺口，但不隐藏 |
| 默认场景 | `make test` 全绿，无失败、无 skip |
| 质量门禁 | `make check-docs`、`make quality` 通过；耦合基线未调整 |
| 真实部署 | `make deploy-local-scenes` 在对象版本专项场景集上通过；不以求和场景冒充专项 |
| 索引联合 | 与 `INDEX-PHYSICAL-01` 的联合验收表逐场景执行：物理回收后读取依据与观察证据仍按各自合同可用 |
| 五条路径 | 读取取得依据并回读、两版比较、条件写后无变化回执、目录零差异发布、历史首页与续页，各自在冻结候选上可执行 |

## 前置

- `READ-BASIS-01`、`COMPARE-01`、`NO-CHANGE-01`、`ZERO-DIFF-01`、`HISTORY-01` 的实现各自稳定。
- `INDEX-PHYSICAL-01` 的物理回收可执行。
- 真实部署环境可用（本地测试栈已起，lakeFS 与 OpenSearch 就绪）。

## 本轮范围前提

按用户 2026-10-22 的推进前提，假定一个对象的各个方面在同一仓：同一 `object_id` 跨仓组合成一个对象的
场景不进本轮验收，读取依据的版本向量按单来源即可满足。这不改变读取依据本身、Dataset 多来源固定、
取值描述等跨仓解释依赖，也不改变已实现的组合行为与其回归。

## 不做

跨仓事务、跨命令合批、按状态摘要反查内容、Dataset 发布模型与生命周期调整。Dataset 的保留、来源清理
和自动跟随仍由 `DATASET-RETENTION-01`、`DATASET-STORAGE-01`、`DATASET-REVISION-01` 承担。

不新增条件写语义、历史加速与 head 缓存；它们的成本目标与停止条件留待真正采纳时另立任务。
不做性能测量、压测与成本资格结论。

## 交付

冻结候选：以 `16bb741fdc943f63b204a76e73baa2168d21a4e3` 为基线，只叠加本批改动（会话列出的场景与
DSL 文件、两条对象读取依据测试）。源码指纹
`c967676f46a0a8c53b3dd3ceff862af8c7628a36f8679ce800899db629c70a0c`，共 3113 个受管文件。
三个入口共用该指纹，各自 `sourceChanged=false`。

实现的实现范围（判定 1）：五条路径的对象层能力与公开入口已接通——取得依据并回读、两版对象服务端比较、
条件写后无变化回执、目录零差异发布、历史首页与续页；场景 DSL 增加 `I remember <path> as <name>`，用于
把上一命令输出里的坐标带进后续命令。证据见各子任务交付节。

已验证范围（判定 2）：

| 入口 | 证据目录 | 结果 | 事件（pass/fail/skip） |
|---|---|---|---|
| 默认场景 | `.validation/runs/20261022T154324Z-93b18dd4deb1` | passed | 18 / 0 / 0 |
| 完整混合契约 | `.validation/runs/20261022T154436Z-5d9104e40a73` | passed-with-skips | 5682 / 0 / 18 |
| 真实部署专项 | `.validation/runs/20261022T155925Z-48b3b9703db5` | passed | 2 / 0 / 0 |

`make check-docs`（21 篇）、`make quality`（units=86，propagation 13.69%，`delta +0.00pp`，ratchet held）、
`make check-surface` 均通过，耦合基线未调整。真实部署专项只跑 `TestLiveLakeFSSceneDFS/^object-published$`，
不以求和场景冒充专项。

混合契约的 18 条 skip 全部是既有 live 环境用例，逐条为：`TestLiveCapturedSourcesAreReadFromRealDocuments`、
`TestLiveCapturedSourcesRejectsMissingIdentity`、`TestLiveSnapshotCheckpointOverwritesPollutedTarget`、
`TestLiveSnapshotCheckpoint`、`TestLiveLunaListwiseRerank`、`TestLiveHTTPStateRuntimeContainer`、
`TestLiveHTTPRuntimeBuildsOpenSearchStateProjection`、`TestLiveWriterUploadThroughput`、
`TestLiveTaihuAuthentication`、`TestLiveApplyThroughput`、`TestWriterUpdateScalesLinearly`、
`TestLiveBulkApplyAgainstRealLakeFS`、`TestLiveSnapshotPublicationJourney`、
`TestLiveHTTPSearchRerankWithLuna`、`TestSceneAccessorRefreshUpdatesSearchIndex`、
`TestLiveLakeFSSceneDFS`、`TestLiveHTTPDynamicStateSearchJourney`、`TestScaleProfileRepoAddLakeFS`。
门控分四类：真实太湖认证（1）、Luna 重排（2）、State runtime 容器与快照仓库（5）、lakeFS 吞吐与规模
（5）、其余 live journey 与 accessor 刷新（5）。它们在本批改动之前即存在，未被本批新增或隐藏，
不计入本任务缺口。按本任务口径，`passed-with-skips` 不当作无 skip 验收，因此这里如实登记而不是声称全无 skip。

索引联合：与 `INDEX-PHYSICAL-01` 联合验收表的五行已逐行定落点并在本候选上执行：

- 未索引字段变化与本地摘要不变：`TestFixedBasisKeepsDependencyCoordinatesWhileNewRequestAdvances`、
  `TestComparisonSeparatesLocalValueDeclarationKindAndVersionCoordinates`。
- A 仓定义、B 仓新修订新增方面：`TestDatasetDependencyBasisRetainsNonContributingCandidates`、
  `TestSearchReadBasisRetainsAllCandidatesAndOutlivesIndexConfiguration`（真实 OpenSearch）。
- A → B → A：`TestReturnToEarlierValueKeepsFirstBasisNewRequestAndBothRevisions`、
  `TestHistoryScanKeepsUnprovenSegmentAcrossEmptyPages`。
- 查询代回收与其后重启：`TestSearchReadBasisRetainsAllCandidatesAndOutlivesIndexConfiguration`、
  `TestBorrowedReadViewKeepsDurableBasisRecords`、`TestHistoryCheckpointSurvivesReopenWithIndependentReadMetadata`。
- Writer 无变化、发布幂等重放、新修订有效输入未变：`TestT4CommandIdempotency`、
  `TestNoChangeWriterPersistsDistinctReceiptsWithoutCommitOrNotification`、
  `TestNoChangeWriterRecoveryRechecksOriginalBaseAndReplaysOnlyDurableSuccess`。

联合表要求索引侧承担的复用、模型调用与成员产物复用判定仍由 `INDEX-PHYSICAL-01` 记账；本任务登记的是
对象侧的本地状态、完整回读、命令结果与历史，不对索引侧结论作替代。

验收结论（判定 3）：冻结候选上默认场景与真实部署专项全绿且无 skip，混合契约无失败、18 条 skip 均为
本批之前的环境门控，三道质量门禁通过，五条路径与索引联合五行均有可复核落点。
