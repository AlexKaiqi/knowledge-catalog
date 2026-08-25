# index/

**③ 检索派生**：Repository/Binding 之上的规划、候选定位与可丢投影。它不拥有知识正文，不进入 Writer/Catalog 核心；命中后必须由独立 hydrator 在计划声明的 basis 上读取完整知识与版本。

## 目标边界

```text
reader                  SearchRequest / SearchResult / View / KnowledgeVersion
   ↑ semantic ports
index                   Planner / Executor / CandidateRef
                        Retriever / ProjectionMaintainer
   ↑ adapters
retrieval/              SQLite / Elasticsearch / StarRocks
upper-layer runtime     Binding pushdown / dynamic managed projection

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

多 provider score 不直接归一为概率。合并保留 provider、lane、local rank/score、matched fields；稳定 tie-break 至少使用 `(repository, object_id)`。执行器必须支持 candidate continuation，因为 residual、去重和 hydrate 失败后仍需翻页填满请求 limit；预算提前耗尽时标 partial。

Snapshot 物理投影按 `(repository, basisCommit, provider, physicalDigest)` 共享，不按 Workspace 建表。live 工作投影可以跟随 `AfterSnapshot`，但消费检索必须使用本次 ResolvedWorkspace 的 commit，不回绕 live。动态投影按 binding generation 与 observation basis 管理，属于上层 Materialization/Retrieval 产品。

## 当前 Go 实现（2026-08-25）

- `reader.AccessSpec` 只编译 `text/filter/sort`，字段身份是完整 `(schema, aspect, path)`；裸 path 仅在唯一时可用。
- `Retriever` 与 `ProjectionMaintainer` 是独立端口；当前 SQLite/ES managed engine 同时实现两者，source pushdown 可只实现 Retriever。
- Provider 先 `Probe`，声明 exact/superset/approximate/unsupported 与 coverage；无法兑现的成员不会被伪装成完整结果。
- `CandidateRef` 不携带正文；Executor 校验 repository/basis 后，在同一 Snapshot commit hydrate Canonical。
- 公开 `SearchResult` 固定 View，并返回 Completeness、Claims、完整 KnowledgeValue、KnowledgeVersion 与 LaneEvidence。
- stale/removed/wrong-basis candidate 会显式降级为 partial；公开 opaque continuation 绑定 query、View 与 Projection revision，residual 或 hydrate 消耗候选时继续翻页。
- AccessDigest 与 PhysicalDigest/ProviderRevision 分开，逻辑声明和物理重建原因可独立解释。
- Workspace 搜索按成员扇出；任一成员不支持时结果是 partial，全部不支持才返回 `CAPABILITY_UNSATISFIED`。

当前仍未实现通用的多 provider cost-based `RetrievalPlan` 与 StarRocks adapter。MVP planner 只选择一个 provider，但逐 clause Probe；SQLite 是完整常见路径 reference，ES 只实现 MATCH/EQ/IN/EXISTS/MISSING/PREFIX 子集并对其它算子明确 Unsupported。

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
| `search.go` | 候选检索、continuation、Canonical hydrate |
| `describe.go` | 固定 basis 的 `IndexDescriptor` |
