# INDEX-SEARCH-SPACE-01 · 按 Dataset 修订建检索空间，搜索只在 Dataset 内

## 目标

逻辑上每个 Dataset 修订是一个检索空间：只认选进 Dataset 的索引声明，按成员构建、按成员增量重建，
查询按五个访问原语进行，查询里不出现成员键。外部值只有 `shared` 的能进索引，命中回读后按新观察补判；
只刷新当前修订的外部值。不再有 `--repo` 搜索或“调用者可读的全部仓”这种范围。

## 依据

- [发现](../../doc/design/discovery.md)：检索空间、完整性、访问原语、索引维护。
- [Dataset](../../doc/design/dataset.md)、[权限](../../doc/design/permissions.md)、[LiveAccess](../../doc/design/liveaccess.md)。

## 验收

- [accept-08-index-build](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-08-index-build.yaml)、[accept-09-search-space](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-09-search-space.yaml) 全绿。
- 承接 [accept-06-index-declaration](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-06-index-declaration.yaml)、[accept-07-index-capability](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-07-index-capability.yaml) 中需 Dataset 内按成员准备、构建或归因的 6 条（`.data/scenes/accept.py` 的 `DATASET_INDEX_SPECS`）在测试部署上执行为绿：`accept-index-writer-accepts-unsupported`、`accept-index-array-input`、`accept-search-unsupported-member-isolated`、`accept-search-vector-member`、`accept-index-catalog-version-ahead`、`accept-index-catalog-upgrade-recheck`。
- [accept-13-cross-cutting](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-13-cross-cutting.yaml) 中 `accept-cross-shared-only-indexed`、`accept-cross-hit-recheck`、
  `accept-cross-retained-revision-not-refreshed` 变绿。
- 不变量 `AUTH-01`、`AUTH-02` 按第 4 节登记的方向改写，附反例测试；`test-catalog.md` 中 I-04、I-13、
  P-22、LiveAccess U20 更新为新证据。
- `make check-docs`、`make quality`、`make test` 通过。

## 替换的旧用例

规则：旧用例在删除对应行为的同一批改动里改写或删除，不提前删；改写时保留原用例防的风险，删除时写明由哪条新规格接住。全部处理完后 `accept-retire-*` 结构检查变绿。路径里 `RA` 指 `.data/scenes/service-initialized/source-repositories-configured/repository-attached`，`QH` 指 `.data/scenes/service-initialized/named-repositories-created/qinghe-object-published`。

- 场景状态：`RA/domain-schema-published/semantic-object-published/projection-synced/` 整个子树（含 `object-search-granted/` 的 construct 与三条 probe）删除。它的前态是“按 Schema 访问声明按仓建好索引、按仓授搜权”，新设计里不存在。三条 probe 的风险分别由 `accept-search-consume-vs-repo-read`（`search-only-principal-cannot-read`、`read-grant-returns-canonical`）与 `accept-search-undeclared-condition`（`schema-search-enforces-declared-access`）接住。
- 场景改写：`RA/.../semantic-dataset-defined/dataset-query-principals-granted/_build/construct.feature` 的 `object.search` 授权、`kc operations projection sync --repo`、`kc search --repo` 改为 Dataset 消费授权与 Dataset 修订准备；`_probes/dataset-search-follows-published-revision.feature` 里的按仓投影操作改为按修订查看准备状态；`semantic-dataset-defined/_probes/dataset-file-read-does-not-grant-repository-access.feature` 与 `dataset-manage-granted/_probes/dataset-manage-does-not-grant-read.feature` 的 `kc search --repo` 步骤改为 `--dataset`，原风险不变。
- 场景删除：`QH/_probes/probe-sync-projections-and-search.feature`，由 `accept-search-dataset-is-space` 接住；`QH/_meta.yaml` 同步。
- Go：`object.search` 动作（`invocation/allow.go`、`home/repository_access.go`、`server/service_routes.go`、`discovery/cli/` 等）与搜索、关系、遍历、投影运维命令的 `--repo` 范围删除；`cli/` 下 13 个用 `search --repo` 的测试改为 Dataset 范围或删除，`TestRepoSearchDeliveryStripsUnauthorizedBody` 的风险由“范围先于搜索”接住。`test-catalog.md` 与 `doc/engineering/architecture-invariants.md` 同步。
- 结构检查：`accept-retire-repository-scoped-search` 的检索部分。

## 前置

`INDEX-DECLARATION-01`、`LIVEACCESS-READ-01`。从当前代码出发按新设计做，不沿用已中止的 `REFACTOR-OBJECT-MODEL-01` 里按旧目标写的步骤。

2026-10-16 移入 `doing/`：`INDEX-DECLARATION-01` 代码与 `make test` 已绿，仅剩真实部署验收与并入的试运行修复；本任务按其代码基础开工，部署验收到环境恢复后补跑，补跑前本任务不移 `done/`。

`CONSUME-BASELINE-01` 与本任务改同一批 `invocation/`、`discovery/` 文件，作为同一条线推进，不另开并行线：两者在途改动一起收口，消费侧固定修订、Dataset 维度 list/describe 等先于 `--repo` 退役落地；`CONSUME-BASELINE-01` 的验收单独核对、可先移 `done/`。

## 不做

不做跨 Dataset 复用分段（只是优化，等规模压力出现再做）。

本轮按用户 2026-10-22 的推进前提，假定一个对象的各个方面在同一仓：同一 `object_id` 分散在两个仓、
由组合拼成一个对象的路径不进剩余验收范围。Dataset 选入多个来源仓、索引声明按仓限定引用别的仓
Schema 与取值描述都不受影响，仍是本任务的范围。已实现的对象组合行为及其回归保留，不在本轮删除；
撤销这条前提时按原设计恢复验收。

## 质量检查快照

2026-10-16 动画交付调用仓库质量检查时，`make check-docs` 通过；`make quality` 在 `cli/catalog_dataset_scope_internal_test.go:60` 报 `ObjectSearchRequest.InventoryDiscovery` 已不存在。此断言处于本任务已列出的检索范围与旧入口替换工作中，随同批测试收口，剩余验收仍为本任务的 `make quality` 及受影响合同。动画任务未修改该测试或产品实现。

## 接管（2026-10-16）

- 停滞判定：本线 `discovery/`、`declarativeindex/` 范围最后实质改动 11:49（`dataset_state.go`），任务文件最后更新 17:02，至 19:30+ 无修改、无提交、无在跑进程，超过 20 分钟停滞阈值，按接管规则由 Codex 接管。
- 接管内容：复核 `DEBT-INDEX-OBSERVATION-PUBLISH-RACE-01` 的“最后校验与 session.Commit 原子边界”剩余验收。结论：观察期间发布的窗口已有守卫覆盖（`TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration`，红绿证据 `.validation/runs/20261016-index-search-space/revision-publication-{red,green}.txt`）；真正未闭合的是锁内复查返回“当前”后、引擎 Commit 执行期间权威发布新修订的进程外窗口。曾尝试并发反例测试（发布钩子注入 Commit 窗口）：夹具钩子无法从包外注入，包内构造 Dataset 材料过重；且引擎 Commit 成功即 `retireGeneration` 删除旧代际，事后撤回需公开新引擎形状。按“不扩大公开形状”与三出口规则，登记为 `DEBT-DATASET-STATE-ACCEPT-ATOMIC-01`，不在本轮硬修。工作区源码改动已全部撤销（`git diff` 对 `declarativeindex/`、`liveaccess/discovery/` 干净），草稿测试文件已删除。
- 环境事实：共享部署栈 OpenSearch 分片已满（1000/1000，其中 `kc-proj-*` 残留 781 个），本轮改用 `scripts/opensearch_auto.py` 独占临时实例完成测试探测；未清理任何未确权残留索引。

- `DEBT-INDEX-UNUSED-IMPORT-01`：2026-10-16 动画交付复用仓库质量检查时，`invocation/verbs_index.go:13` 的 `kc/discovery` 导入别名 `retrieval` 未使用，阻塞编译与 `make quality`。这是本任务进行中文件的工作区快照；随本任务收口复核并修复，剩余验收为 `make quality` 通过。动画任务未修改该产品文件；原债按归并关闭。

- `DEBT-INDEX-UNUSED-CONTRACT-01`：2026-10-16 Realcase 候选方案复用统一质量门禁时，`discovery/service/search.go:25` 的 `projectedAccessContracts` 被 staticcheck 报 `U1000`。并入本任务的检索调用关系收尾；剩余验收为移除遗留函数或恢复必要调用后，受影响合同及 `make quality` 通过。证据 `.data/realcase/proposal/quality-final.log`。Realcase 本轮仅准备候选 Schema，没有修改该产品文件；原债按归并关闭。

- `DEBT-INDEX-CONTRACT-GATES-01`：`FILE-ACCESS-01` 复验发现 typed 守卫仍要求旧 `SearchExecutor`（实际 Dataset 搜索已转 `DatasetSearchExecutor`），`test-catalog.md:507` 的两项 ProjectionNotify 引用已不解析，以及 `discovery/space` 新单元、传播成本 14.09%/基线 14.01% 与 `discovery/datasets` 不稳定度 0.50→0.57 的质量门禁。原债已按同批文件规则归并；保留执行器守卫风险、旧用例替代关系与依赖图解释后，复验 `make quality`、验证库存及受影响合同。typed 定向证据 `.validation/runs/20261016T033623Z-4d171c1a0e4e/`，quality 证据 `.validation/runs/20261016T033529Z-ce304d1b84dd/`，两次 sourceChanged=false。本轮未改本任务的产品实现、守卫或质量基线。

- `DEBT-INDEX-OBSERVATION-PUBLISH-RACE-01`：`declarativeindex/projection/dataset_state.go` 在构建锁后和物理接受前复查当前修订，覆盖观察期间发布新修订的拒绝与原代保持；最后校验与 `session.Commit` 仍未同 Dataset 当前修订权威建立原子条件边界。剩余验收为在最后校验后推进修订的并发反例拒绝旧观察接受，原观察代不变。2026-10-16 复核细化：观察期间发布的窗口已被 `TestDatasetRevisionPublishedDuringObservationPreservesOldGeneration` 覆盖；未覆盖的是锁内复查返回“当前”后、引擎 `Commit` 执行期间权威发布新修订的进程外窗口，且引擎 Commit 成功即删除旧代际、无法事后撤回。该缺口登记为 `DEBT-DATASET-STATE-ACCEPT-ATOMIC-01`，原债按归并关闭。

## 2026-10-16 实现与定向验证快照

本轮以 MVP 的发布 Dataset、在 Dataset 内搜索、同修订回读为交付边界；完整任务仍在进行。

- Dataset 固定修订形成一份派生 corpus，所选来源共用物理查询与打分；索引成员和 Schema 命名空间来自选入范围，未选入的同对象其它方面不参与构建。跨仓同一对象的不同方面由既有对象组合能力合并，保留真实来源版本向量。准备、概览、固定声明检查、查询共用该依据，查询不触发构建。
- 默认构建按成员保留物理键、模板、缺失策略与归一化；显式向量成员才产生语义向量。命中保留成员、角色、声明来源版本依据。语义召回走同一 Dataset corpus。
- 搜索、关系、遍历和投影运维公开入口退役仓范围及旧搜索动作，采用 Dataset 消费或管理授权。正式投影概览返回单份 Dataset 投影和真实来源版本向量，不把派生索引坐标列成来源提交。
- 预取消在打开引擎、权威读取和构建前拒绝，metadata 沿请求上下文读取；同 corpus 的未知或未选入对象坐标失败关闭。范围反例、坐标反例与取消反例均已从红到绿。
- 外部观察准备通过固定 Dataset 描述依赖，仅接受 shared 输入；消费回读按当前观察补判，并在淘汰候选后继续翻页。索引原观察与本次 READ 新观察分别保留。刷新入口、拿锁后及接受前复查当前修订；观察期间发布新修订拒绝候选，原观察代保持。同一当前修订反复刷新使用既有独立 building 存储，避免重开已持有的观察文件。
- 裸字段条件先按已有访问合同解析再判断观察空间，保留单 Schema 下 `exists name`、`sort name` 的消费续页行为。未声明访问的拒绝保留 Dataset/revision 与 schema describe 恢复入口。

定向验证已覆盖 Dataset 单次物理查询、跨仓同对象组合、选入方面隔离、固定旧修订、真实 OpenSearch corpus 准备、Home 发布准备、有无插件、语义窗口、外部观察失败保持、消费新观察补判、保留修订拒绝刷新及同修订刷新。`discovery/...`、声明投影与插件、LiveAccess discovery、Home 的所选测试组为绿；这不替代统一门禁和真实部署验收。观察期间发布新修订的红/绿证据位于 `.validation/runs/20261016-index-search-space/revision-publication-{red,green}.txt`，红证据通过仅移除接受前复查的临时 overlay 验证风险，正式源码始终保留该守卫。

质量信号处置：范围外方面泄漏、corpus 坏坐标被吞、取消丢失、重复观察文件锁、裸字段解析时序均当场修并加风险回归；成员命名空间新增精确架构边界，拒绝 `discovery/plugin` 直连 space 及 Dataset 核心直连 reader/维护/Writer，并补间接桥毒化反例。退休 notice 后 HTTP 库存同步为 90 个入口、Server 路由 62 个，保留显式拒绝旧入口的守卫；未使用访问合同 helper 已删除。当前修订最终接受的原子边界登记并入债，未宣称完整并发合同。

最终收口前的关系/遍历定向检查在仅准备 corpus 后复现来源投影未准备的能力错误，已当场接入同一 corpus 关系端口，复用原分页、预算、续页与固定依据回读算法；正式 Dataset 一跳仅一次物理查询，遍历逐跳复用。旧提供方最小合同保持，CLI/HTTP 关系、完整遍历组以及低层错误依据、假阳性、取消、预算与续页回归绿；新增跨来源关系只入选范围、来源 HEAD 变化后仍读固定来源且一次物理查询的回归绿。证据 `.validation/runs/20261016-index-search-space/relation-traverse-{after-corpus,green}.txt` 与 `relation-corpus-fixed-sources.txt`。

首轮统一门禁中 `semantic-dataset-defined` 的退役 probe 暴露合法无 Schema/索引声明 Dataset 准备被字段提取误拒绝：结构依赖未选入检索命名空间仍被强解为索引合同。已仅跳过未参与选入合同的单位；private 引用 Schema 集保留选入声明输入和已选历史释放的严格判断。无声明准备成功且查询拒绝、ready 声明不被无索引单位阻断、选入声明但缺 Schema 仍拒绝三条低层对照绿，原节点真实 OpenSearch 定向从红到绿。另独立复核发现 `space.ReadMany` 未承接单点读的跨来源 Schema 和部分选入对象语义，已按来源固定提交分组批读并沿既有选入路径能力补齐；跨来源批次与部分对象范围反例绿。证据 `.validation/runs/20261016-index-search-space/metric-scene-{red,green}.txt`、`unindexed-unit-red.txt`、`namespace-batch-red.txt`、`unindexed-namespace-green.txt`。这里更正早期批量命名空间作为节点根因的猜测：它是独立合同缺口，节点根因由无索引单位低层反例证明。

仍需完成完整任务的成员增量重建、逐成员不支持/外部失败隔离与失败单元说明、多个索引实例的成员选择、丢失 corpus 的自动恢复，以及全部五访问原语的完整真实部署规格。当前 shared 观察若含 per-caller 输入会拒绝整次准备，尚未实现仅隔离对应声明成员。`accept-08`、`accept-09` 和所承接的六条部署规格未因此快照视为全绿；不移动任务目录。

### 2026-10-16 耦合信号收口

统一质量检查前的结构红证据 `/tmp/kc-coupling-before-freeze.log` 为传播成本 14.0921% 高于原基线 14.0123%，新增 `discovery/space` 未登记，`discovery/datasets` 不稳定度从 0.50 增至 0.57。保留 space 的真实语义归属，当场删除两条重复装配依赖：Dataset 准备通过注入的固定候选口委派，读取依据统一由 `home.DatasetPin` 转换；插件登记将 `DatasetMember` 委派给实例的只读口，具体命名空间由声明投影提供方解释。两条边各自加入架构禁止依赖及间接桥反例。

同口径重算后传播成本为 13.9717%，低于原基线，已有单元不稳定度漂移均小于 0.05，datasets 回到 0.50；显式执行 `go run ./scripts/coupling -write-baseline` 收紧总成本并登记新 space 单元，未放宽指标或修改统计口径。随后 `-strict` 为绿。定向 Dataset 准备、插件登记、真实声明插件合同及 Home 实际 OpenSearch 发布/查询组为绿，证据 `/tmp/kc-coupling-contracts-green.log`；准备口的回归核验固定来源、选入范围、依赖完整传递及失败拒绝。旧 Controller 固定来源恢复夹具仍显式使用既有逐来源 `Provider` 口，保留原故障和恢复断言；它不证明 Dataset corpus 自动恢复。本记录不替代统一门禁或真实部署规格验收。

## 对象发布收尾发现的门禁信号（2026-10-16）

- 位置与信号：固定源码副本质量门禁只剩 `discovery/space` 未登记耦合基线（传播成本 13.97%，原基线 14.01%）；第一次运行的 U1000 为 `discovery/service/search.go` 内无调用的 `projectedAccessContracts`，已删除该函数，保留 `dataset_scope.go` 仍使用的 `accessContractProjection` 接口。未改基线或放宽阈值。
- 位置与信号：完整 discovery/service 合同的 6 个 `TestWorkspaceBudget*` 和 `TestDatasetSearchKeepsReleaseWhenPublicationAdvancesBeforeObjectRead` 失败，涉及迁移后的 Dataset 派生引擎坐标与固定修订回读；全部保留原断言。
- 位置与信号：固定架构组 3 项失败：`TestCLIAndHTTPUseSameTypedApplicationExecutor` 仍要求旧 SearchExecutor；`TestRuntimeHasNoBundledDataWarehouseStory` 指向 `discovery/cli/help.go` 的 metric/gmv 业务例；`TestCatalogIsMachineCheckable` 的 I-21 仍引用两个已无定义的旧通知测试。
- 判定与处置：以上由本任务已有的检索迁移、旧入口替换与工程验收范围承接；现有自动守卫已经拒绝这些信号，无须另开改同一批文件的债。触发条件为本任务统一验收前，按新设计接住原风险并跑绿受影响合同与门禁，不删除/削弱断言。
- 证据与边界：固定副本摘要（`.validation/runs/20261016-object-self-publication/summary.json`），各最终检查均 `sourceChanged=false`。本轮没有重跑 `make test`、`make test-contracts` 或真实部署；可变工作区上的探索架构检查因正在写入的忽略目录 realcase 原材料产生大量旧形状诊断而主动终止，未算通过。固定副本只含 tracked/non-ignored 源码与材料，相关原始探索报告另存同一证据目录。

对象发布门禁最新复查：在本任务随后显式收紧耦合基线后，`make quality` 退出 0、`sourceChanged=false`；上节固定副本的耦合失败已解决。同批 discovery/service 复查的 7 项失败仍在、`sourceChanged=false`，最终验收仍需接住其风险。报告入口为上述对象发布摘要的 `latestRechecks`。


## 对象发布验收发现的操作说明债（2026-10-16）

`DEBT-CLI-SURFACE-CONTRACT-01` 登记后立即并入本任务：`cli/SURFACE.md` 的 URN 来源仓、对象命令 `--repo`、Schema 浏览范围、仓检索及 Schema 访问声明/origin 叙述仍为旧合同。判定为既存开发者说明漂移，与本任务 CLI 检索迁移同批收口；剩余验收为按对象、发现与 Affordance 设计同步旧段落，相关公开文档/命令合同及 `make check-docs`、`make quality` 通过。触发条件是本任务统一交付前。原债按归并关闭；本轮不扩大 help 或新增陌生 Agent 试跑。


## 统一契约复核（2026-10-16）

- 当前帮助、空来源 Dataset 发布及 HTTP 路由闭包的定向测试通过；空来源准备只在显式维护时证明所选范围为空，不补写来源协议元数据，也不把缺失声明索引变成零命中。完整契约与真实部署仍按冻结源码单独执行。
- 已迁移付费 live rerank 夹具的退休搜索授权，保留 Dataset 消费授权及独立 rerank 授权；未执行付费模型验收。
- 尚存墙外 Observer 夹具 `.data/scenes/service-initialized/source-repositories-configured/repository-attached/_materials/accessor/observer.py:46` 向已退役的 `projections:notice` 发送通知。该自动刷新不在本轮静态 MVP 内；剩余处置是改成维护者显式配置的 Dataset 修订刷新或实现设计允许的调度入口，并验证旧修订不刷新、当前修订观察更新与 HEAD 不变。不得把改写 live 测试当作该 Observer 已恢复。此信号登记为 `DEBT-STATE-OBSERVER-RETIRED-ROUTE-01` 后并入本任务。


## 控制索引初始化 503（2026-10-16）

`DEBT-OPENSEARCH-CONTROL-STARTUP-503-01` 登记后并入本任务：统一门禁首次组件段的 `TestConnectedPluginHandlerExplicitStartUsesOneFixedRuntime` 在 Dataset sync 读取新命名空间控制文档时收到 OpenSearch GET 503。该轮 2670 pass / 1 fail / 6 skip，后续 boundary/e2e 未启动。冻结源码未变；同一测试随后连续 5 次通过，下一次完整组件段也通过。现有证据不能区分新控制分片初始化、后台启动竞争或资源压力，不以“环境抖动”结案、不添加业务重试掩盖原因。剩余验收是在原并发下留存失败窗口的 OpenSearch shard/日志/JVM/FS/线程池依据，确认真正拥有问题的层后修复并保留原生命周期断言。根证据为 `.validation/runs/20261016-mvp-unified-043602Z/`（最终证据索引追加具体报告）；诊断采样在本轮独占节点删除前进行，不读取或清理其他任务资源。

## MVP 优先级复核（2026-10-16，当前源码只读核对）

本节覆盖前文对当前阻塞的判断，历史运行结果继续保留。此轮目标是手动发布对象与声明、发布 Dataset、按消费权限找到 Dataset、搜索、按返回修订回读；允许通过显式 projection sync 恢复可丢索引。核对依据为发现篇的范围、完整性、同版回读与维护，Dataset 篇的发布和消费，以及相关包 README；未改设计或生产实现。

- 前文七项旧失败已有在途迁移：`dataset_budget_progress_test.go` 的六项预算测试及 `dataset_fixed_basis_race_test.go` 通过 `fixedSourceSearchProjection` 明确保留原固定来源提供方合同，原预算进展、并发发布后的固定来源与范围断言仍在。不能继续把旧失败日志当作当前源码缺陷。当前 corpus 路径固定一次 Resolve，沿 pin 回读；Corpus 固定来源、范围和坏坐标已有专门回归，分页预算复用已有 `searchEngineAtContext` 执行器及其预算/取消回归，本轮未添加重复测试。
- 库存的当前消费授权筛选、默认可读 Dataset 和撤权已由 `DEBT-INVENTORY-CONSUMPTION-01` 实际落地；声明默认模板、缺值和规范化，以及显式向量能力由 `INDEX-DECLARATION-01` 已有在途实现承接。任务仍在 doing 不等于这些功能尚未实现。
- 本次静态链路源码复核没有再确认需要立即修改的实现阻碍。已知的控制索引初始化 503 仍须统一验证和失败窗口归因；不得仅凭后来通过关闭它。当前结论不是最终验收通过。
- 完整 INDEX 目标中的成员增量、逐成员 unsupported/per-caller 或外部失败隔离、多实例成员选择、丢失 corpus 自动恢复、当前观察最终接受与发布的原子边界，以及已退役 Observer 入口，继续由本任务承接，排在本轮静态 MVP 之后。`INDEX-USAGE-01` 的使用汇总、`DATASET-REVISION-01` 的自动来源跟随和其保留/存储前置不阻断此次手动流程，不因此宣称它们完成。

按用户“改完统一验证”的要求，本轮只核对源码和已有证据，未执行测试、文档门禁或启动环境。最终统一验证需覆盖 `go test ./discovery/service` 的预算与固定修订合同、受影响声明投影/插件与 CLI 链路、`make check-docs`、`make quality`、完整约定入口；真实部署独立核对。任务保持 doing，未提交。


## 部署复验发现与修复（2026-10-16）

- 5 条索引声明规格在真实部署通过；MVP 的 2 条执行规格先后识别出交付目录重复、来源子目录未显式选择、共有 Schema 重复持有。夹具改用独立交付目录和 Metric Schema 的单文件选择，保留唯一共有 Schema 来源。实际文件清单、同版本回读、未选相邻文件及三个旧混合范围用例均纳入回归；不得以放宽对象冲突处理让夹具通过。
- 随后真实部署识别出运行时启动后新建仓仍被静态索引来源映射拒绝。改为复用插件共同作用域的宿主当前准入事实，维护库存同取宿主内存集合；部署实例保持一次构造，不通过重启或热加载补齐。新增来源发布/搜索、移除来源、未接纳的非首个 Dataset 成员及物理调用不发生的反例保留；新增成员关系不授用户权限。
- 控制索引 503 已取得原始响应：共享控制索引创建期间的主分片处于 `RECOVERING`，前台 GET 与后台初始化竞争。限定这一明确状态作最多一秒、服从请求截止时间的只读元数据就绪等待；其它 503、外索引错误、其它分片状态仍立即失败，不把失败转成空元数据。新增有界等待、调用者取消、永久恢复中及非恢复错误的回归。
- 上述增量以新冻结源码重新跑统一门禁和真实规格，当前记录不代表最终验收通过。原有源内容、登记与系统仓保持原态验证；本轮未提交。

- 生命周期复核同时将 `ManagedHTTPHandler.Start` 的成功激活缓存为一次；失败仍可重试。独立激活 I/O 计数回归保留重复启动无额外工作要求，真实连接用例用激活次数归因，避免把合法的并发后台维护误算到第二次 Start；固定查询结果与关闭后无 I/O 的断言保留。


## MVP 范围内绑定坐标修复（2026-10-16，待统一验证）

继续核对静态发布、搜索、同修订读取时，确认一个先前只读结论未覆盖的真实范围缺口：Reader 的 Dataset `BindingSchemaObjectIDs` 原先只把全仓绑定 Schema 身份与已选 Schema 取交集，未检查绑定单元是否选入。同一 Schema 同时被静态单元和范围外绑定采用时，静态 Dataset 会被误判需要观察，继而因未准备观察而无法搜索。`bindingReferences` 的整对象范围判断又会漏掉部分对象中实际选入的绑定 Aspect/Member，导致其 Descriptor 依赖未冻结。两者同为绑定坐标没有按单元发表范围过滤，属于本任务已有范围与固定依据合同；不扩大 MVP 到自动观察或恢复。

- 依据为对象篇的单元与范围读取、Dataset 篇的单文件组合及依赖冻结、LiveAccess 篇的单元来源、发现篇的范围先于搜索；已重读相关 Reader 说明，不改设计。
- `object/reader/dataset_repository.go` 与 `repository_service.go` 复用已有固定 `BindingCoordinate` 和元数据引用规划，私有口保留 Address/Schema/Descriptor。先证明整对象路径完整选入；部分对象再按已选 Address 取交集。Schema 分类与 Descriptor 引用共用这一组坐标，不读正文、解析 Descriptor 或扫描全仓。完整对象沿既有 paths-only 证明继续可用；部分对象缺精确坐标、任意 native 缺 Reader 私有安全元数据口仍失败关闭，真实元数据错误不伪装为未选。没有新增公开端口，也没有在 projection 中绕过 Reader。
- 新增 `object/reader/dataset_binding_scope_test.go`：仅 Schema、只选静态对象、同对象只选静态成员、选入完整绑定对象、部分对象独立绑定成员、静态与绑定并选；Schema 分类期间正文与 Descriptor 都禁止读取，依赖冻结仅允许读取实际引用的 Descriptor。未选绑定不触发依赖读取，部分对象绑定依赖必须保留。另保留完整对象旧坐标正对照与部分对象缺坐标失败反例；原 native 缺安全元数据回归不修改。
- Reader README 已同步并重读改动段。未触碰验收夹具、控制器、插件准入、ManagedHTTPHandler、部署或运行资源；未提交。

已完成实现与回归代码、gofmt；按用户要求尚未执行任何测试或门禁。最终统一验证需包含新增 Reader 回归、`TestDatasetNativeBindingCoordinatesRequireMetadataOnlyPort`、`TestDatasetReferencePlanKeepsOnlyPublishedCoordinates`、Reader/Writer/发现与声明投影相关合同及 `make check-docs`、`make quality`、约定完整入口；本记录不表示验收通过，任务保持 doing。

### 同批依赖依据与消费穿透回归

- 依赖冻结发现任一候选仓的 HEAD 故障或空版本时，现在按已有来源合同明确失败；不会跳过未知来源后把另一仓误认成唯一 Descriptor 持有者。新增 `liveaccess/binding_dependency_failure_test.go` 保留原错误、空版本拒绝及故障仓零正文读取，既有无持有与多持有反例不变。
- 新增 `declarativeindex/projection/dataset_binding_scope_test.go`，在没有外部观察 runtime/投影的情况下，实际准备静态 Dataset、搜索并按原修订读取命中；同 Schema 的范围外绑定单元不能把该请求路由到观察空间。
- 本轮仅修改 Reader 相关范围/依赖逻辑与独立回归，未触碰另一项的部署、控制索引、插件准入或验收夹具。实现完成后统一验证，结果以本轮证据为准，不将未执行入口写成通过。

## 本轮 MVP 增量统一验证（2026-10-16）

Reader/LiveAccess 受影响包、Descriptor 唯一持有者及固定依赖、静态 Dataset 搜索与 corpus 组合合同已通过；旧 Reader 实现覆盖对照会触发范围泄漏、部分绑定依赖遗漏及未知 HEAD 被忽略的新增回归。

统一冻结源 `109295d7d0ed28077b13a358419c6a66a434f8e4737bcb018af07ad1b6feb81a`：144 个 Go 顶层测试（含子项 299 条通过事件）、60 项 Python 测试、场景 views/states/product 检查及 `make quality`（含 `make check-docs`）均通过、无 skip，各报告 `sourceChanged=false`。源码、增量 diff 与逐项证据见本轮证据索引（`.validation/runs/20261016-mvp-isolated-fixes/summary.json`）。旧实现的失败对照在全部改动后的验证阶段执行，不冒充改前先红。

本节仅交付上述独立 MVP 修复；未运行本轮源码的完整 `make test`、`make test-contracts` 或真实部署验收。任务原有全目标和缺口继续保留，目录保持 doing；未提交、安装依赖或操作其它任务的环境。追加本节后单独复查文档链接，已验证实现、测试与 README 内容保持冻结源一致。

## MVP 搜索与消费穿透修复（2026-10-16，待统一验证）

- 正式 `DatasetSearchExecutor` 的命中回读原先要求来源的整对象都被选入，合法的部分文件交付因此搜索失败；现在按固定选区规划实际选入单元，经现有逻辑引用读取后组合。完整来源继续原 READ，未选兄弟不读取，不采用索引正文。新回归穿过真实 Writer、Reader、Index 与正式执行器，覆盖单来源、跨来源、HEAD 前进、外部成功/局部失败及取消；保留引用、来源版本向量与观察依据，失败不会伪装成静态摘要。
- 完整固定成员 `file.read` 原先已经允许库存列出和下载，却被对象读取及搜索的中央授权、完整性和交付过滤拒绝。现在这些步骤使用同一份已接受成员/提交，仍独立要求 `dataset.resolve`，每层重读当前权限。只缓存本次调用坐标；共用修订解析入口复用已固定依据，不让并发发布改变本次范围。原 Dataset 级许可和首次本地采用保持既有路径；部分许可、错误提交、别的 Dataset、临时标签、伪 pin、仓直读以及撤权均有拒绝回归。
- 统一验证进一步定位到更早的候选过滤：部分来源的 Resolve 会读取整个来源对象，越过所选文件。该处改为固定版本的路径和已选 Address 元数据判断，不读取正文；新增毒化回归保留原断言。授权测试的模拟搜索响应同步补全正式 SearchView 证据，不降低生产证据合同。
- 目标依据为[发现](../../doc/design/discovery.md)的范围与同版本回读、[对象](../../doc/design/object.md)的部分单元读取、[Dataset](../../doc/design/dataset.md)的固定交付及[权限](../../doc/design/permissions.md)的整体消费。未改设计或公开 API；只修改 discovery/service 的回读、declarativeindex/projection 的候选范围过滤及 invocation 的消费授权缝和必要回归/README，没有改控制器、插件、CLI 场景夹具、部署或共享搜索资源。
- 实现与差异复核已完成；按用户要求，所有验证留到实现完成后统一执行。本节不把实现完成记为整个 INDEX 任务验收通过，任务仍在 doing，未提交。

### 本轮最终增量验证

以上实现完成后，冻结源 `04004f90814757c481d3f36b91108ce96d44c9ba3ec20b690ce5af493b954de2` 的 discovery/service、invocation、Reader/LiveAccess 受影响包、Dataset corpus 范围与搜索、Server 身份/消费/交付合同及六项架构顶层守卫全部通过：267 个 Go 顶层测试、含子项 564 条通过事件，0 fail / 0 skip；`make quality`（含 `make check-docs`）通过。七份最终报告均 `sourceChanged=false`，实现、逐项命令和原始输出见本轮证据（`.validation/runs/20261016-mvp-consumer-closeout/summary.json`）。

同一最终验证阶段对原生产实现覆盖回放，新回归在候选越界/部分对象读取与合法成员消费处按预期失败，拒绝非法授权的反例继续通过；这是改后对照，不是改前测试记录。首轮发现的候选越界已当场修并由禁止未选文件读取的回归守护；模拟搜索响应缺少 SearchView 的夹具错误已补全正式返回，不降低证据合同。

本轮独立实现范围已验证，不代表整个 INDEX 目标或真实部署验收通过。完整 `make test`、`make test-contracts` 与最新合并候选的真实部署仍须独立记录，任务保持 doing；自动跟随、恢复、成员统计等不影响手动 MVP 的内容保留原优先级。未提交、安装依赖、向另一聊天发消息或操作其环境。追加本段后仅复查文档链接，已验证实现/测试/README 字节保持冻结版本一致。

## MVP 同修订回读与组合交付闭环（2026-10-16，待统一验证）

- 上轮搜索内回读已能返回选入的部分对象，公开根引用 READ 仍落入 Reader 的完整 Canonical 前置而失败。现将部分根的逻辑读取统一到 LiveAccess 既有 ReadReference：固定元数据规划完整选入单元，经同一逻辑单元入口读取后组合；搜索复用该路径，删除原重复单元循环。根集合不伪造维护摘要，声明、外部观察、逐值失败及取消保留。Reader 的 Canonical 和整对象缓存拒绝部分对象合同不变。
- 仅有完整成员文件许可时，跨仓组合没有单一 Repository，搜索交付和 READ 末端过滤原来都会丢掉正文。两处现共用实际 Sources 与 accepted pin 的校验，重查独立 resolve 和当前全部成员许可；来源唯一、提交固定，方面恰好覆盖内容。未知、错版、缺失/重复来源、残留单仓坐标及撤权均拒绝，不普遍放行空 Repository。
- 同次依赖准备原来为各 Descriptor 引用分别取候选 HEAD，可能先后核对不同版本并用最后提交覆盖依赖。现在固定一次候选清单，每个候选的提交只读取一次，所有引用复用，包括没有命中过的候选；已选仓沿成员 pin，错误/空 HEAD/无持有者/冲突防线保留。
- 新回归覆盖真实 Writer/Reader 的部分逻辑根、Aspect/Member、外部成功与失败、取消和未选正文毒化；正式 typed 组合 READ、搜索交付、观察及撤权；依赖 HEAD 推进后删除/修改已核对 Descriptor。CLI 原应用层部分根拒绝预期改为强断言只读选入内容，并保留私有方面、仓直读、冲突反例及修订 1/2 的固定提交对照；未削弱 Reader 原守卫。
- 依据为[对象](../../doc/design/object.md)的范围组装、[发现](../../doc/design/discovery.md)的搜索与同版回读、[Dataset](../../doc/design/dataset.md)的发布依赖固定、[LiveAccess](../../doc/design/liveaccess.md)的逐值依据及[权限](../../doc/design/permissions.md)的整体消费。实现只触碰这些能力接缝和相应回归/README，未改控制器、插件、部署、共享搜索或场景夹具。实现及只读差异复核完成，未运行任何测试或门禁，按约定接下来统一验证。

### 统一验证发现的组合检索留证缺口

首轮统一验证中，部分根的 CLI、逻辑服务及原 Reader/权限防线已通过；Descriptor 新夹具因方面与 Schema 不匹配在初始化时失败，已只补匹配 Schema，保留原生产校验与固定版本断言。组合 typed READ 已通过，但组合搜索在可靠留证处仍要求单一仓和提交，无法表达同一命中的真实来源向量；这是实际流程阻塞，不能跳过留证或伪造单仓来交付。

按[服务](../../doc/design/service.md)的可靠留证和[发现](../../doc/design/discovery.md)的命中固定来源要求，候选证据补充可选的既有 SourceBasis 向量：旧单仓合同保留；组合仍是一个对象、一个排名，只接受命名 Dataset 中两个以上唯一来源，提交精确属于固定 SearchView，方面非空且不冲突，禁止混入单仓坐标。候选摘要覆盖向量，持久化、回查和样本重建保留它。反馈统计与模型重排不扩展，本节只补搜索成功所必经的证据链；最终结果另附。

### 本轮最终统一验证结果

上述实现、必要回归和说明完成后，在冻结源 `698833f5a1b0f8e3a02f715e7bf1b43077e9d378fc1746018e3562321fe7062d` 统一执行九组门禁：逻辑 LiveAccess/搜索服务、Reader 完整性与缓存、invocation/交付/耐久检索证据、Descriptor 固定依赖、正式 CLI 部分根及修订回读、Dataset corpus 范围、Server 当前授权与来源证据、架构守卫以及 `make quality`（含 `make check-docs`）全部通过。共 312 个 Go 顶层测试、含子项 687 条通过事件，0 fail / 0 skip；九份报告均 `sourceChanged=false`。实现文件摘要、增量、准确命令和原始结果见本轮最终证据（`.validation/runs/20261016-mvp-last-mile/summary.json`）。

改后验证阶段覆盖回放六份原生产文件，新增回归在部分根、组合交付、来源向量留证和依赖固定处按预期失败，确认本轮修复对应实际故障；该对照不是改前先红。首轮真实留证缺口和 Schema 夹具不匹配均已修复，旧 Schema、Canonical、授权及证据拒绝合同没有放宽。对照中来源缺失的回归先显式断言，再访问向量，避免测试以越界 panic 代替可解释的失败。

本轮仅交付上述独立 MVP 阻塞修复，不代表整个 INDEX 目标或最新合并版本的完整 `make test`、`make test-contracts`、真实部署及真实 Agent 消费验收通过；原任务保持 doing。统计、自动跟随、生命周期和恢复目标按原验收保留。未提交、安装依赖、向另一聊天发消息或操作共享服务；追加结果记录后单独复查文档链接，并核对实现/回归/README 字节与冻结版本一致。

## doing 统一补验（2026-10-16）

- 真实 HTTP 动态搜索复现 `analyzed MATCH residual requires a provider verifier`。OpenSearch 现按已发布物理字段的分析器补判新观察值，核对词项及位置，支持全部词、任一词和短语；分析请求只读并保留本次上下文取消。异常响应或不能证明的分析明确拒绝，不用子串匹配或零命中掩盖。增加词项/位置、错误响应、取消及查询上下文回归，原缺能力守卫保持。依据为发现设计的同版本回读与观察补判，公开查询形状不变。
- 冻结候选 `25e9543a8bc452f70dfc07ecd1958f5e8ead7c81a986fe7bda5658ee0b3256ee`：完整 component 2888 pass / 0 fail / 6 个显式 live skip，完整 boundary 680 pass / 0 fail / 0 skip，`make test` 18 pass / 0 fail / 0 skip，真实 State runtime/索引/HTTP 搜索专项 3 pass / 0 fail / 0 skip，`make quality`（含 check-docs）通过；各报告源码稳定。此前完整 `make test-contracts` 三段在 `547254bb…` 候选为 4818 pass / 0 fail / 11 个显式 live skip；后续变更按受影响入口复验，两个候选不混记为同一次运行。
- 真实部署原 37 条可执行检查已全部通过：本候选全量 36 条通过，最后一条按既有范围合同修正旧错误码预期后定向通过；源规格 117 条不变。归本任务的 3 条已执行，其余 29 条尚无 probe，不能视为已通过。前轮真实 DFS 的 16 节点/63 probe 及单次初始化证明保留原候选记录。
- 完整任务继续进行：成员增量重建、不支持/外部失败逐成员隔离、多实例成员选择、丢失 corpus 恢复、观察接受与修订发布的原子边界，以及未接入的原部署规格仍未验收。未配置的付费重排、Taihu、Observer、规模入口分别保留未验范围。后续在途的登录、打包与首次上传改动沿各自任务验证。
- 逐入口命令、固定源码、红绿证据、37 条规格及未验项见 `.validation/runs/20261016-doing-validation/summary.json`。未提交、未安装依赖，任务保留 doing。
