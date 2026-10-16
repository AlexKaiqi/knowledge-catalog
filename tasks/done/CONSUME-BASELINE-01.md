# CONSUME-BASELINE-01 · 消费侧最小可用：同一修订、Dataset 维度的 list/describe、可发现的检索能力

## 目标

消费方（Agent）沿一个 Dataset 完成“列类型 → 看类型与检索能力 → 搜索 → 回读 → 关系展开”，全程固定在同一个
Dataset 修订上，不需要自己拼 pin 或 commit：

- `read`、`schema list`、`schema describe`、`search`、`relations`、`traverse`、`operations access-spec describe`
  在 `--dataset` 下接受 `--revision <N>`，按该已接受修订的冻结来源解析；与 `--pin` 同时给出时拒绝。
- `schema list --dataset` 列出 Dataset 交付的 Schema；`schema describe --dataset` 在字段上标出该 Dataset 选入的
  索引声明提供的用途，并给出相关声明成员的状态与原因，消费方只记一个 describe。
- `operations access-spec describe --dataset` 与搜索使用同一份声明编译结果（此前只给出结构，看不到声明）。
- 搜索因字段未声明用途被拒时，错误指向 `kc schema describe --dataset <id>`。
- `kc search`、`kc help consume` 等帮助按 Dataset 消费路径重写。
- 索引声明的 `text` 构建允许多个输入（跨字段全文），这是最常用、且现有引擎已能按字段承接的一项。

## 依据

- [发现](../../doc/design/discovery.md)：找到 Dataset、访问原语、翻页与预算。
- [Dataset](../../doc/design/dataset.md)：消费、修订。

## 验收

- 新增回归：修订固定（旧修订读到旧值、未发布修订报版本未解析、与 `--pin` 互斥）、Dataset 维度 schema list、
  describe 带出声明用途与未支持原因、未选入的声明不出现、access-spec describe 带出声明、搜索拒绝信息带
  describe 指引、`text` 多输入通过能力清单检查。
- 受影响包定向测试通过，`make check-docs`、`make quality` 通过。

## 前置

无。与 `INDEX-SEARCH-SPACE-01` 的关系：本任务不删除 `--repo`，帮助只把 Dataset 写成消费路径；`--repo` 退役仍归该任务。
2026-10-16 起两者改同一批文件，作为同一条线推进，不另开并行线；本任务验收单独核对。
`invocation/dataset_access.go` 由 `DEBT-INVENTORY-CONSUMPTION-01` 冻结，本任务不改该文件。

## 不做

`feedback`（归 `INDEX-USAGE-01`）；输入级 `boost`、`missing`、`maxTokens`，`keyword` 归一化与向量构建（引擎需按成员构建，
归 `INDEX-SEARCH-SPACE-01` 之后）。

## 交付

实现范围：
- 修订固定：Object 与 Discovery CLI、SDK 请求、Server 路由带 `revision`；`--revision` 须为正整数、须配 `--dataset`、与 `--pin` 互斥；Discovery 执行器以修订替换宿主传入的 pin。`invocation/dataset_revision.go` 只在解析成功时回写 pin 标记。
- `schema list --dataset [--revision]` 返回 `DatasetSchemaPage`（续页绑定首页修订）；`schema describe --dataset` 返回每成员 `DatasetSchemaReport`（字段 `access`、`search.state/reason/declarations`）。CLI 产品视图 `object/cli/output.go` 原先丢掉 `dataset`/`revision`/`search`，已改为保留。
- `operations access-spec describe` 经新增 `IndexPlugins.DeclaredAccessSpecAt` 用实例清单检查编译声明，不触碰引擎（`TestIndexPluginsExecuteRealSourceContributionsAndKeepDatasetMappings` 的“无物理操作”断言先因改用 `AccessSpecAt` 读投影元数据而红，改后绿，断言未改）。
- Dataset 搜索拒绝信息指向 `kc schema describe --dataset <id> --revision <N>`；结果带 `dataset.id/revision`（`datasetBasis` 移到 `dataset_basis.go`，使 `dataset_search.go` 回到 700 行门限内）。
- 帮助：search 以 Dataset 为主路径重写（用法行保留 `kc search (` 范围选择形式），schema list/describe、read/resolve/log/provenance、relations/traverse/access-spec、`kc help consume` 补修订；entrypoints 已重新生成。
- `text` 构建至多 8 个输入。
- 探针 `dataset-query-principals-granted/_probes/dataset-consumer-fixes-revision.feature`：消费者经 Server 沿修订 1 走 schema list 续页 → describe → access-spec → search → read/relations；修订 2 由本地 `dataset define` 发布，进程内 Server 持有启动时 Home 看不到带外发布，故修订 2 及对照的修订 1 本地回读，未发布修订与 `--repo --revision` 经 Server 断言错误码。
- 顺带修复：`cli/read_flow_test.go` 的 `TestWorkspaceSearchPublicContinuation` 原本因夹具无声明而红，补每仓一个声明成员后绿。

已验证（2026-10-16，本机 OpenSearch 夹具）：
- `go test ./discovery/... ./object/... ./invocation/... ./declarativeindex/... ./server/...` 全绿。
- `./cli` 定向：`TestMetricPermissionScenes`（全部节点）、`TestWorkspaceSearch*`、`TestShortHelpIsNotAPositional`、`TestUsageInvalidShowsReasonAndLeafUsage`、`TestEveryPublicHelp`、`TestRoleHelp`、`TestConsumeHelp`、`TestSystemCLIEntrypoints`、`TestSceneCatalog`、`TestDocumentedCommands` 全绿，无 skip。
- `make check-docs`、`make quality` 通过。

未闭项去处：`./cli` 全量另有 15 项红（`.validation/runs/20261016T0230Z-consume-baseline/kc-cli-full.log`，其中两项帮助用法已当场修）：13 项为夹具未带索引声明，Dataset/单仓搜索按设计以 `MATCH needs text access` 拒绝（如 `TestConsumeViewFollowsPublishedBranch`、`TestSearchAfterPutIsIncremental`、`TestHTTPDatasetSemanticRecallServesMemberVectorWindows`），另 `TestSceneProbesRestoreAbsentProjection` 停在 projection sync 的 `object locator lacks complete Schema referrers`。两类均属 `INDEX-DECLARATION-01` 的夹具迁移与投影改动，该线仍在改同批文件，本任务不碰。未做真实部署验收。
