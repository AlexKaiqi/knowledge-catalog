# 动态知识物化与统一检索

日期：2026-08-27
定位：Binding/Observation 与统一检索的语义设计。实现状态只在 `MVP_ACCEPTANCE.md` /
`TEST_CATALOG.md` 维护；State 控制算法见 `PROJECTION_CONTROLLER.md`。

本文解释高频变化的当前态和事件流为什么不属于 Knowledge Catalog 的权威 Store，以及怎样通过版本化 Aspect 句柄进入统一检索。字段形状选定后由 Conformance 钉死；未冻结的 Stream 问题列在 §8.3，不是「不做」。

---

## Goal

说明高频当前态和事件流为什么不属于 Knowledge Catalog 的权威 Store，以及稳定 Aspect 如何通过版本化 Binding 句柄被观察。统一检索代数见 `RETRIEVAL.md`。

## Non-Goals

- 不把外部 runtime 的 checkpoint/WAL 登记成 Knowledge Repository（下文「为什么」）。
- 实时 State/Stream 的运行与存储不属于 Store Adapter（`STORE_ADAPTERS.md`）。
- 本文不维护实现完成度（`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`）；控制算法不在本文（`PROJECTION_CONTROLLER.md`）。
- 不拥有 SEARCH 查询代数（`RETRIEVAL.md`）。
- 不拥有发现/读授权与交付屏蔽（`PERMISSIONS.md`）。

## 硬性约束 / Invariants

- `D-01` Bound State 必须同时标识声明 basis 与 observation basis；Stream 不得隐式数组化（`K-28`）。
- `P-01` 投影失败不得回滚 Canonical commit。
- `V-01` 消费 SEARCH 使用本次解开的 commit，不回绕 live HEAD。
- Catalog 不固定动态 cut（[ADR-022](KNOWLEDGE_CATALOG_DESIGN.md#adr-022)）。

## 选定方案 / 被否决方案

- 选定：[ADR-017](KNOWLEDGE_CATALOG_DESIGN.md#adr-017) / [ADR-022](KNOWLEDGE_CATALOG_DESIGN.md#adr-022) / [ADR-027](KNOWLEDGE_CATALOG_DESIGN.md#adr-027)：② 只保存 Binding/ResourceDescriptor；Serving 经窄端口 hydrate；③ 按 capabilities 编 RetrievalPlan。
- 否决（本文边界）：APPEND Surface；访问默认沉淀为知识。系统级拒绝见 [R-03](KNOWLEDGE_CATALOG_DESIGN.md#r-03)。

## 接口契约 / 状态机

Binding/Observation 语义以本文为准。消费侧需要可注入的 State 读取端口。SEARCH 代数与 RetrievalPlan 见 [`RETRIEVAL.md`](RETRIEVAL.md)。参考实现：`knowledge/serving.StateLookup`、`index.ChangeNotice`。


## 1. 为什么 Snapshot 不能当运行时

ETL 任务展示了一个常见分裂：

- `job_id`、Aspect、Schema 和访问方式长期稳定；
- running/failed/progress 等当前值高频变化；
- 运行记录是不断增长的事件流；
- 用户既要搜“失败的任务”，也要读取某个任务的最新状态或历史窗口。

Git Snapshot 适合保存稳定知识，却不适合作为实时状态和事件流运行时。反过来，只有一个可调用句柄能读取已知资源，却不能让用户发现候选。

这里的“不进入 Snapshot”只指**不进入 Knowledge Catalog 的 Canonical Repository**。流处理器、数据库和时序系统仍会为故障恢复制作 checkpoint、WAL 或内部 snapshot；这些是运行时拥有、按 retention 回收的恢复产物，不是用户可治理的知识版本，不能因为也叫 snapshot 就注册成 Knowledge Repository。

问题因此是：

> Repository 保存什么稳定声明，外部产品承担什么运行语义，Retrieval 又怎样把两者编译成统一发现与回源路径？

---

## 2. 推导

“非稳定”不是一种数据类型，变化频率也不是唯一落位依据。决定载体前先问：谁是权威、读取的是
当前态还是历史、是否要求回放、能冻结什么 basis、允许怎样的 retention/过期。低频的运行状态
仍可能只是一种 observation；高频结果若经过确认并需要治理，也可以显式捕获为 Snapshot 知识。

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

### 2.2 State 与 Stream 是两种访问形态，不是完整介质分类

- State：每个 Address 在一次观察上有 0..1 个当前值。
- Stream：每个 Address 或 ResourceRef 有 0..N 条有序记录。

这是 Binding 对调用方暴露的最小逻辑形态。实际运行系统通常还会按责任继续拆分：当前态 KV/源查询、可回放事件日志、指标/日志/trace 时序存储、流处理 checkpoint，以及面向查询的 materialized view/index。时序观测可以通过 Stream Binding 暴露，checkpoint 不属于 ValueSource，materialized view/index 则属于派生 serving state。

State 与 Stream 可以互相派生：事件 Fold 成当前态，当前态变化可产生 change stream。但这只说明物化代数，不表示它们具有相同 retention、顺序、恢复和查询语义，更不表示底座需要拥有 Stream Store。

### 2.3 每次动态观察仍必须有 basis

把 Stream 搬出底座不会消除 cursor、watermark、source revision、observedAt 和 late-data 语义。它只改变这些语义的责任人：

- Snapshot basis 由 Catalog/Repository 的 commit 保证；
- State/Stream observation basis 由 Materialization Runtime 保证；
- Retrieval 结果必须把两类 basis 分开返回。

没有 observation basis 的“实时”无法复核，也无法解释索引与回源不一致。

不同源能提供的一致性强度不同，上层不能统一伪装成 repeatable read：

- repeatable：源支持 MVCC/as-of、固定 generation 或可冻结的分区高水位；
- bounded：只有单调 revision/watermark，可给出有界 freshness，但未必能重读旧值；
- latest-only：只能读取调用时最新值；跨页和多次 hydrate 只能声明 best-effort。

多个 Binding 通常也没有一个全局原子 cut。除非外部运行时另有协调协议，动态 projection revision 只能标识本次收集到的一组 observations，不能伪装成源系统的“全局实时快照”。SearchView 保存该紧凑 revision；每个命中的 KnowledgeVersion 保存其实际 observation bases，而不是把全库 observations 内联进响应。

### 2.4 索引始终是派生状态

Snapshot、State 和 Stream 的索引都只定位候选。CandidateRef 不携带知识正文；命中后必须通过 typed reference 回到 Snapshot 或固定 Binding 读取完整知识及版本。物理引擎的 stored fields、summary、doc values 或 `_source` 只可作为内部优化，不能成为协议结果。候选已变化、消失或不可按原 basis 重读时返回 `PRECONDITION_FAILED`；partial 只描述已声明的 approximate coverage 或预算耗尽，不能掩盖投影与权威不一致。

### 2.5 Invalidation 不证明完整

通知可能丢失、合并或乱序。动态投影若要声明完整，Materialization Runtime 必须至少提供 delta、enumerate/checkpoint、周期 reconcile 或有界 TTL 中的一组恢复机制。

---

## 3. 绑定与运行时边界

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

### 3.4 Dynamic lane 是 Repository-bound、非 Repository-owned

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

接入方不写 OpenSearch 等物理索引。它只声明访问能力并报告 Binding、Address、source identity 或 scope 的变化。

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

clause 级 Probe（Exact / Superset / Approximate / Unsupported）与 completeness 由 [`RETRIEVAL.md`](RETRIEVAL.md) 拥有。coverage 与 freshness 不能从 guarantee、索引存在或 invalidation 成功推断。

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

## 5. State exact READ 与托管物化

SEARCH 代数、AccessSpec 与 RetrievalPlan 见 [`RETRIEVAL.md`](RETRIEVAL.md)。本文只拥有 Binding / Observation 与 Serving State。

消费侧精确 READ 必须先在固定 Workspace commit 解析 Binding，再由注入的窄 `StateLookup`
端口返回 value + `ObservationBasis`。没有 runtime、observation basis 不合法或遇到不支持的
Stream 访问时失败关闭；具体 Binding adapter、Serving State 和源 runtime 不属于仓库根核心。

Snapshot SEARCH 命中后的正文回读必须复用同一 State hydrate，不能让 `READ` 与 `SEARCH`
对同一 basis 分别返回 live 值和占位。动态字段的候选发现、`SearchView`、continuation 见 [`RETRIEVAL.md`](RETRIEVAL.md)；同 revision hydrate 的控制算法由 `PROJECTION_CONTROLLER.md` 拥有；当前支持范围只查
`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`。

动态 Binding 的源访问放在下层 Materialization Runtime；Controller/Index 只看已经解析、校验和规范化的
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

Serving State 是某个 generation/basis 上可重读的完整观察物化，不自动成为业务源权威，也不是
Knowledge Catalog Canonical。它可以由源查询、事件 Fold 或 CDC 构建，并应能按自身恢复策略
重建；“可用于一致 hydrate”与“拥有知识真相”是两件事。

单个任务或 Address 变化只刷新一个 key。以下情况才全量重建动态投影：Binding generation、
Schema/AccessSpec、解析算法或 physical revision 改变；checkpoint 断档；reconcile 发现无法安全
增量修复；首次建立投影。

invalidation 只是低延迟提示，不能证明完整。一个宣称 complete 的动态投影必须至少具有：

```text
invalidate → lookup 的实时路径
+ delta(since checkpoint) 或 enumerate/checkpoint 的恢复路径
+ 周期 reconcile 或有界 TTL 的兜底
```

上层首版只应冻结 State 当前态，不冻结 Stream event/window 查询。跨 Serving State 与物理索引无法
原子提交时，controller 必须先写 basis-addressable Serving State 和投影，再切换 active observation basis；查询
发现 Candidate basis 与 State basis 不一致时返回 `PRECONDITION_FAILED`，不能拼接两个版本或降级为 partial。


---

## 7. 调研结论

这套方向既能在成熟基础设施和数据目录产品中找到对应模式，也有流关系、增量视图、联邦查询与
分布式检索的理论基础。业界没有把所有“非稳定信息”收进一种通用 Store；共同做法是按权威、
查询、恢复和 retention 责任拆开，再用稳定 identity、schema 和 basis 连接。

### 7.1 产品与基础设施模式

| 系统 | 处理方式 | 对本项目的启示 |
|---|---|---|
| [Kubernetes Objects](https://kubernetes.io/docs/concepts/overview/working-with-objects/) / [API watch](https://kubernetes.io/docs/reference/using-api/api-concepts/) | `spec` 表达声明态，`status` 表达控制器观察的当前态；客户端 list 后从 `resourceVersion` watch，旧 revision 被压缩后必须重新 list | 声明与观察分开；通知历史有限，完整恢复依赖 enumerate/reconcile |
| [Kubernetes Events](https://kubernetes.io/docs/reference/kubernetes-api/events/) | Event 有限保留，官方定义为 informative、best-effort、supplemental | 运行事件不能自动充当审计或知识真相 |
| [Apache Kafka Log Compaction](https://kafka.apache.org/43/design/design/) | 事件按 partition/offset 排序并按时间/大小保留；compaction 保留每个 key 的最后状态，但旧记录和 tombstone 仍会清理 | Event Log、Current State 与永久审计是三种不同承诺 |
| [Apache Flink Checkpointing](https://nightlies.apache.org/flink/flink-docs-stable/docs/dev/datastream/fault-tolerance/checkpointing/) / [Checkpoint vs Savepoint](https://nightlies.apache.org/flink/flink-docs-stable/docs/ops/state/checkpoints_vs_savepoints/) | checkpoint 将 operator state 与输入位置一起保存用于故障恢复；savepoint 才是用户管理、面向迁移的运行状态镜像 | 动态运行时可以做 snapshot，但其所有权、生命周期和语义不等于 Knowledge Snapshot |
| [Materialize Views](https://materialize.com/docs/concepts/views/) | materialized view 持久并增量更新结果，index 在集群内存中服务查询 | Serving State 与查询索引分层；索引可丢，物化状态按自己的恢复模型管理 |
| [Prometheus Storage](https://prometheus.io/docs/prometheus/latest/storage/) | 指标按时间块、WAL 和 retention 管理，可转发到远端时序存储 | metric/health/lag 属于时序可观测数据，不应高频 COMMIT 成知识 |
| [DataHub Metadata Model](https://github.com/datahub-project/datahub/blob/master/docs/modeling/metadata-model.md) | Versioned Aspect 存关系库；高频 Timeseries Aspect 直接进 Kafka/Elasticsearch，且文档明确提示其灾备恢复更困难 | 数据目录也拆稳定与时序 lane；但搜索索引不应因此被提升为 Canonical |

横向结论：

1. durable 不等于 Canonical。Kafka log、Flink checkpoint、TSDB block 都可以持久，但不因此成为知识仓；
2. 当前态、事件历史、运行恢复和可观测数据应分别声明 retention、顺序和恢复承诺；
3. subscribe/watch/invalidation 只降低延迟，正确性仍依赖 revision、relist/delta、checkpoint 和 reconcile；
4. 每次动态读取必须返回能解释 freshness、重读能力和分区进度的 basis；无法证明就降级为 bounded/latest-only/partial；
5. 动态观察只有经明确选择、汇总和 provenance 捕获后，才由 Collector COMMIT 晋升为知识。

### 7.2 数据集成与能力驱动改写

- Halevy 的 [Theory of Answering Queries Using Views](https://homepages.inf.ed.ac.uk/libkin/dbtheory/alon.pdf) 说明怎样用已声明的 view/source 回答统一查询。
- IBM Garlic 的 [Capabilities-Based Query Rewriting](https://research.ibm.com/publications/capabilities-based-query-rewriting-in-mediator-systems) 让源声明能力，由 mediator 生成可执行子查询和组合计划。
- Garlic/DB2 的 [Federated Query Processing](https://research.ibm.com/publications/garlic-a-new-flavor-of-federated-query-processing-for-db2) 证明 Catalog 元数据、源模块与联邦优化器可以协同。
- Li 的 [Limited Access Patterns](https://ics.uci.edu/~chenli/pub/jvldb03.pdf) 说明只有受限 lookup 时，并非所有查询都能得到完整答案。

结论：capability checking、pushdown 和 cost-based routing 很成熟；任意外部协议与任意查询的完全自动改写不成立。

### 7.3 Stream–Relation 与增量视图

- [CQL](https://web.stanford.edu/class/cs245/readings/cql.pdf) 区分 stream、relation、window 和 relation-to-stream 输出。
- [DBToaster](https://vldb.org/pvldb/vol5/p968_yanifahmad_vldb2012.pdf) 研究高频动态视图的高阶 delta 维护。
- [Differential Dataflow](https://www.cidrdb.org/cidr2013/Papers/CIDR13_Paper111.pdf) 用逻辑时间维护变化中的计算。
- [DBSP](https://www.vldb.org/pvldb/vol16/p1601-budiu.pdf) 用 differentiation/integration 给出通用 IVM 代数。

结论：Stream、当前态、window/cut 和增量维护有成熟理论；这支持上层产品实现 Materialization Runtime，不构成把 Stream 塞进 Repository 的理由。

### 7.4 联邦检索

[Searching Distributed Collections with Inference Networks](https://sigir.org/wp-content/uploads/2017/06/p160.pdf) 已把分布式检索拆成 collection representation、selection 和 result merging。

结论：自动选源与候选合并成熟；异构相关性分数仍不天然可比。

### 7.5 成熟度判断

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

---

## 8. 已定边界与待冻结问题

### 8.1 当前底座 MVP 裁决

当前已冻结**声明交接 + State 精确观察**，没有借此引入运行宿主：

- `ResolvedBinding` 已返回 `repository + declarationCommit + Address + declarationDigest`；引用
  ResourceDescriptor 时另返回 `descriptorDigest`。这比含糊的 `repositoryCommit + bindingDigest`
  更完整，已经足够让墙外运行时确定本次使用的固定声明；
- `resolve-binding` 只解析声明，不调用 runtime；`knowledge/serving` 的精确 READ 才经注入的
  `StateLookup` hydrate，并同时返回 declaration basis 与 observation basis；
- 普通 READ 不执行 Stream Binding；底座没有 `APPEND`、StreamStore、window 或动态 SEARCH 正路径；
- 动态观察要晋升为知识时，Collector 复用现有 Writer COMMIT 与 `ProvenanceEnvelope`。可重读的
  provider basis 当前以稳定 `sourceRefs`/`evidenceRefs` 保存；只有一次 latest-only 观察时用
  `OBSERVATION + producedAt` 如实表达，不能声称可重放；
- `ObservationBasis` 与 `UnitObservation` 已有 Go 类型和 Conformance；它们只表达一次 State exact
  read 能证明的 generation/consistency/source revision/watermark/observedAt，不预造 StreamCut、
  动态 continuation、freshness/lag/coverage 等尚无人兑现的协议。

State managed projection 与跨页动态检索已经由 `PROJECTION_CONTROLLER.md` 冻结为下一阶段能力。
下面条件只约束具体源 provider 和后续 Stream API，不再阻止 State Controller 的参考实现：

1. runtime/provider 能执行真实 State lookup 或 Stream window；
2. provider 能给出真实 generation/source revision，并区分 repeatable、bounded、latest-only；
3. Stream 调用方确实需要 window、重放或事件 continuation；
4. 有对应 Conformance 验证 basis mismatch fail closed、latest-only 分页、gap/reconcile 和可证明的 partial。

届时最小结果 envelope 才需要包含 declaration basis、provider 自己可证明的 observation basis、
`observedAt/watermark` 以及 freshness/lag/coverage/partial claims；多 Binding 保存各自 cut，不承诺
不存在的全局原子 cut。subscribe/invalidation 仍只是提速路径，完整性必须来自 delta、enumerate、
reconcile 或有界 TTL。

### 8.2 已定边界

已定：

- 底座的权威 Store 只有 Snapshot；
- “底座不存动态值”不禁止外部运行时制作 checkpoint/WAL；运行恢复 snapshot 不是 Knowledge Snapshot；
- Aspect 可声明 Snapshot、State Binding 或 Stream Binding；
- State/Stream 是 Binding 的逻辑访问形态，不是物理介质分类；当前态、事件日志、时序观测、checkpoint 与查询投影仍分别治理；
- `value_source` 写在 Address 单元 frontmatter；Snapshot 为默认，Binding 有独立 DeclarationDigest；
- inline Binding 与同 commit 的 ResourceDescriptor reference 同时支持，且二者互斥；
- Binding 声明属于 ②，运行值属于墙外 Materialization Runtime；
- 消费侧 `knowledge/serving` 可以通过注入端口看到 State 运行值，但不拥有 runtime/provider；
- Catalog 只固定 Repository commit，不固定动态 cursor/watermark；
- Writer 不提供 APPEND；动态值若要沉淀，Collector 显式 COMMIT Snapshot；
- Stream Schema 描述单条记录，不是数组；
- 接入方通知变化，平台按 Binding 拉取；
- Projection 非权威，命中后按 typed reference hydrate；
- Schema 只声明 `text/filter/sort`；SEARCH 代数、Probe、RetrievalPlan 见 `RETRIEVAL.md`；
- State 动态首版采用 invalidate-and-pull + basis-addressable Serving State + 动态 State 投影；控制语义与验收见 `PROJECTION_CONTROLLER.md`，不进入 Repository/Writer/Catalog；
- 调用方信封是否含全文见 `PERMISSIONS.md` 交付链首段。

### 8.3 Stream、容错与规模待冻结

以下问题不在 State 首版中预先造类型；有真实 Stream、容错或规模需求后再冻结：

1. 旧 Repository commit 对已经下线的旧 runtime generation 的可用性；
2. ObservationCut 对分区水位、cursor/window 和部分序的具体表达；
3. Stream retention、tombstone、compaction 与 gap 后 relist/reset 的责任协议；
4. Stream Event Projection 与 Current-State Fold 的声明位置；
5. passthrough 与 managed projection 的成本、authority role 和 retention 模型；
6. stale/removed 在流式事件和 retryable error 之间的具体编码；
7. 多副本 controller、worker lease、durable queue 与历史动态 revision retention。
