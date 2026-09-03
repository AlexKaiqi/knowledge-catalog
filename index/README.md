# index/

**③ 检索派生**：Repository/Binding 之上的规划、候选定位与可丢投影。它不拥有知识正文，不进入 Writer/Catalog 核心；命中后必须由独立 hydrator 在计划声明的 basis 上读取完整知识与版本。

## 目标边界

```text
retrieval               SearchRequest / SearchResult / SearchView / KnowledgeVersion
   ↑ semantic ports
index                   Planner / Executor / CandidateRef
                        Retriever / ProjectionMaintainer
                        Snapshot + State observation projection control
   ↑ adapters
retrieval/              OpenSearch service provider
upper-layer runtime     Binding lookup / source pushdown

catalog                 只提供 ResolvedWorkspace；不 import index
cli                     组装上述依赖与 Catalog.Hook
```

Schema 字段只声明 `access: [text|filter|sort]` 与逻辑类型。编译分三步：

```text
AccessSpec      固定 repository + commit + FieldRef(schema, aspect, path)
ProjectionSpec 某 provider 对 AccessSpec 的可重建物理实现
RetrievalPlan  某次 SearchRequest 的 provider fragments、residual、combine、hydrate、claims
```

Schema 不写 provider、index name、analyzer、`stored/summary/key` 或查询算子。`AccessDigest` 与 `PhysicalDigest` 分开，允许 analyzer/provider revision 改变时在 Schema 不变的情况下重建。

Provider 端口必须分开：

```text
Retriever             Probe(requirement) / Retrieve(fragment, continuation)
ProjectionMaintainer  Describe / Rebuild / Apply
```

`Probe` 逐 clause fragment 返回 `exact / superset / approximate / unsupported` 与 coverage。superset 在 Canonical hydrate 后执行 residual；完成 residual 后仍可返回 complete，approximate 只能支持 partial。仅支持 source pushdown 的 Binding 可以只实现 Retriever，不被迫伪造 rebuild/apply。

CandidateRef 是 provider 与 hydrator 之间的内部值，只保留 repository/object 或 dynamic resource identity、basis 与 LaneEvidence。provider 的 `_source`、stored field、summary/doc value 不得穿透为知识结果。公开 SEARCH 返回完整 `KnowledgeValue + KnowledgeVersion`；上层需要裁剪时在此之后处理。

多 provider score 不直接归一为概率。合并保留 provider、lane、local rank/score、matched fields；稳定 tie-break 至少使用 `(repository, object_id)`。执行器必须支持 candidate continuation，因为 residual false positive、去重或授权过滤后仍需翻页填满请求 limit；预算提前耗尽时标 partial。候选坐标错误、同 basis Canonical 缺失或 hydrate I/O 失败是执行错误，不能当成普通候选消耗后继续。

Snapshot 物理投影按 `(repository, basisCommit, provider, physicalDigest)` 共享，不按 Workspace 建表。live 工作投影可以跟随 `AfterSnapshot`，但消费检索必须使用本次 ResolvedWorkspace 的 commit，不回绕 live。State 动态投影在同一固定声明 commit 上按 observation basis 独立发布；runtime 仍在墙外，`index` 只经注入的 `StateLookup` 控制 hydrate、编译和维护。

Workspace 是请求范围，不是投影文档属性。`CompiledDoc` 和 OpenSearch 文档不得出现
`workspace_id/workspace_ids` 或 PinID；一次 Workspace SEARCH 从固定 ResolvedWorkspace 为每个
已授权成员生成 fragment，再合并 Candidate 并 hydrate。同一 Repository basis 因而可以被多个
Workspace 复用。OpenSearch 多 index、`_msearch` 或按不可变 PinID 建短期 alias 都可以作为部署
优化，但 alias 必须可丢、可回收，且不能承担授权或版本语义。

## 当前 Go 实现（2026-08-25）

- `retrieval.AccessSpec` 只编译 `text/filter/sort`，字段身份是完整 `(schema, aspect, path)`；裸 path 仅在唯一时可用。
- `CompiledDoc` 是一个 object_id 一篇完整文档；Aspect/Member 只是维护单元。Member path 相对每个 member value 求值并合并成多值 cells。
- 类型化 cells 分离 string/long/double/boolean/date/text；`eligibleFields` 保留“适用但缺值”，使 MISSING 不会命中无关 schema。
- Relation 仍是独立对象，通用 type/direction/endpoints 进入保留投影字段；属性仍经 AccessSpec 编译。
- `Retriever` 与 `ProjectionMaintainer` 是独立端口；OpenSearch managed engine 还实现 streaming rebuild session，500-doc batch 写 candidate generation 后原子 publish。
- Provider 先逐叶子 `Probe`，声明 exact/superset/approximate/unsupported 与 coverage；显式
  `SearchExpr` 还要求 `ExpressionProber` 对整棵 `All/Any` 组合给出独立证明。无法兑现的成员不会
  被伪装成完整结果。
- `CandidateRef` 不携带正文；Executor 校验 repository/basis 后，在同一 Snapshot commit 通过
  `knowledge.BatchReadStore.ReadMany` 按候选页 hydrate Canonical；不支持批量端口的 Repository
  才退回逐对象读。
- 公开 `SearchResult` 固定 SearchView，并返回 Completeness、Claims、完整 KnowledgeValue、KnowledgeVersion 与 LaneEvidence。
- stale/removed/wrong-basis candidate 返回 `PRECONDITION_FAILED`，不得静默降级为 partial；公开 opaque continuation 绑定 query、SearchView 与 Projection revision，residual false positive、去重或授权过滤消耗候选时继续翻页。
- AccessDigest 与 PhysicalDigest/ProviderRevision 分开，逻辑声明和物理重建原因可独立解释。
- Workspace 搜索按成员扇出并做 k 路全局归并：显式 SORT 使用冻结的 typed order，MATCH 使用各成员 local rank，最后以 `(repository, object_id)` 打破并列；continuation 保存每个成员下一个未读位置。任一可见成员不支持查询时 fail closed，只有授权裁剪或 provider 明确声明覆盖不足才是 partial。
- `RefreshState` 对固定 commit 逐 Binding lookup，用 `UnitObservation` 区分 observed null 与未观察；Serving State 落本地有界批次存储，OpenSearch 以 500-doc streaming warm rebuild 发布独立 generation，不保留全量 map 或在响应中返回全量 observations。
- State-field SEARCH 的 SearchView 只绑定紧凑 projection revision；每个命中携带其相关 Address observations，并从同 revision Serving State hydrate。Snapshot hook 只按 key 持久化 desired target，不在 Writer receipt 前访问 OpenSearch。长寿命 `kc serve` 的 Controller worker 启动时与周期 tick 对账 published HEAD；显式 `projection sync` 用于历史 pin、强制重建和排障。一次性 `Open()` 不得 Start。

当前仍未实现通用的多 provider cost-based `RetrievalPlan`。MVP planner 只选择 OpenSearch；它逐
clause Probe，并能证明和翻译嵌套 `All/Any`。`RetrievalFragment` 目前仍是能力解释记录，不是独立
调度的物理分支。OpenSearch 使用固定 typed mapping、Bulk、generation rebuild、独立 control
index，以及钉死不可变 generation 的 `search_after` continuation；每页只临时持有 PIT。它覆盖
MATCH/EQ/IN/NEQ/EXISTS/MISSING/PREFIX/CONTAINS/range/SORT；SORT 的多值规则固定为 asc=min、desc=max、
missing last。未配置 OpenSearch 时 SEARCH 返回 `CAPABILITY_UNSATISFIED`。

## 文件定位

| 文件 | 负责 |
|---|---|
| `engine.go` | Retriever / ProjectionMaintainer 端口、Candidate 与物理 Meta |
| `plan.go` | provider capability 探测与单 provider RetrievalPlan |
| `extract.go` | Canonical value + AccessSpec → provider-neutral `CompiledDoc` |
| `residual.go` | provider 只保证 superset 时，对已 hydrate Canonical 做逻辑补判 |
| `index.go` | `Index` 生命周期、Hook 入口与 id 清洗 |
| `runtime.go` | live / frozen pin 物理引擎缓存 |
| `spec.go` | 固定 commit 的 `AccessSpec` 编译入口 |
| `sync.go` | Ensure / Apply / Rebuild 及投影一致性判定 |
| `controller.go` | durable desired/applied target、HEAD 对账、serve worker 与显式追赶 |
| `search.go` | 候选检索、continuation、Canonical hydrate |
| `state.go` | State Binding 选择、refresh、Serving State 与动态 projection 发布 |
| `describe.go` | 固定 basis 的 `IndexDescriptor` |
