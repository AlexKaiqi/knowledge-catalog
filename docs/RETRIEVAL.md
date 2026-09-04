# 检索代数与 RetrievalPlan

日期：2026-09-04
定位：SEARCH 查询代数、AccessSpec / RetrievalPlan 与 provider Probe。Binding / Observation
语义仍由 `LIVE_MATERIALIZATION.md` 拥有；投影控制由 `PROJECTION_CONTROLLER.md` 拥有。

---

## Goal

冻结知识发现的逻辑检索面：Schema 只声明 `text/filter/sort`，请求用 MATCH 与 typed filter
定位候选，命中后在固定 basis hydrate Canonical。动态 State 字段可以进入同一代数，但其
observation 权威不在 Snapshot。

## Non-Goals

- 不拥有 Binding / Observation 语义（`LIVE_MATERIALIZATION.md`）。
- 不拥有投影控制与 change notice 入站政策（`PROJECTION_CONTROLLER.md`）。
- 不拥有发现/读授权与交付屏蔽（`PERMISSIONS.md`）。
- 不把 Facet、VECTOR、SQL/RQL 或 Stream window 写进本代数。

## 硬性约束 / Invariants

- `S-01` Schema 只声明 `text/filter/sort`；字段身份是 `(schema, aspect, path)`。
- `C-01` / `R-01` SEARCH 返回 CandidateRef，必须在固定 basis hydrate；stored fields 不能代替正文。
- `V-01` 消费 SEARCH 使用本次解开的 commit，不回绕 live HEAD。
- 索引文档不按 principal 复制。

## 选定方案 / 被否决方案

- 选定：[ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023) / [ADR-027](KNOWLEDGE_CATALOG_DESIGN.md#adr-027)：一份 AccessSpec，Probe 后编 RetrievalPlan；MATCH + typed filter/PREFIX/CONTAINS。
- 否决：通用 SQL/RQL；`NOT`/空扫描伪装 completeness；semantic overlay 写进 `access[]`。系统级拒绝见 [R-10](KNOWLEDGE_CATALOG_DESIGN.md#r-10)。

## 接口契约 / 状态机

查询代数与 RetrievalPlan 以本文为准。参考实现：`retrieval/`、`index/`。Binding 句柄与 Serving State 见 `LIVE_MATERIALIZATION.md`；notice → pull 见 `PROJECTION_CONTROLLER.md` 与 `index.ChangeNotice`。

---

## 5. 索引 MVP 契约

本节冻结第一版必须能被实现和验收的逻辑检索面。它不是完整 Query DSL，也不要求每个
provider 实现全部关系代数：仓库根的 MVP 必须由真实 OpenSearch provider
完整兑现；其它 provider 按请求报告真实能力。动态 State 的物化契约属于墙外上层产品，
不能反向成为 Repository、Writer 或 Catalog 的职责。

### 5.1 MVP 要回答的问题

MVP 面向知识发现，而不是任意数据计算，必须稳定覆盖：

- 按名称、列名、描述和正文找对象；
- 按类型、系统、状态、owner、标签等结构化字段精确过滤或多选；
- 查字段存在或缺失；
- 按数值和时间做范围过滤；
- 按 qualified name、路径或技术名称做前缀定位；
- 按名称、列名或其它字符串字段做字面子串定位；
- 按相关度或一个声明过的字段排序，并稳定分页；
- 命中后在同一 basis hydrate 完整知识和版本；
- provider 或成员无法完整回答时显式失败或返回 partial，不能空成功。

MVP 只有三个 Schema 访问声明：

```text
text    analyzed text discovery
filter  typed structured predicate
sort    ordered result
```

`PREFIX` 与 `CONTAINS` 都是字符串 `filter` 的查询用法，不新增 `pattern` lane。Schema 只声明逻辑访问面，
不声明某个 provider 已经实现该算子；后者由 `Probe` 针对请求判定。
`PREFIX` 对齐 Elasticsearch term-level prefix 与 DataHub `START_WITH`，用来定位 qualified name
或技术名前缀。`CONTAINS` 对齐 Google Dataplex / Knowledge Catalog 的 `:`、DataHub `CONTAIN` 与
OpenMetadata contains：它是字面子串（`name:foo` 命中 `barfoo`），**不是** PREFIX，也不是调用方自带
`*`/`?` 的 GLOB。DataHub 弃用 `TEXT_PARTIAL` 只说明不要把它做成第四种索引标注；查询代数仍需要
这一用法。
已知 `object_id` 仍走 `RESOLVE/READ` 精确读取；只有业务 Schema 显式声明了普通字符串字段，
该字段才进入 `PREFIX`/`CONTAINS` 检索，不能把身份协议偷偷改成路径搜索。

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
| `PREFIX` | `filter` + string | 至少一个规范化字符串值具有给定前缀；不是分词 MATCH，也不是 substring/contains |
| `CONTAINS` | `filter` + string | 至少一个规范化字符串值包含给定字面子串；区分大小写；值中的 `*`/`?`/`\` 是字面量，不是通配符 |
| `SORT` | `sort` | 最多一个显式业务排序，执行器追加稳定 tie-break |
| `LIMIT` | request | 限制 residual、去重和 hydrate 后的公开 hit 数，不是 provider candidate 数 |
| continuation | request/result | 继续同一个 query/SearchView/projection；token 对调用方不透明 |

`MATCH` 使用一个显式 mode，不把不同召回语义藏进 provider 默认值：

```text
MatchMode ::= AllTerms | AnyTerms | Phrase
default   ::= AllTerms
```

- `AllTerms`：每个分析后的查询 term 都必须命中目标 text 文档；
- `AnyTerms`：至少一个 term 命中，相关度决定本地顺序；
- `Phrase`：按 analyzer 的 token 顺序做短语匹配；只能给出 superset 的 provider 必须保留 residual。

兼容 `clauses` 默认组成隐式 `All`；结构化查询可使用有深度和叶子数上限的
`SearchExpr = Clause | All | Any`。同字段多选仍优先用 `IN`。当前不提供 `Not`：物理计划没有
证明有界全集时，不能把补集伪装成可执行的候选定位。一个请求必须至少有一个定位 clause，不能
只给 `SORT` 或空过滤扫描整个知识空间；`SORT` 是请求级顺序，不能进入表达式树。

字段身份始终是 `(schema, aspect, path)`。裸 path 只在当前 AccessSpec 中唯一时可用；
歧义时必须要求调用方补全 FieldRef，不能选择第一个字段。

多值字段采用 existential 语义：`EQ/IN/range/PREFIX/CONTAINS` 只需一个值满足；`NEQ` 要求字段
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
- continuation 必须绑定 query digest、SearchView、不可变 provider generation/revision 和当前位置；
  不能拿旧 token 跟随新 HEAD、active generation 或另一条查询。
- residual false positive、去重或无权候选会消耗 candidate。执行器必须继续翻页，
  直到填满 `LIMIT`、所有 fragment exhausted，或预算耗尽后返回 partial。
- 候选坐标错误、同 basis 正文缺失或 hydrate I/O 失败必须传播为查询错误，不得作为普通候选跳过。

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
3. Snapshot basis 或动态 observation basis 满足本次 SearchView/freshness policy；
4. 所有公开 hit 都在同一计划固定的 basis hydrate 成功；
5. provider exhausted，或已证明 LIMIT 之后不影响本页语义。

默认策略是：必需 fragment `Unsupported` 时返回 `CAPABILITY_UNSATISFIED`；只有调用方显式
允许 best-effort 时才可跳过并返回 partial + claims。AccessSpec 中没有声明某字段，不是
“扫描 JSON 的兜底理由”，而是该字段不属于可检索空间。

索引文档携带固定元信息（至少 `repository`）供 typed filter，不按 principal 复制投影。谁可发现、谁可看见正文由 [`PERMISSIONS.md`](PERMISSIONS.md) §7.2 拥有。无权的墙外 Binding 不能通过 hit、total、facet、错误差异或 timing 成为旁路可见信息。

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
或 stored fields，但检索必须回固定 commit 的 Canonical hydrate。调用方信封是否含全文由
`PERMISSIONS.md` 交付链首段决定，不得用 stored fields 代替 hydrate。
Schema 对象不进入文档集。联邦查询按 Workspace 本次 pin 扇出，不为每个 Workspace 复制一份
大索引，也不把 `workspace_id/workspace_ids` 编进文档。Workspace membership 是请求时组合，
不是知识字段；同一 `(repository, basisCommit, provider, physicalDigest)` 投影可被任意多个
Workspace pin 复用。OpenSearch 的多 index 搜索、`_msearch` 或绑定 PinID 的短期 alias 只能是
执行优化，不能成为 Workspace、权限或 SearchView 的权威。

### 5.7 SEARCH 不做的事（另面承担）

下列不是「代码还没写所以从协议删掉」，而是 SEARCH 代数的边界。未冻结的 Stream 问题见 `LIVE_MATERIALIZATION.md` §8.3；产品有界 BROWSE 见 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`，不能用本表取消。

以下能力成熟但不属于 SEARCH 契约：

- 任意 `OR/NOT/括号`、通用 RQL/SQL。Google/DataHub/Purview 能做 `NOT` 和空查询/`*` browse，
  是因为它们把目录当封闭 corpus，且 browse 是 UI 起点。本协议要诚实 completeness：无界补集
  和空扫描都不能伪装成可证明的定位；有界浏览走 Schema/Catalog BROWSE（源卡片 + 类型目录，不是对象 LIST），不是 SEARCH。
- `GLOB/REGEX` 和调用方自带的前导/中缀通配模式。`CONTAINS` 已经是字面子串算子；它不是用户传入 `*`/`?` 的 GLOB，也不因 DataHub 弃用 `TEXT_PARTIAL` 索引标注而被排除出代数。
- typo tolerance、fuzzy、stemming 的跨 provider 统一语义；
- Facet/total count 作为 SEARCH 返回；若 UI 需要，作为独立 projection capability，并标 exact/approximate。有界 Schema/Catalog **BROWSE**（源卡片 + 类型目录，不是对象 LIST）是另一条产品面，不是本条延期。
- `SEMANTIC_MATCH`、VECTOR、HYBRID 和跨 lane rerank。Google、Databricks、DataHub 已把
  NL/向量叠在 keyword 之上；本协议对应 Refine / RERANK，不把 semantic 写成第四个 AccessHint。
- aggregate、join、group、graph traversal。

Stream window、ObservationCut wire format、动态 continuation 与 current-state Fold 的公开查询协议尚未冻结（§8.3）。没有 Stream capability 时失败关闭，而不是假装 Stream Binding 不存在。

这些边界不得通过改变 `MATCH`、`EQ` 或返回正文的既有含义偷偷加入。

### 5.8 验证入口

本文只冻结查询代数与 RetrievalPlan。MATCH、typed filter、continuation、Candidate hydrate 与 capability/failure 的逐项证据在 `TEST_CATALOG.md`；产品是否可用在 `MVP_ACCEPTANCE.md`。二者都不能反向删除本文已定的查询面。Binding / Observation 形态见 `LIVE_MATERIALIZATION.md`。

Provider 新增 wildcard、semantic、facet、stored payload 或 Stream window 前，必须先扩展公开
能力合同与 Conformance，不能借实现差异改变既有 `MATCH`、`EQ` 或结果 envelope 的含义。

---


---

## 6. Planning 与路由

```text
ResolvedWorkspace {repository → commit}
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

RetrievalPlan  = SearchView + fragments[] + residual + combine + hydrate + claims
Fragment       = provider + lane + basis + clauses + guarantee + coverage
```

`AccessSpec` 来自版本化 Schema；`ProjectionSpec` 是可丢的 provider 运行态；`RetrievalPlan` 每次请求根据 ResolvedWorkspace、SearchRequest、provider inventory、预算/freshness policy 编译。Schema 不出现 provider 名，也没有用户可写的 IndexDefinition。

依赖反转后的 provider 端口分开：

```text
Retriever             Probe(requirement), Retrieve(fragment, continuation)
ProjectionMaintainer  Describe(), Rebuild(spec), Apply(delta)
```

外部 Binding 可以只实现 Retriever；OpenSearch 一类 managed projection 可以同时实现 Retriever 与 ProjectionMaintainer。Repository hydrator 独立于二者，防止物理索引载荷穿透为知识结果。

Catalog 不读取 Binding，也不固定动态 cut。未来上层 Retrieval 在请求开始时创建概念上的：

```text
SnapshotBasis   repo → commit
ObservationCut  declarationCommit + declarationDigest + bindingGeneration
              + consistency(repeatable | bounded | latest-only)
              + sourceRevision | partitionOffsets | cursor/window
              + watermark? + observedAt
```

具体源只填写自己能证明的字段，不能用 `observedAt` 冒充 source revision，也不能用单个 watermark
掩盖分区偏序。若上层产品需要跨请求重放，应该显式保存 Retrieval Observation；只有 provider
承诺旧 basis 可重读时，它才是 replay token。动态 cut 不塞回 WorkspaceDefinition 或 Catalog Registry。

BM25、向量距离、图距离和外部 search score 没有天然共同尺度。Candidate union 只统一 envelope、typed identity 和 evidence，保留 provider、lane、local rank/score、matched fields 与各自 basis。

公开结果不是 CandidateSet，而是：

```text
SearchResult  = SearchView + Completeness + KnowledgeHit[]
KnowledgeHit = KnowledgeValue + KnowledgeVersion + LaneEvidence[]

KnowledgeVersion = repository + objectId + declarationCommit
                 + unit(Address, digest, schemaRef, valueBasis)[]

valueBasis = SnapshotCommit | ObservationBasis
```

`SearchView` 解释本次查询观察了哪些 Snapshot/Binding；`KnowledgeVersion` 解释返回正文的确切版本；provenance 中的 source revision 仍是第三种版本。continuation / replay token 由本文拥有。调用方信封是否含全文由 `PERMISSIONS.md` 交付链首段在收到 hydrate 后的 KnowledgeHit 之后处理，不是检索代数。

因为 residual false positive、去重或授权过滤会消耗候选，执行器必须支持 continuation：持续取 candidate page 直到填满 limit、所有 fragment exhausted 或预算耗尽。预算耗尽且可能仍有命中时返回 partial。候选坐标错误、同 basis 正文缺失或 hydrate I/O 失败必须 fail closed。跨 provider 的稳定 tie-break 至少使用 `(repository, object_id)`，不能拿异构 score 直接当全局概率。

---

---

## 7. 业界对照

下列小节只解释第 5 节契约为什么成立，不改变 `text/filter/sort` 或查询代数。

### 可直接参考的开源实现

| 项目 | 可借鉴接口 | 本项目取舍 |
|---|---|---|
| [Apache DataFusion TableProvider](https://datafusion.apache.org/library-user-guide/custom-table-providers.html) | 对每个 filter 返回 `Exact / Inexact / Unsupported`，Inexact 后保留 residual filter | `Retriever.Probe` 沿用逐 requirement 探测；另加 `Approximate` 表示可能漏候选，不能与只多返回的 Inexact/Superset 混同 |
| [Trino Connector SPI](https://trino.io/docs/current/develop/connectors.html) | `applyFilter/applyProjection/applyLimit/applyTopN` 按具体调用返回剩余条件和 guarantee | Planner 保存 residual、limit/top-N guarantee；不接受 provider 粗粒度自称“支持搜索” |
| [Apache Calcite Adapters](https://calcite.apache.org/docs/adapter) | Adapter 只实现自身 convention 支持的算子，Planner 用 rule/converter 组合异构引擎 | provider 端口由 Planner 依赖；Schema 不依赖 adapter，也不要求每个 provider 实现同一物理生命周期 |
| [Substrait](https://github.com/substrait-io/substrait) | 逻辑计划与执行后端之间的跨语言 IR、扩展和 consumer validation | 可参考 RetrievalPlan 的序列化与 conformance；当前查询代数较小，不直接引入其完整关系计划格式 |

这些项目解决的是“逻辑请求怎样下推到异构执行方”，不是 Knowledge Address、observation basis、完整 hydrate 与 provenance。因此参考其 SPI/IR 分层，不把 row/table 结果模型复制为 Knowledge Catalog 协议。


### MVP 查询面的业界覆盖

核对时间：2026-09-03。本节只解释第 5 节契约为什么成立；不改变 `text/filter/sort` 或查询代数。
Schema 声明访问面见 `ASPECT_ACCESS.md` 决策 7。

目录产品普遍是两车道——**analyzed 发现 + typed filter**——不是通用 SQL/RQL：

| 产品 | 发现 | 过滤 / 其它 | 对本契约的含义 |
|---|---|---|---|
| [Google Knowledge Catalog](https://docs.cloud.google.com/dataplex/docs/search-assets) / [search syntax](https://docs.cloud.google.com/dataplex/docs/search-syntax) | keyword；已叠 semantic overlay | `=` 精确；`:` 是 substring 或 token（`name:foo` 命中 `barfoo`）；时间比较；AND/OR/NOT；无 `*`/`?` wildcard | 主路径是 keyword + typed predicates。`:` **不是 PREFIX**。semantic 是 overlay，不是字段访问面 |
| [DataHub searchable 标注](https://docs.datahub.com/docs/metadata-modeling/extending-the-metadata-model) / [search CLI](https://docs.datahub.com/docs/cli-commands/search) | query 关键字；另有 `--semantic` | TEXT vs KEYWORD；`TEXT_PARTIAL`/`WORD_GRAM`/`queryByDefault` 已弃用，改 TEXT + `searchTier`；SDK `EQUAL`/`CONTAIN`/`START_WITH`/`END_WITH`/比较；AND/OR/NOT；默认 `search "*"` 是浏览 | 逻辑标注在收成 TEXT/KEYWORD。`START_WITH` 是 PREFIX 旁证；`CONTAIN` 是 CONTAINS 旁证，不是「不要做 substring」的理由。`*` browse 不是 SEARCH 空扫描 |
| OpenMetadata Discovery / Advanced Search | keyword；API 宣传 fuzzy | `==` / `!=` / in / contains；AND/OR；Facet | contains 与 Facet 是产品能力，不是必须写进 Schema |
| Microsoft Purview Unified Catalog | keyword、短语、AND/OR/NOT | 侧栏 facet；`field:value`；空查询或 `*` 可 match-all | match-all 对照本协议的有界 BROWSE，不是 SEARCH |
| Databricks Unity Catalog | 表名/列名/注释 keyword；另有 semantic overlay | type/owner/tag | GRANT 不进表检索（同 `ASPECT_ACCESS.md`）；semantic 仍是 overlay |
| Elasticsearch / OpenSearch | [analyzed full-text](https://www.elastic.co/docs/reference/query-languages/query-dsl/full-text-queries) | [term-level exact/range/exists/prefix](https://www.elastic.co/docs/reference/query-languages/query-dsl/term-level-queries)；fuzzy/regexp/wildcard 成熟但 expensive | `text` ≠ `keyword` 对应 `text`/`filter`。PREFIX 对齐 term-level prefix。CONTAINS 可用对 keyword 转义后的 `*literal*` wildcard 兑现 Exact；贵不等于 Approximate，也不等于用户 GLOB |

Probe 模型见上文 Probe 模型：DataFusion `Inexact` 是可能多返回、上层 residual；本项目另加
`Approximate` 表示可能漏候选。倒排和近似投影漏的项不能靠 residual 补回，结果只能 partial。

因此：

1. 三分访问面仍然正确。DataHub 还在把物理 `fieldType` 收成 TEXT/KEYWORD + `searchTier`；
   拒绝 `stored/summary/key` 与这条线同向。
2. PREFIX 留在 `filter` + string，依据是 ES prefix 与 DataHub `START_WITH`，用来定位前缀。
   不要拿 Dataplex `:` 当 PREFIX 的直接证据。
3. CONTAINS 同样留在 `filter` + string，依据是 Dataplex `:`、DataHub `CONTAIN` 与
   OpenMetadata contains。它覆盖「按名称/列名找对象」这条 MVP 主路径。TEXT_PARTIAL 弃用
   只约束 Schema 标注，不约束查询算子；实现曾经缺这一算子，不能反过来把协议写成延期。
4. Semantic 已经是产品 overlay，对应 Refine / RERANK，不进 `access[]`。
5. `NOT`、match-all、Facet 看起来「大家都有」，但不能直接抄进定位原语：前两者依赖封闭
   全集或浏览面；Facet 改的是聚合计数，按独立 projection capability 加。

MVP 选择 `MATCH + typed filter/range + PREFIX + CONTAINS + sort/page`，不是因为底层引擎只能做到
这些，而是它已经覆盖知识发现主路径，同时仍能用明确 capability 向后扩展。Facet 和 typo
tolerance 需要 UI 时可参考 [Algolia Faceting](https://www.algolia.com/doc/guides/managing-results/refine-results/faceting)。

---

