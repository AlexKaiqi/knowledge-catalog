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
- `C-01` / `R-01` Retriever 只返回候选；SEARCH 必须在固定 basis hydrate 后交付知识结果，stored fields 不能代替正文。
- `V-01` 消费 SEARCH 使用本次解开的 commit，不回绕 live HEAD。
- `CA-02` / `CA-03` 内部正文缓存保持同版本完整 hydrate，候选页 miss 只批量回源未命中部分；不改变交付链。
- 索引文档不按 principal 复制。

## 选定方案 / 被否决方案

- 选定：[ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023) / [ADR-027](KNOWLEDGE_CATALOG_DESIGN.md#adr-027)：一份 AccessSpec，Probe 后编 RetrievalPlan；MATCH + typed filter/PREFIX/CONTAINS。
- 选定：正文缓存位于服务端独立 hydrate 端口，复用完整 Snapshot 读取；Retriever 继续只定位候选。
- 否决：SDK 收到候选后自行拼装 SEARCH 正文；缓存伪装成 Retriever、索引 `CompiledDoc` 或动态 Serving State。
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
应在物化时产生明确的规范化值。传输形式不能改变类型语义；执行前应按声明类型解析输入，无法解析时拒绝请求。具体编码和错误合同由公开 API 与 Conformance 维护。

整数从知识发布、物理索引到续页和联邦比较均须保真，不能以浮点近似充当精确比较。时间选定纳秒精度，
支持范围由共享知识类型合同确定；更细且非零的精度必须明确拒绝，不能默默截断。时间规范化、物理表示和
排序应表达同一时刻，不能因后端默认毫秒精度或较窄日期范围收窄逻辑能力。

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

一次 Workspace 请求共享执行预算，限制总候选、后端页数与时间，不能由成员子查询反复重置。
输出条数不等于执行成本。预算耗尽时标明部分完成并保留准确可继续位置；调用方取消应传播，
不得继续发起下一轮访问。已有不支持取消的外部端口只能在调用边界检查，不能虚报其 I/O 已被中断。
成员批量缓冲不得把尚未消费的知识正文塞进游标；缺少成员头部时，也不能假装已证明全局顺序。
初始获取应避免单个成员的预取挤占其它成员建立排序依据的预算。空页只有在成员的实际读取位置、
已消费偏移或耗尽状态发生可保存进展时才可继续；仅重新包装相同位置不算进展。预算不足且无法前进时
应明确失败，不能反复返回相同空页；补判淘汰候选但读取位置确已推进时仍可续页。
准备阶段尚未建立可验证的固定计划时，超时应明确失败；只有已有执行依据时才返回可继续的部分结果，
不能为超时请求编造尚未验证的游标。

### 5.4 Provider 能力与完整性

`Probe` 针对本次 request/fragment 返回，而不是让 provider 粗粒度声明“支持 SEARCH”：

```text
Exact        直接满足，不多不少
Superset     不漏候选，但必须 hydrate 后执行 residual
Approximate  可能漏候选，只能返回 partial
Unsupported  无合法执行路径
```

`Superset` 本身不必导致 partial：如果 residual 在完整候选集上执行完毕，结果仍可 complete。
补判还必须证明与原表达式语义一致；字符串包含不能替代分词或短语判断。缺少分析语义证明时，
涉及全文匹配的补判组合应明确不支持；单个叶子的证明不能代替整棵布尔表达式的证明。
结果只有同时满足以下条件才能声明 complete：

1. 所有必需 fragment 都有 Exact，或 Superset 已完成 residual；
2. projection coverage 为 1，且没有未恢复的 invalidation gap；
3. Snapshot basis 或动态 observation basis 满足本次 SearchView/freshness policy；
4. 所有公开 hit 都在同一计划固定的 basis hydrate 成功；
5. provider exhausted，或已证明 LIMIT 之后不影响本页语义。

动态消费分别判断覆盖、新鲜度、可重读性与当前授权（`LIVE_MATERIALIZATION.md` §2.6）。
完整遍历一个旧观察集合不能证明“当前没有匹配项”；零命中也必须有查询范围级的覆盖与时效
依据，不能只依赖 hit 中的观察时间。多个 Binding 无协调协议时，不得把一次 projection revision
解释成来源的全局原子快照。公开合同尚不能表达所需承诺时明确缺能力，不先放宽以上 complete
条件；分页、同依据 hydrate 和来源撤权也不能通过 best-effort 绕过。

默认策略是：必需部分没有合法执行能力时明确失败；只有调用方显式
允许 best-effort 时才可跳过并返回 partial + claims。AccessSpec 中没有声明某字段，不是
“扫描 JSON 的兜底理由”，而是该字段不属于可检索空间。

索引文档携带固定元信息（至少 `repository`）供 typed filter，不按 principal 复制投影。谁可发现、谁可看见正文由 [`PERMISSIONS.md`](PERMISSIONS.md) §7.2 拥有。无权的墙外 Binding 不能通过 hit、total、facet、错误差异或 timing 成为旁路可见信息。

### 5.5 检索与投影维护的分工

检索计划选择与请求知识版本匹配的投影；它不负责在消费请求中创建或追赶索引。首次构建、
连续增量、声明变化后的重建及发布失败恢复，由 [投影控制设计](PROJECTION_CONTROLLER.md) 拥有。

Workspace 是请求范围，不能成为知识字段或另一份索引权威。同一个仓与固定版本的投影可以
供多个 Workspace 使用；多索引查询、临时 alias 等物理优化不能改变成员、授权与版本语义。
Schema 对象通过专门的类型浏览入口发现，不把字段定义顺带索引成普通业务正文。

### 5.7 SEARCH 不做的事（另面承担）

下列不是「代码还没写所以从协议删掉」，而是 SEARCH 代数的边界。未冻结的 Stream 问题见 `LIVE_MATERIALIZATION.md` §8.3；产品有界 BROWSE 见 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`，不能用本表取消。

以下能力成熟但不属于 SEARCH 契约：

- 无约束字符串查询语言、通用 RQL/SQL 与未经有界全集证明的补集。已有的有界 All/Any 组合仍
  属于查询代数；它不等于允许任意字符串表达式。目录产品提供 NOT 和全量浏览能力，
  是因为它们把目录当封闭 corpus，且 browse 是 UI 起点。本协议要诚实 completeness：无界补集
  和空扫描都不能伪装成可证明的定位；有界浏览走 Schema/Catalog BROWSE（源卡片 + 类型目录，不是对象 LIST），不是 SEARCH。
- `GLOB/REGEX` 和调用方自带的前导/中缀通配模式。`CONTAINS` 已经是字面子串算子；它不是用户传入 `*`/`?` 的 GLOB，也不因 DataHub 弃用 `TEXT_PARTIAL` 索引标注而被排除出代数。
- typo tolerance、fuzzy、stemming 的跨 provider 统一语义；
- Facet/total count 作为 SEARCH 返回；若 UI 需要，作为独立 projection capability，并标 exact/approximate。有界 Schema/Catalog **BROWSE**（源卡片 + 类型目录，不是对象 LIST）是另一条产品面，不是本条延期。
- `SEMANTIC_MATCH`、VECTOR、HYBRID 和跨 lane rerank。现有 Refine / RERANK 只评判输入候选，
  不承担新增候选的召回，也不把 semantic 写成第四个 AccessHint。词表映射、查询改写和模型渐进阅读
  可以在消费层组合既有访问能力；它们不要求向量索引。新增候选提供方须另行选择合同，不能暗改本代数。
- aggregate、join、group、graph traversal。

State 查询的 continuation 必须保持同一查询与投影依据。Stream 的窗口、进度表达和事件到当前态的派生，属于 LIVE_MATERIALIZATION.md §8.3 的开放问题；缺少相应能力时失败关闭。

这些边界不得通过改变 `MATCH`、`EQ` 或返回正文的既有含义偷偷加入。

### 5.8 验证入口

本文只冻结查询代数与 RetrievalPlan。MATCH、typed filter、continuation、Candidate hydrate 与 capability/failure 的逐项证据在 `TEST_CATALOG.md`；产品是否可用在 `MVP_ACCEPTANCE.md`。二者都不能反向删除本文已定的查询面。Binding / Observation 形态见 `LIVE_MATERIALIZATION.md`。

Provider 新增 wildcard、semantic、facet、stored payload 或 Stream window 前，必须先扩展公开
能力合同与 Conformance，不能借实现差异改变既有 `MATCH`、`EQ` 或结果 envelope 的含义。

---

## 6. Planning 与路由

```text
ResolvedKnowledgeSet {repository → commit}
  → 读取该 commit 上的 Aspect/Schema/Binding
  → 编译 AccessSpec
  → Retrieval Planner 按 clause Probe capability 与 runtime policy
  → 选择 Snapshot projection、source pushdown 或 managed dynamic projection
  → 分页取得 CandidateRef
  → 按 typed reference hydrate 完整知识与版本
  → residual filter / union / deduplicate / rank
  → 未填满 limit 时继续 candidate page
```

逻辑声明、物理投影和单次计划承担三种不同责任：

| 对象 | 为什么需要独立 |
|---|---|
| AccessSpec | 解释固定知识版本允许查询哪些字段，与物理引擎无关 |
| ProjectionSpec | 描述某个运行提供方如何满足声明；重建或更换引擎不改变业务 Schema |
| RetrievalPlan | 针对本次请求、能力与预算选择路径，保留需回读后判断的条件和完整性依据 |

候选定位和投影维护也要分离：能直接回答源侧查询的提供方不应被迫伪造重建操作；维护托管索引
的提供方则必须承担构建、增量与发布职责。正文回读独立于两者，防止索引载荷取代知识结果。
这一内部 hydrate 接缝允许注入同版本正文缓存：先验证 Candidate repository/basis，再按固定
Repository、commit 与完整读取身份取值；只有未命中的候选子集批量回读 authority。缓存不能接受
Candidate 的 stored payload 当作知识来源，也不跳过 residual 或每次交付授权。完整对象和 Address
的端口由 `knowledge.Hydrator` 拥有，装配由 `SERVICE_ARCHITECTURE.md` 拥有，介质与版本隔离由
`STORE_ADAPTERS.md` 拥有。动态 State 仍从相同 observation basis 的 Serving State 回读。
已选定字段、端口与调用形状由 [Retrieval 合同](../retrieval/README.md)、
[查询类型](../retrieval/searchop.go)、[Provider 端口](../index/engine.go) 拥有。

Catalog 只固定知识仓版本。动态观察的依据由运行方证明、检索方选择并随结果保留，不能把动态
进度塞入 Workspace 定义。它需要解释所用声明、运行代际、来源进度与观察时间，而不是制造
不存在的全局原子快照。

具体源只填写自己能证明的字段，不能用 `observedAt` 冒充 source revision，也不能用单个 watermark
掩盖分区偏序。若上层产品需要跨请求重放，应该显式保存 Retrieval Observation；只有 provider
承诺旧 basis 可重读时，它才是 replay token。动态 cut 不塞回 KnowledgeSet 或 Catalog Registry。

BM25、向量距离、图距离和外部 search score 没有天然共同尺度。Candidate union 只统一 envelope、typed identity 和 evidence，保留 provider、lane、local rank/score、matched fields 与各自 basis。

公开交付需要让调用方同时解释查询范围、完整性、正文版本与候选证据。SearchView 说明本次
检索依据，命中版本说明实际交付内容的依据；来源记录中的源版本承担另一种溯源责任，不能
互相替代。具体结果结构由 [结果类型](../retrieval/result.go) 拥有，正文授权屏蔽由
[权限设计](PERMISSIONS.md) 的交付边界拥有。

因为 residual false positive、去重或授权过滤会消耗候选，执行器必须支持 continuation：持续取 candidate page 直到填满 limit、所有 fragment exhausted 或预算耗尽。预算耗尽且可能仍有命中时返回 partial。候选坐标错误、同 basis 正文缺失或 hydrate I/O 失败必须 fail closed。跨 provider 的稳定 tie-break 至少使用 `(repository, object_id)`，不能拿异构 score 直接当全局概率。

---

## 7. 业界对照

下列小节只解释第 5 节契约为什么成立，不改变 `text/filter/sort` 或查询代数。
派生投影控制、Retriever 作为定位口（而非 RAG 正文口）、以及控制面缺口分析见
[`INGESTION_RETRIEVAL_RESEARCH.md`](INGESTION_RETRIEVAL_RESEARCH.md)；本节不拥有那条对照。

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

上述机制对照支持逐条件能力探测：DataFusion `Inexact` 是可能多返回、上层 residual；本项目另加
`Approximate` 表示可能漏候选。倒排和近似投影漏的项不能靠 residual 补回，结果只能 partial。

因此：

1. 三分访问面仍然正确。DataHub 还在把物理 `fieldType` 收成 TEXT/KEYWORD + `searchTier`；
   拒绝 `stored/summary/key` 与这条线同向。
2. PREFIX 留在 `filter` + string，依据是 ES prefix 与 DataHub `START_WITH`，用来定位前缀。
   不要拿 Dataplex `:` 当 PREFIX 的直接证据。
3. CONTAINS 同样留在 `filter` + string，依据是 Dataplex `:`、DataHub `CONTAIN` 与
   OpenMetadata contains。它覆盖「按名称/列名找对象」这条 MVP 主路径。TEXT_PARTIAL 弃用
   只约束 Schema 标注，不约束查询算子；实现曾经缺这一算子，不能反过来把协议写成延期。
4. 本协议已选定的候选内语义处理对应 Refine / RERANK，不进 `access[]`；它不代表所有语义理解
   或候选扩展路线。模型可以通过查询改写与进一步阅读继续探索，不由重排的边界推出必须使用向量。
5. `NOT`、match-all、Facet 看起来「大家都有」，但不能直接抄进定位原语：前两者依赖封闭
   全集或浏览面；Facet 改的是聚合计数，按独立 projection capability 加。

MVP 选择 `MATCH + typed filter/range + PREFIX + CONTAINS + sort/page`，不是因为底层引擎只能做到
这些，而是它已经覆盖知识发现主路径，同时仍能用明确 capability 向后扩展。Facet 和 typo
tolerance 需要 UI 时可参考 [Algolia Faceting](https://www.algolia.com/doc/guides/managing-results/refine-results/faceting)。

---
