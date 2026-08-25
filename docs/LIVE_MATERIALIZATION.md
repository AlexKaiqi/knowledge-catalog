# 动态知识物化与统一检索

日期：2026-08-25
状态：**分层、Binding Snapshot envelope、检索结果与索引 MVP 逻辑契约已落地；墙外动态 observation API 仍待单独冻结。**

本文解释高频变化的当前态和事件流为什么不属于 Knowledge Catalog 的权威 Store，以及怎样通过版本化 Aspect 句柄进入统一检索。具体字段和接口由后续代码与 Conformance 描述。

---

## 1. 问题

ETL 任务展示了一个常见分裂：

- `job_id`、Aspect、Schema 和访问方式长期稳定；
- running/failed/progress 等当前值高频变化；
- 运行记录是不断增长的事件流；
- 用户既要搜“失败的任务”，也要读取某个任务的最新状态或历史窗口。

Git Snapshot 适合保存稳定知识，却不适合作为实时状态和事件流运行时。反过来，只有一个可调用句柄能读取已知资源，却不能让用户发现候选。

问题因此是：

> Repository 保存什么稳定声明，外部产品承担什么运行语义，Retrieval 又怎样把两者编译成统一发现与回源路径？

---

## 2. 第一性原理

### 2.1 知识声明与运行值分开

Aspect 回答“这是什么知识”；Binding 回答“怎样观察它”。前者和后者都可以作为稳定声明随 Repository commit 版本化，但 Binding 指向的当前值、事件、cursor 和 watermark 不进入 Snapshot。

```text
② Repository knowledge
   Aspect identity / schema / access hints / binding handle
                              ↓
M  external materialization runtime
   state / events / cursor / watermark / health
                              ↓
③ retrieval projection and routing
```

M 是上层产品能力，不是 Knowledge Catalog 新增的编号协议层，也不进入底座 import DAG。

### 2.2 State 与 Stream 是两种观察形态

- State：每个 Address 在一次观察上有 0..1 个当前值。
- Stream：每个 Address 或 ResourceRef 有 0..N 条有序记录。

二者可以互相派生：事件 Fold 成当前态，当前态变化可产生 change stream。但这只说明物化代数，不表示底座需要拥有 Stream Store。

### 2.3 每次动态观察仍必须有 basis

把 Stream 搬出底座不会消除 cursor、watermark、source revision、observedAt 和 late-data 语义。它只改变这些语义的责任人：

- Snapshot basis 由 Catalog/Repository 的 commit 保证；
- State/Stream observation basis 由 Materialization Runtime 保证；
- Retrieval 结果必须把两类 basis 分开返回。

没有 observation basis 的“实时”无法复核，也无法解释索引与回源不一致。

### 2.4 索引始终是派生状态

Snapshot、State 和 Stream 的索引都只定位候选。CandidateRef 不携带知识正文；命中后必须通过 typed reference 回到 Snapshot 或固定 Binding 读取完整知识及版本。物理引擎的 stored fields、summary、doc values 或 `_source` 只可作为内部优化，不能成为协议结果。候选已变化、消失或不可按原 basis 重读时，显式返回 stale/removed/partial。

### 2.5 Invalidation 不证明完整

通知可能丢失、合并或乱序。动态投影若要声明完整，Materialization Runtime 必须至少提供 delta、enumerate/checkpoint、周期 reconcile 或有界 TTL 中的一组恢复机制。

---

## 3. 设计决策

### 3.1 Aspect 允许声明值来源

目标概念模型：

```text
AspectDeclaration = Identity × Schema × AccessHints × ValueSource

ValueSource = Snapshot
            | Binding(mode = State | Stream)
```

- Snapshot：值就在固定 Repository commit 中。
- State Binding：句柄返回某次观察上的当前值。
- Stream Binding：句柄返回按 cursor/window 组织的记录。

`schema_ref` 描述解析后的业务值：State 描述当前值，Stream 描述单条记录。runtime、endpoint、cursor 和凭证不属于业务 Schema。

示意只表达方向，不冻结磁盘格式：

```yaml
object_id: job/orders_daily
aspect: runtime-status
schema_ref: schema/job-runtime-status
value_source:
  binding:
    mode: state
    protocol: scheduler-access/v1
    lookup: getJobStatus
    search: searchJobs
    delta: changedJobs
```

### 3.2 ResourceDescriptor 是可选包装

Binding 可以内嵌在单个 Aspect 中；多个 Aspect 共享复杂协议时，也可以引用独立 ResourceDescriptor。无论怎样包装，运行方必须能从固定 Repository commit 得到完整、可验证的访问声明。

ResourceDescriptor 是句柄包装，不是 live 知识必须独立成文件的本体结论。

### 3.3 Materialization Runtime 在底座之外

实时状态和 Stream 由更上层产品实现，包括：

- lookup、search、window、delta、subscribe；
- cursor/watermark、retention、late data、回放和当前态 Fold；
- projection controller、调度、checkpoint、reconcile；
- 源侧认证、凭证、限流、健康状态和调用 trace。

因此底座不再定义 `repository.Stream`、Writer `APPEND`、Workspace `AppendCuts` 或 Stream Adapter。Catalog 只固定 Repository commit；动态 observation cut 由上层 Retrieval 请求创建和持有。

### 3.4 Dynamic lane 是 Repo-bound、非 Repo-owned

动态 lane 与 Repository 的关系是：

```text
belongs-to       repository identity
compiled-from    declaration commit + binding digest
keyed-by         Knowledge Address or ResourceRef
observed-through fixed binding generation
```

它借 Repository 获得身份、Schema、声明版本和 Workspace 可见范围，但运行值不是 Repository 内容，也不是 Workspace 成员。

完全无 Repository 关联的资源搜索可以由上层产品提供，但它不自动获得 Knowledge Workspace 的身份、授权和完整性语义。

### 3.5 Stream 是记录集合，不是 Aspect 数组

Stream Binding 的 Schema 描述单条记录。上层产品可以维护两种不同投影：

```text
external stream
  ├── Event Projection          回答历史/window 问题
  └── Current-State Projection  通过 Fold 回答当前状态
```

候选必须保留 event identity、order、event time、observation basis 和 continuation。不能用一个无界 JSON 数组或一份含糊的“列表索引”替代这些语义。

### 3.6 接入方通知变化，平台拉取

默认采用 invalidate-and-pull：

```text
integration 发出 source changed
  → upper-layer controller 合并与调度
  → 按固定 Binding lookup/delta/enumerate
  → 更新或重建可丢投影
```

接入方不写 Elasticsearch/SQLite 等物理索引。它只声明访问能力并报告 Binding、Address、source identity 或 scope 的变化。

source key 到 Address 的映射仍属于 integration/scene。新实体需要先经 Collector 用 COMMIT 建立知识身份；否则只能作为外部 ResourceRef 返回。

### 3.7 Agent 看语义元数据，不看秘密

Agent 与 Planner 应看到 Aspect、Schema、ValueSource、逻辑访问面、freshness、retention、coverage 和 hydrate 语义。Schema 的访问面只表达 `text/filter/sort`；provider 和物理索引参数属于运行时。

watermark、lag、availability、last error 和 active generation 是运行可观测状态，不应高频 COMMIT。凭证、实际 endpoint、内部拓扑和未脱敏 payload 不暴露。

---

## 4. 物化与检索代数

### 4.1 载体与 basis

```text
ValueSource ::=
    Snapshot(repository, commit)
  | Binding(State, bindingGeneration, observationBasis)
  | Binding(Stream, bindingGeneration, cursorOrWindow)

Basis ::= Commit | ObservationBasis | StreamCut
```

| 形态 | 基数 | 权威读取 |
|---|---:|---|
| Snapshot | 每 Address 0..1 | READ @ commit |
| Bound State | 每 Address 0..1 | Binding lookup @ observation basis |
| Bound Stream | 每 Address/Resource 0..N | Binding window/lookup @ stream cut |

概念上：

```text
Stream ── Integrate/Fold ──→ State
State  ── Differentiate  ──→ Change Stream
Observe(value, basis)       → 可解释的一次动态读取
```

代数统一的是规划和增量维护，不是权威归属。只有 Snapshot 属于 Knowledge Catalog 的 Canonical Store。

### 4.2 能力

```text
Source access  Lookup | Scan | Search | Delta | Subscribe | Window
Transform      Project | Filter | Fold | Window | Union
Retrieval      Locate | Rank | Hydrate
```

只有 lookup 的 Binding 只能刷新已知对象，不能证明全局发现完整；search 可以下推；delta/enumerate 使 managed projection 能追赶和重建。

Provider 不能只报一个粗粒度的“支持 SEARCH”。Planner 必须针对具体 clause 探测保证：

```text
Guarantee ::= Exact | Superset | Approximate | Unsupported
```

- Exact：provider 直接满足 clause。
- Superset：不会漏项，但需要 hydrate 后做 residual filter。
- Approximate：可能漏项，只能产生 partial 结果。
- Unsupported：该 fragment 不可路由到此 provider。

coverage 与 freshness 另行报告，不能从 guarantee、索引存在或 invalidation 成功推断。

### 4.3 等价关系

上层产品的 Conformance 应优先验证语义而不是物理引擎：

```text
Build(X ⊕ ΔX) = Apply(Build(X), IndexDelta(X, ΔX))

Events(≤k₂) = Events(≤k₁) ⊕ Events(k₁,k₂]

Fold(Events(≤k₂)) = Fold(Fold(Events(≤k₁)), Events(k₁,k₂])

Refresh(binding, key, sourceRevision) 是幂等的
```

这些不进入 Repository Conformance；它们属于 Materialization/Retrieval 产品契约。

---

## 5. 索引 MVP 契约

本节冻结第一版必须能被实现和验收的逻辑检索面。它不是完整 Query DSL，也不要求每个
provider 实现全部关系代数：仓库根的 MVP 必须由至少一个 Snapshot reference provider
完整兑现；其它 provider 按请求报告真实能力。动态 State 的物化契约属于墙外上层产品，
不能反向成为 Repository、Writer 或 Catalog 的职责。

### 5.1 MVP 要回答的问题

MVP 面向知识发现，而不是任意数据计算，必须稳定覆盖：

- 按名称、列名、描述和正文找对象；
- 按类型、系统、状态、owner、标签等结构化字段精确过滤或多选；
- 查字段存在或缺失；
- 按数值和时间做范围过滤；
- 按 qualified name、路径或技术名称做前缀定位；
- 按相关度或一个声明过的字段排序，并稳定分页；
- 命中后在同一 basis hydrate 完整知识和版本；
- provider 或成员无法完整回答时显式失败或返回 partial，不能空成功。

MVP 只有三个 Schema 访问声明：

```text
text    analyzed text discovery
filter  typed structured predicate
sort    ordered result
```

`PREFIX` 是字符串 `filter` 的查询用法，不新增 `pattern` lane。Schema 只声明逻辑访问面，
不声明某个 provider 已经实现该算子；后者由 `Probe` 针对请求判定。
已知 `object_id` 仍走 `RESOLVE/READ` 精确读取；只有业务 Schema 显式声明了普通字符串字段，
该字段才进入 `PREFIX` 检索，不能把身份协议偷偷改成路径搜索。

### 5.2 查询代数

| 算子 | AccessHint 与类型 | MVP 语义 |
|---|---|---|
| `MATCH` | `text` | analyzed 关键字匹配；可限定 FieldRef，省略字段时覆盖所有 text 字段 |
| `EQ` | `filter` | 至少一个字段值与 typed scalar 精确相等 |
| `IN` | `filter` | 至少一个字段值等于集合中的任一 typed scalar |
| `NEQ` | `filter` | 字段存在，且没有字段值等于给定 scalar；只有完整枚举的投影才可报 Exact |
| `EXISTS` | `filter` | 字段至少有一个已索引值 |
| `MISSING` | `filter` | 字段没有已索引值；只有完整枚举的投影才可报 Exact |
| `GT/GTE/LT/LTE` | `filter` + number/date/datetime/timestamp | 按 Schema 类型比较，不按普通字符串字典序伪装时间或数值比较 |
| `PREFIX` | `filter` + string | 至少一个规范化字符串值具有给定前缀；不是分词 MATCH |
| `SORT` | `sort` | 最多一个显式业务排序，执行器追加稳定 tie-break |
| `LIMIT` | request | 限制 residual、去重和 hydrate 后的公开 hit 数，不是 provider candidate 数 |
| continuation | request/result | 继续同一个 query/view/projection；token 对调用方不透明 |

`MATCH` 使用一个显式 mode，不把不同召回语义藏进 provider 默认值：

```text
MatchMode ::= AllTerms | AnyTerms | Phrase
default   ::= AllTerms
```

- `AllTerms`：每个分析后的查询 term 都必须命中目标 text 文档；
- `AnyTerms`：至少一个 term 命中，相关度决定本地顺序；
- `Phrase`：按 analyzer 的 token 顺序做短语匹配；只能给出 superset 的 provider 必须保留 residual。

查询 clause 默认全部隐式 `AND`。同字段多选用 `IN` 表达常见的 `OR`；MVP 不提供任意
`OR/NOT/括号` AST。一个请求必须至少有一个定位 clause，不能只给 `SORT` 或空过滤扫描
整个知识空间。

字段身份始终是 `(schema, aspect, path)`。裸 path 只在当前 AccessSpec 中唯一时可用；
歧义时必须要求调用方补全 FieldRef，不能选择第一个字段。

多值字段采用 existential 语义：`EQ/IN/range/PREFIX` 只需一个值满足；`NEQ` 要求字段
存在且没有任何值等于目标值。`MISSING` 与 `NEQ` 分开，避免把缺失值偷偷解释为“不等于”。
MVP 的精确字符串比较区分大小写并按规范化后的字段值比较；需要大小写无关的业务字段，
应在物化时产生明确的规范化值。请求当前即使使用字符串 wire value，也必须先按 AccessField
类型解析，解析失败是 `USAGE_INVALID`。

### 5.3 排序、相关度与分页

- 有 `MATCH` 且无显式 `SORT` 时，provider 可以按本地相关度排序；LaneEvidence 保留
  provider、local rank/score 和 matched fields。
- 没有 `MATCH` 且无显式 `SORT` 时，使用 `(repository, object_id)` 稳定排序。
- 有显式 `SORT` 时，先按 typed field 排序，再追加 `(repository, object_id)` tie-break。
- 异构 provider 的 BM25、向量或外部 score 不直接归一成全局概率。MVP 联邦合并保留
  lane evidence，并用稳定 identity 打破并列。
- continuation 必须绑定 query digest、View、provider projection revision 和当前位置；
  不能拿旧 token 跟随新 HEAD、active generation 或另一条查询。
- residual filter、去重、无权候选或 hydrate 失败会消耗 candidate。执行器必须继续翻页，
  直到填满 `LIMIT`、所有 fragment exhausted，或预算耗尽后返回 partial。

### 5.4 Provider 能力与完整性

`Probe` 针对本次 request/fragment 返回，而不是让 provider 粗粒度声明“支持 SEARCH”：

```text
Exact        直接满足，不多不少
Superset     不漏候选，但必须 hydrate 后执行 residual
Approximate  可能漏候选，只能返回 partial
Unsupported  无合法执行路径
```

`Superset` 本身不必导致 partial：如果 residual 在完整候选集上执行完毕，结果仍可 complete。
结果只有同时满足以下条件才能声明 complete：

1. 所有必需 fragment 都有 Exact，或 Superset 已完成 residual；
2. projection coverage 为 1，且没有未恢复的 invalidation gap；
3. Snapshot basis 或动态 observation basis 满足本次 View/freshness policy；
4. 所有公开 hit 都在同一计划固定的 basis hydrate 成功；
5. provider exhausted，或已证明 LIMIT 之后不影响本页语义。

默认策略是：必需 fragment `Unsupported` 时返回 `CAPABILITY_UNSATISFIED`；只有调用方显式
允许 best-effort 时才可跳过并返回 partial + claims。AccessSpec 中没有声明某字段，不是
“扫描 JSON 的兜底理由”，而是该字段不属于可检索空间。

授权必须发生在向 provider 发请求之前。无权 Repository/Binding 不能通过 hit、total、facet、
错误差异或 timing 成为旁路可见信息。

### 5.5 Snapshot 投影生命周期

Snapshot 工作投影按 `(repository, basisCommit, provider, physicalDigest)` 标识：

```text
首次建立                                      → Rebuild
知识正文变化且 access/physical/basis 连续      → Apply(upsert/delete)
Schema 或 AccessHints 变化                     → Rebuild
provider/analyzer/normalizer revision 变化      → Rebuild
stored basis 与 from commit 不连续              → Rebuild
reconcile 发现缺失、重复或 digest 漂移          → Rebuild
```

投影只产生带 repository/object/basis/evidence 的 CandidateRef；provider 可私有保存 `_source`
或 stored fields，但公开 SEARCH 必须回固定 commit 的 Canonical hydrate 完整 KnowledgeHit。
Schema 对象不进入文档集。联邦查询按 Workspace 本次 pin 扇出，不为每个 Workspace 复制一份
大索引。

### 5.6 动态 State 物化 MVP

动态 Binding 的执行放在下层 Materialization Runtime；Index 只看已经解析、校验和规范化的
文档，不 import Binding adapter：

```text
change notice(binding/key/sourceRevision)
  → controller 合并、去重、按固定 binding generation lookup
  → Schema 校验并写 Serving State
  → Apply(upsert/delete) 到动态投影
  → 发布新的 active observation basis
```

Serving State 保存完整值及 `bindingGeneration/sourceRevision/observedAt/tombstone`；索引只保存
AccessSpec 声明的检索字段。SEARCH 从索引取得 CandidateRef 后，在同一 observation basis 从
Serving State hydrate，不需要为每个 hit 再调用外部源，也不能直接把索引载荷当权威正文。

单个任务或 Address 变化只刷新一个 key。以下情况才全量重建动态投影：Binding generation、
Schema/AccessSpec、解析算法或 physical revision 改变；checkpoint 断档；reconcile 发现无法安全
增量修复；首次建立投影。

invalidation 只是低延迟提示，不能证明完整。一个宣称 complete 的动态投影必须至少具有：

```text
invalidate → lookup 的实时路径
+ delta(since checkpoint) 或 enumerate/checkpoint 的恢复路径
+ 周期 reconcile 或有界 TTL 的兜底
```

MVP 只冻结 State 当前态，不冻结 Stream event/window 查询。跨 Serving State 与物理索引无法
原子提交时，controller 必须先写版本化 State 和投影，再切换 active observation basis；查询
发现 Candidate basis 与 State basis 不一致时返回 stale/partial，不能拼接两个版本。

### 5.7 MVP 明确延期

以下能力成熟但不属于第一版必需契约：

- 任意 `OR/NOT/括号`、通用 RQL/SQL；
- `CONTAINS/GLOB/REGEX` 和任意前导 wildcard；
- typo tolerance、fuzzy、stemming 的跨 provider 统一语义；
- Facet/total count；若上层 UI 后续需要，作为独立 projection capability，并标 exact/approximate；
- `SEMANTIC_MATCH`、VECTOR、HYBRID 和跨 lane rerank；
- aggregate、join、group、graph traversal；
- Stream window、event pattern、current-state Fold 的公开查询协议。

这些延期项不得通过改变 `MATCH`、`EQ` 或返回正文的既有含义偷偷加入。

### 5.8 MVP Conformance 最小矩阵

| 类别 | 必测结果 |
|---|---|
| 声明 | 无 AccessHint、错误类型、歧义裸 path 都被拒绝，不回退整包 JSON contains |
| 文本 | bare/scoped MATCH；AllTerms/AnyTerms/Phrase；analyzer revision 触发 physical rebuild |
| 过滤 | EQ/IN/NEQ/EXISTS/MISSING；多值字段与缺失字段语义固定 |
| 类型 | number/time 四种比较按 typed value；错误值拒绝；PREFIX 只允许 string filter |
| 组合 | clause 隐式 AND；IN 表达同字段 OR；只有 SORT 的请求拒绝 |
| 版本 | SearchAt 只 hydrate 请求 commit；旧 continuation 不跟新 basis |
| 能力 | Exact/Superset+residual/Approximate/Unsupported 四条路径和 coverage/freshness claims |
| 结果 | Candidate 无正文；公开 hit 有 KnowledgeVersion/Evidence；失败消耗候选后继续翻页 |
| 动态上层 | invalidate 幂等、乱序/重复、delete、checkpoint gap、reconcile、basis mismatch |

### 5.9 与当前 Go 实现的差距

本节是实现检查点，不改变前述规范：

| MVP 契约 | 当前实现 | 后续动作 |
|---|---|---|
| MATCH mode | 已实现 `AllTerms/AnyTerms/Phrase`，默认 AllTerms | 后续 analyzer revision 仍由 PhysicalDigest 管理 |
| MISSING / PREFIX | SQLite 与 ES 高频子集已实现 | 其它 provider 逐请求如实 Probe |
| typed scalar/range | string wire value 按 AccessField type 规范化；number/time range 不再按普通字符串比较 | MVP 不扩任意 scalar/collection DSL |
| opaque continuation | 单仓与 Workspace token 已绑定 query、View、Projection revision/成员位置 | 暂不承诺跨 provider 全局 score 游标 |
| Superset residual | 已在固定 basis hydrate 后执行通用 MVP residual；完成后可报 complete | Approximate 或 hydrate/basis 缺口仍为 partial |
| 逐 fragment Probe | 单 provider planner 已逐 clause fragment Probe | 多 provider cost/routing 暂缓 |
| reference/profile | SQLite 覆盖常见 MVP 全路径；ES 覆盖 MATCH/EQ/IN/EXISTS/MISSING/PREFIX | ES 对 range/NEQ/SORT 明确 Unsupported |
| 动态 State | 根包只有稳定 Binding resolution，没有 controller/Serving State | 保持墙外实现，不在 Repository/Writer/CLI 长运行时 |

实现顺序固定为：先收窄请求和结果语义，再补 SQLite reference conformance，然后实现
residual/continuation/completeness，最后迁移 ES 和墙外动态 Runtime。不能先让某个 provider
私自增加 wildcard、semantic 或 stored payload 返回。

---

## 6. Planning 与路由

```text
WorkspacePin {repo → commit}
  → 读取该 commit 上的 Aspect/Schema/Binding
  → 编译 AccessSpec
  → Retrieval Planner 按 clause Probe capability 与 runtime policy
  → 选择 Snapshot projection、source pushdown 或 managed dynamic projection
  → 分页取得 CandidateRef
  → 按 typed reference hydrate 完整知识与版本
  → residual filter / union / deduplicate / rank
  → 未填满 limit 时继续 candidate page
```

逻辑声明、物理投影和单次计划是三个对象，不能合并成 `IndexPlan`：

```text
FieldRef       = schema + aspect + path
AccessField    = FieldRef + type + access[text|filter|sort]
AccessSpec     = repository + commit + fields + accessDigest

ProjectionSpec = providerId + repository + targetBasis
               + accessDigest + providerRevision + physicalDigest + fields

RetrievalPlan  = view + fragments[] + residual + combine + hydrate + claims
Fragment       = provider + lane + basis + clauses + guarantee + coverage
```

`AccessSpec` 来自版本化 Schema；`ProjectionSpec` 是可丢的 provider 运行态；`RetrievalPlan` 每次请求根据 ResolvedWorkspace、SearchRequest、provider inventory、预算/freshness policy 编译。Schema 不出现 provider 名，也没有用户可写的 IndexDefinition。

依赖反转后的 provider 端口分开：

```text
Retriever             Probe(requirement), Retrieve(fragment, continuation)
ProjectionMaintainer  Describe(), Rebuild(spec), Apply(delta)
```

外部 Binding 可以只实现 Retriever；SQLite/Elasticsearch 一类 managed projection 可以两者都实现。Repository hydrator 独立于二者，防止物理索引载荷穿透为知识结果。

Catalog 不读取 Binding，也不固定动态 cut。Retrieval 在请求开始时创建：

```text
SnapshotBasis   repo → commit
ObservationCut  binding generation → watermark/cursor/observedAt
```

若上层产品需要跨请求重放，应该显式保存 Retrieval Observation，而不是把动态 cut 塞回 WorkspaceDefinition 或 Catalog Registry。

BM25、向量距离、图距离和外部 search score 没有天然共同尺度。Candidate union 只统一 envelope、typed identity 和 evidence，保留 provider、lane、local rank/score、matched fields 与各自 basis。

公开结果不是 CandidateSet，而是：

```text
SearchResult  = View + Completeness + KnowledgeHit[]
KnowledgeHit = KnowledgeValue + KnowledgeVersion + LaneEvidence[]

KnowledgeVersion = repository + objectId + declarationCommit
                 + unit(Address, digest, schemaRef, valueBasis)[]

valueBasis = SnapshotCommit | ObservationBasis
```

`View` 解释本次查询观察了哪些 Snapshot/Binding；`KnowledgeVersion` 解释返回正文的确切版本；provenance 中的 source revision 仍是第三种版本。上层若需 token 裁剪，只能在收到完整 KnowledgeHit 后处理。

因为 residual filter、去重或 hydrate 失败会消耗候选，执行器必须支持 continuation：持续取 candidate page 直到填满 limit、所有 fragment exhausted 或预算耗尽。预算耗尽且可能仍有命中时返回 partial。跨 provider 的稳定 tie-break 至少使用 `(repository, object_id)`，不能拿异构 score 直接当全局概率。

---

## 7. 调研结论

这套方向是几条成熟研究线的交叉。

### 7.1 数据集成与能力驱动改写

- Halevy 的 [Theory of Answering Queries Using Views](https://homepages.inf.ed.ac.uk/libkin/dbtheory/alon.pdf) 说明怎样用已声明的 view/source 回答统一查询。
- IBM Garlic 的 [Capabilities-Based Query Rewriting](https://research.ibm.com/publications/capabilities-based-query-rewriting-in-mediator-systems) 让源声明能力，由 mediator 生成可执行子查询和组合计划。
- Garlic/DB2 的 [Federated Query Processing](https://research.ibm.com/publications/garlic-a-new-flavor-of-federated-query-processing-for-db2) 证明 Catalog 元数据、源模块与联邦优化器可以协同。
- Li 的 [Limited Access Patterns](https://ics.uci.edu/~chenli/pub/jvldb03.pdf) 说明只有受限 lookup 时，并非所有查询都能得到完整答案。

结论：capability checking、pushdown 和 cost-based routing 很成熟；任意外部协议与任意查询的完全自动改写不成立。

### 7.2 Stream–Relation 与增量视图

- [CQL](https://web.stanford.edu/class/cs245/readings/cql.pdf) 区分 stream、relation、window 和 relation-to-stream 输出。
- [DBToaster](https://vldb.org/pvldb/vol5/p968_yanifahmad_vldb2012.pdf) 研究高频动态视图的高阶 delta 维护。
- [Differential Dataflow](https://www.cidrdb.org/cidr2013/Papers/CIDR13_Paper111.pdf) 用逻辑时间维护变化中的计算。
- [DBSP](https://www.vldb.org/pvldb/vol16/p1601-budiu.pdf) 用 differentiation/integration 给出通用 IVM 代数。

结论：Stream、当前态、window/cut 和增量维护有成熟理论；这支持上层产品实现 Materialization Runtime，不构成把 Stream 塞进 Repository 的理由。

### 7.3 联邦检索

[Searching Distributed Collections with Inference Networks](https://sigir.org/wp-content/uploads/2017/06/p160.pdf) 已把分布式检索拆成 collection representation、selection 和 result merging。

结论：自动选源与候选合并成熟；异构相关性分数仍不天然可比。

### 7.4 成熟度判断

| 能力 | 判断 |
|---|---|
| Binding capabilities → 合法 pushdown | 成熟，可借鉴 |
| State/Stream/window 时间语义 | 成熟，可交给专门运行时 |
| 增量维护 filter/join/aggregate/fold | 成熟，有理论与系统实现 |
| passthrough 与 managed projection 自动选路 | 可行，需要 stats/SLA/成本策略 |
| invalidation-only 保证完整 | 不成立 |
| 任意外部源完全自动改写 | 不成立 |
| 异构检索 score 统一标尺 | 不成熟，不应伪造 |
| Knowledge Address + Binding basis + Agent 元数据 | 没有现成标准，是本项目设计空间 |

### 7.5 可直接参考的开源实现

| 项目 | 可借鉴接口 | 本项目取舍 |
|---|---|---|
| [Apache DataFusion TableProvider](https://datafusion.apache.org/library-user-guide/custom-table-providers.html) | 对每个 filter 返回 `Exact / Inexact / Unsupported`，Inexact 后保留 residual filter | `Retriever.Probe` 沿用逐 requirement 探测；另加 `Approximate` 表示可能漏候选，不能与只多返回的 Inexact/Superset 混同 |
| [Trino Connector SPI](https://trino.io/docs/current/develop/connectors.html) | `applyFilter/applyProjection/applyLimit/applyTopN` 按具体调用返回剩余条件和 guarantee | Planner 保存 residual、limit/top-N guarantee；不接受 provider 粗粒度自称“支持搜索” |
| [Apache Calcite Adapters](https://calcite.apache.org/docs/adapter) | Adapter 只实现自身 convention 支持的算子，Planner 用 rule/converter 组合异构引擎 | provider 端口由 Planner 依赖；Schema 不依赖 adapter，也不要求每个 provider 实现同一物理生命周期 |
| [Substrait](https://github.com/substrait-io/substrait) | 逻辑计划与执行后端之间的跨语言 IR、扩展和 consumer validation | 可参考 RetrievalPlan 的序列化与 conformance；当前查询代数较小，不直接引入其完整关系计划格式 |

这些项目解决的是“逻辑请求怎样下推到异构执行方”，不是 Knowledge Address、observation basis、完整 hydrate 与 provenance。因此参考其 SPI/IR 分层，不把 row/table 结果模型复制为 Knowledge Catalog 协议。

### 7.6 MVP 查询面的业界覆盖

- [Google Knowledge Catalog Search](https://docs.cloud.google.com/dataplex/docs/search-assets)
  把常用交互收敛为 keyword/natural-language + typed filters；不同过滤维度 AND，同一维度
  多选 OR，数值和 datetime 走类型化条件。
- 它的[搜索语法](https://docs.cloud.google.com/dataplex/docs/search-syntax)支持精确值、比较和
  受限布尔组合，但明确不支持 `*`/`?` wildcard。这说明任意 pattern 不是知识目录的基础门槛。
- Elasticsearch 同样区分 [analyzed full-text](https://www.elastic.co/docs/reference/query-languages/query-dsl/full-text-queries)
  与 [term-level exact/range/exists/prefix](https://www.elastic.co/docs/reference/query-languages/query-dsl/term-level-queries)；
  fuzzy、regexp、wildcard 等能力虽然成熟，但可能是 expensive query，不应自动成为统一契约。
- Facet 和 typo tolerance 是成熟搜索产品的高频 UI 能力，但分别影响聚合计数和召回/排序，
  不应伪装成基础 filter。需要 UI 时可参考 [Algolia Faceting](https://www.algolia.com/doc/guides/managing-results/refine-results/faceting)，
  以独立 provider capability 加入。

因此 MVP 选择 `MATCH + typed filter/range + PREFIX + sort/page`，不是因为底层引擎只能做到
这些，而是它已经覆盖知识发现主路径，同时仍能用明确 capability 向后扩展。

---

## 8. 已定边界与待冻结问题

已定：

- 底座的权威 Store 只有 Snapshot；
- Aspect 可声明 Snapshot、State Binding 或 Stream Binding；
- `value_source` 写在 Address 单元 frontmatter；Snapshot 为默认，Binding 有独立 DeclarationDigest；
- inline Binding 与同 commit 的 ResourceDescriptor reference 同时支持，且二者互斥；
- Binding 声明属于 ②，运行值属于墙外 Materialization Runtime；
- Catalog 只固定 Repository commit，不固定动态 cursor/watermark；
- Writer 不提供 APPEND；动态值若要沉淀，Collector 显式 COMMIT Snapshot；
- Stream Schema 描述单条记录，不是数组；
- 接入方通知变化，平台按 Binding 拉取；
- Projection 非权威，命中后按 typed reference hydrate；
- Schema 只声明 `text/filter/sort`；`stored/summary/key` 不进入访问契约；
- 索引 MVP 是 `MATCH(AllTerms/AnyTerms/Phrase)`、typed `EQ/IN/NEQ/EXISTS/MISSING/range/PREFIX`、一个显式 `SORT`、`LIMIT` 与 opaque continuation；
- MVP clause 隐式 AND，同字段 OR 用 IN；不提供任意布尔 AST；
- 动态 MVP 采用 invalidate-and-pull + Serving State + 增量投影 + checkpoint/reconcile，且只冻结 State 当前态；
- AccessSpec、ProjectionSpec 与每请求 RetrievalPlan 分离；
- Retriever 与 ProjectionMaintainer 分离，provider capability 按 clause 探测；
- CandidateRef 无知识正文，公开 SEARCH 返回完整 KnowledgeHit 与 KnowledgeVersion；
- 无法证明无漏项时结果必须标 partial。

待冻结：

1. Canonical READ 与显式 dynamic hydrate 的 API 分界；当前 `resolve-binding` 只返回稳定声明；
2. 旧 Repository commit 对已经下线的旧 runtime generation 的可用性；
3. Change notice 是否必须携带已解析 Address；
4. ObservationCut 对 watermark、分区水位和部分序的表达；
5. Stream Event Projection 与 Current-State Fold 的声明位置；
6. passthrough 与 managed projection 的授权、成本和 retention 模型；
7. stale/removed 在同步返回、流式事件和 retryable error 之间的具体编码；
8. 上层 Materialization/Retrieval Conformance 的测试载体与包归属。
