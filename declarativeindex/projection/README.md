# declarativeindex/projection/

声明式索引插件的查询与物理投影：Repository/Binding 之上的规划、候选定位与可丢投影。它不拥有对象正文，不进入 Writer/Dataset 服务 核心；命中后必须由独立 hydrator 在计划声明的 basis 上读取完整对象与版本。

## Dataset 修订检索空间

`PrepareDataset` 从固定 `DatasetPin` 的选入单位构建一个 corpus，`SearchDataset` 在该 corpus
中执行一次物理查询，跨来源使用同一套打分。编译命名空间通过
[`discovery/space`](../../discovery/space/README.md) 汇总所选 Schema 与索引成员；范围外的方面
不参与构建或候选回读。同一对象的不同方面由既有 `ComposeHoldingValues` 组合，保留真实
`Sources` 版本向量，不把 Dataset 的派生索引摘要当作某个来源仓的提交。

`DatasetMember` 只创建同一固定范围的声明视图，供实例登记委派；它不打开引擎或准备投影。

`DatasetAccessSpec`、`CheckDatasetProjection` 和 `DescribeDataset` 读取同一个准备依据。
Query 只核验就绪与投影保留的精确声明地址；未就绪、失效、不支持与未声明均拒绝，查询
不构建索引或扫描来源。候选的范围和依据先于正文读取验证；同 corpus 中不存在或未选入
的对象坐标失败关闭。查询、语义窗口和构建在预取消时不打开引擎或读取权威，元数据读取
沿上下文端口传播取消。

冻结对象的结构 Schema 可以只是 READ 依赖，没有选入检索声明或 Schema 时不参与
字段构建，也不阻止合法 Dataset 准备。选入声明引用的 Schema 或已选 Schema 的精确
采用释放仍须固定可解；不能通过跳过解释掩盖损坏的声明依据。

`RefreshDatasetState` 为当前修订发布独立观察空间，取值描述在 Dataset 固定依赖中解析，
只允许 `shared` 值进入它。刷新入口及接受前拒绝已成为保留修订的请求；观察期间发布新
修订会拒绝本次刷新，原观察仍可查。消费方经通用候选补判
接缝按本次 READ 的观察重核条件，淘汰候选后继续翻页；命中同时保留索引原观察及 READ
新观察。取消或准备失败不切换已发布的观察代。

OpenSearch 对新观察值的 MATCH 补判使用已发布物理字段的分析器，按 token 和位置核对
全文、任一词及短语；分析请求只读并沿本次搜索上下文取消。缺失、无效或无法证明的
分析响应明确报错，不改成零命中，也不以子串比较代替分析语义。

`SemanticDatasetWindow` 只从显式向量成员形成的同一 corpus 召回，返回近似窗口；全文成员
不自行产生向量。命中依据按声明成员、来源版本和角色归因，调用者不传成员键。

`RelationsDataset` 使用该 corpus 的关系候选与代际续页，在选入的固定来源中回读完整
Relation。正式 Dataset 一跳查询只执行一次物理关系查询，遍历的每一跳复用该有界分页
执行器；命中保留 Relation 自身的真实来源仓与提交，端点不扩大候选范围。底层
`RelationsAtContext` 接到 Dataset scope 时转入此端口，其余提供方合同仍在固定来源上执行。

下述按来源的 `SearchAt`、`EnsureAt` 等仍是提供方和维护合同的底层端口；正式消费命令只
走 Dataset 修订检索空间。成员增量准备、外部失败单元单独说明、丢失 corpus 的自动恢复，
以及最后一次当前修订校验与 Dataset 发布之间的原子条件边界
尚由 `INDEX-SEARCH-SPACE-01` 继续收口，本段不表示这些验收已经通过。

## 目标边界

```text
discovery                 SearchRequest / SearchResult / SearchView / ObjectVersion
  ↑                       AccessSpec / residual / merge
discovery/projection      Engine / CandidateRef / Meta / Notice / budget / candidate scope
  ↑                       通用消费与物理维护端口
declarativeindex/projection Index / Planner / Snapshot + State 发布与查询
  ↑                       只消费通用端口，不 import control
declarativeindex/opensearch 公开 adapter → internal/opensearchimpl 实际物理引擎

declarativeindex/control   Controller / TargetStore / published HEAD 对账与 worker
  →                       通过维护端口调用 Index，不进入查询执行路径
```

Schema 只声明逻辑类型与结构。采用 `kc/index-declaration` 的成员是查询能力的唯一来源；
Writer 校验形状与 Schema 字段定位符，实例按其能力清单逐成员判断支持。
声明变化单独判断索引影响，不自动构成对象值结构不兼容；固定旧版本按当时声明解释。
编译分三步：

```text
AccessSpec      固定 repository + commit + FieldRef(schema, aspect, path)
ProjectionSpec 某 provider 对 AccessSpec 的可重建物理实现
RetrievalPlan  某次 SearchRequest 的 provider fragments、residual、combine、hydrate、claims
```

Schema 不写索引能力；索引声明可以从实例清单选择 analyzer 等构建参数，不携带 provider、
index name 或原始 mapping/settings。`SetDeclarationManifest(check, digest)` 同时安装实例检查和
清单摘要；`PhysicalDigest` 合成 driver 与清单摘要，清单变化使旧物理依据失效。
`AccessDigest` 仍是逻辑声明摘要；Snapshot、Serving State、关系与查询就绪沿同一物理判断，
不能用支持结果恰好相同的旧投影冒充当前清单。

Provider 端口必须分开：

```text
Retriever             Probe(requirement) / Retrieve(fragment, continuation)
ProjectionMaintainer  Describe / Rebuild / Apply
```

`Index.SetHydrator(object.Hydrator)` 为同版本 Snapshot 正文回读提供独立接缝；
nil 保留原 `ReadMany`/精确读。SEARCH/RELATIONS 先验证候选身份和 basis，再批量查上层缓存，
仅 miss 回源；动态 State 查询继续读取已发布的同 revision Serving State，不进入 Snapshot 缓存。

后台维护、耐久目标和 notice 处理由 [control](../control/README.md) 拥有；独立 Snapshot
消费者合同由 [object/maintenance](../../object/maintenance/README.md) 拥有。
Index 构造不启动 worker，查询只读取已发布投影；维护与资源关闭由插件 Entry 组合。

`Probe` 逐 clause fragment 返回 `exact / superset / approximate / unsupported` 与 coverage。superset 在 Canonical hydrate 后执行 residual；完成 residual 后仍可返回 complete，approximate 只能支持 partial。仅支持 source pushdown 的 Binding 可以只实现 Retriever，不被迫伪造 rebuild/apply。

CandidateRef 是 provider 与 hydrator 之间的内部值，只保留 repository/object 或 dynamic resource identity、basis 与 LaneEvidence。provider 的 `_source`、stored field、summary/doc value 不得穿透为对象结果。SEARCH 在固定 basis hydrate Canonical；调用方信封是否含全文走交付链首段（`doc/design/permissions.md`）。

多 provider score 不直接归一为概率。合并保留 provider、lane、local rank/score、matched fields；稳定 tie-break 至少使用 `(repository, object_id)`。执行器必须支持 candidate continuation，因为 residual false positive、去重或授权过滤后仍需翻页填满请求 limit；预算提前耗尽时标 partial。候选坐标错误、同 basis Canonical 缺失或 hydrate I/O 失败是执行错误，不能当成普通候选消耗后继续。

Snapshot 物理投影按 `(repository, basisCommit, provider, physicalDigest)` 共享，不按 Workspace 建表。live 工作投影可以跟随 `AfterSnapshot`，但消费检索必须使用本次 ResolvedDataset 的 commit，不回绕 live。State 动态投影在同一固定声明 commit 上按 observation basis 独立发布；runtime 仍在墙外，本包只经通用 `ObservationSource` 注入的 lookup 完成 hydrate、编译和维护。

Workspace 是请求范围，不是投影文档属性。`CompiledDoc` 和 OpenSearch 文档不得出现
`workspace_id/workspace_ids` 或 PinID；一次 Workspace SEARCH 从固定 ResolvedDataset 为每个
已授权成员生成 fragment，再合并 Candidate 并 hydrate。同一 Repository basis 因而可以被多个
Workspace 复用。OpenSearch 多 index、`_msearch` 或按不可变 PinID 建短期 alias 都可以作为部署
优化，但 alias 必须可丢、可回收，且不能承担授权或版本语义。

## 当前 Go 实现

- `reader.DescribeRepoSchema` 在固定仓与版本，通过有界 Schema locator 读取字段声明，
  `retrieval.AccessSpecFromReport` 编译逻辑类型与查询能力，再进入 provider Probe 与就绪/执行检查。
  `AccessDigest` 保留归一化字段合同；查询能力改变触发重建，普通内容版本推进保持增量。
- `retrieval.AccessSpec` 只编译 `text/filter/sort`，字段身份是完整 `(schema, aspect, path)`；裸 path 仅在唯一时可用。
- `CompiledDoc` 是一个 object_id 一篇完整文档；Aspect/Member 只是维护单元。Member path 相对每个 member value 求值并合并成多值 cells。
- 类型化 cells 分离 string/long/double/boolean/date/text；`eligibleFields` 保留“适用但缺值”，使 MISSING 不会命中无关 schema。
- Relation 仍是独立对象，通用 type/direction/endpoints 进入保留投影字段；属性仍经 AccessSpec 编译。
- `Retriever` 与 `ProjectionMaintainer` 是独立端口；OpenSearch managed engine 还实现 streaming rebuild session，500-doc batch 写 candidate generation 后原子 publish。
- Provider 先逐叶子 `Probe`，声明 exact/superset/approximate/unsupported 与 coverage；显式
  `SearchExpr` 还要求 `ExpressionProber` 对整棵 `All/Any` 组合给出独立证明。无法兑现的成员不会
  被伪装成完整结果。
- `CandidateRef` 不携带正文；Executor 校验 repository/basis 后，在同一 Snapshot commit 通过
  `object.BatchReadStore.ReadMany` 按候选页 hydrate Canonical；不支持批量端口的 Repository
  才退回逐对象读。
- 公开 `SearchResult` 固定 SearchView，并返回 Completeness、Claims、完整 ObjectValue、ObjectVersion 与 LaneEvidence。
- stale/removed/wrong-basis candidate 返回 `PRECONDITION_FAILED`，不得静默降级为 partial；公开 opaque continuation 绑定 query、SearchView 与 Projection revision，residual false positive、去重或授权过滤消耗候选时继续翻页。
- AccessDigest 与 PhysicalDigest/ProviderRevision 分开，逻辑声明和物理重建原因可独立解释。
- `CheckSearchProjectionAt(repo, commit)` 只读校验固定 Snapshot 投影是否满足 `SearchAt` 的 basis、READY、AccessDigest 与 PhysicalDigest/ProviderRevision 前置条件，并返回观察到的 `Meta`。失败时仍返回已读到的生命周期，便于区分构建中、失败、停用和缺失；不会执行查询、hydrate、Ensure 或 rebuild。成功只说明 Snapshot 投影可用，不授予 SEARCH 权限，也不保证某个具体查询或动态 State 的能力。
- `RefreshState` 对固定 commit 逐 Binding lookup，用 `UnitObservation` 区分 observed null 与未观察；Serving State 落本地有界批次存储，OpenSearch 以 500-doc streaming warm rebuild 发布独立 generation，不保留全量 map 或在响应中返回全量 observations。
- `StateProjectionReady(ctx, repo, commit)` 为维护读取已发布的 Serving State 介质和实际 State
  提供方 Meta，核固定依据、READY、access/physical digest、revision 与 observation digest。
  未发布或介质丢失返回未就绪；提供方或本地读取错误保留。它不检索、hydrate、外部取值或重建。
  `StateView` 仍只返回进程内已发布观察身份，供续页校验，不证明物理就绪。
- State 查询在已有提供方 Meta 读取后、规划和候选读取前，复核同一已发布观察的固定声明依据、
  READY、access/physical digest、revision 与 observation digest。错依据返回 `PRECONDITION_FAILED`，
  不从驻留旧观察补齐另一代候选，也不现场重新取值。
- `RefreshStateObjects` 只刷新指定动态身份；刷新失败保留原已发布 revision，消费 SEARCH 从同 revision
  Serving State hydrate。通知入站与自动对账的调度归 [control](../control/README.md)。

当前仍未实现通用的多 provider cost-based `RetrievalPlan`。MVP planner 只选择 OpenSearch；它逐
clause Probe，并能证明和翻译嵌套 `All/Any`。`RetrievalFragment` 目前仍是能力解释记录，不是独立
调度的物理分支。OpenSearch 使用固定 typed mapping、Bulk、generation rebuild、独立 control
index，以及绑定 generation 与 basis 的 `search_after` continuation；每页只临时持有 PIT，发布或
增量改变依据后旧游标明确失效。它覆盖
MATCH/EQ/IN/NEQ/EXISTS/MISSING/PREFIX/CONTAINS/range/SORT；SORT 的多值规则固定为 asc=min、desc=max、
missing last。未配置 OpenSearch 时 SEARCH 返回 `CAPABILITY_UNSATISFIED`。

## 文件定位

`SearchAtContext`、`SearchStateAtRevisionContext` 与 `RelationsAtContext` 接收调用方 context；旧入口
使用默认执行策略。通用 [discovery/projection](../../discovery/projection/README.md) 的 `WithSearchBudget` 让一次 Workspace 的成员共享 `SearchBudget`，默认上限为
10,000 个候选、100 个后端页、30 秒；零值选择默认值。准备阶段超时返回临时失败，已建立固定计划后
耗尽预算则返回 partial/claims 与可继续位置。`SearchBudgetExhausted` 区分执行时限与调用方取消。
`ContextRetriever`、`ContextMetaLoader` 以及 `retrieval.ContextRelationRetriever` 是兼容的可选端口；
没有 context 的旧 Schema/Canonical 读取只能在边界检查，不能强行中断其 I/O。

涉及 MATCH 的 Superset 补判要求提供方实现 `ResidualMatchVerifier` 并证明与其分析语义一致。
类型化补判与 Workspace 比较复用 `retrieval.CompareScalarValues`，时间使用 `ParseScalarTime`。
Workspace 扇出、合并和成员 continuation 属于通用 [discovery](../../discovery/README.md)；本包只返回
固定 Repository basis 的有界候选页和查询结果。预算与范围 context 状态只在通用 projection 包定义，
本包通过其公开函数预留页、检查取消、过滤候选并绑定 continuation，不复制 context key。

增量按去重后的变更身份，分批回读两个已证明连续的 basis 后比较投影摘要；每批最多 500 个身份，
不扫描全仓。只声明 text 的字段不产生标量槽位；相同投影只推进 basis，真实变化才写入后端。

| 文件 | 负责 |
|---|---|
| `../../discovery/projection/engine.go` | 通用 Retriever / ProjectionMaintainer 端口、Candidate 与物理 Meta |
| `plan.go` | provider capability 探测与单 provider RetrievalPlan |
| `extract.go` | Canonical value + AccessSpec → provider-neutral `CompiledDoc` |
| `../../discovery/residual.go` | provider 只保证 superset 时，对已 hydrate Canonical 做逻辑补判 |
| `index.go` | `Index` 生命周期、Hook 入口与 id 清洗 |
| `runtime.go` | live / frozen pin 物理引擎缓存 |
| `spec.go` | 固定 commit 的 `AccessSpec` 编译入口 |
| `sync.go` | Ensure / Apply / Rebuild 及投影一致性判定 |
| `../control/` | durable 目标、State notice 队列、HEAD 对账、worker 与显式追赶 |
| `../../discovery/projection/notice.go` | 通用 `ChangeNotice` 入站合同：定位 hint，拒绝正文 |
| `search.go` | 候选检索、continuation、Canonical hydrate |
| `state.go` | State Binding 选择、refresh / RefreshStateObjects、Serving State 与动态 projection 发布 |
| `describe.go` | 固定 basis 的 `IndexDescriptor` |
