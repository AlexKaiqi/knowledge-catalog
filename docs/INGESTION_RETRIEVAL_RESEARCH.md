# 派生投影控制与 Retriever 业界对照

核对日期：2026-09-18。本文保存业界机制核对、名词映射与架构推论，不是新增协议或实现排期。
调研采用官方文档与部分公开源码，并对照本仓已选定设计；没有部署对照项目的生产集群，
也没有取得本项目的检索质量或容量结果。外部链接指向核对时的内容，采用前应固定具体版本复核。

查询代数为什么是 MATCH + typed filter，仍由 [检索设计](RETRIEVAL.md) §7 拥有。
模型如何探索知识，由 [知识探索调研](KNOWLEDGE_EXPLORATION_RESEARCH.md) 拥有。
本文只回答：业界把「ingestion」和「retriever」做成了什么，和本仓面 3 / Retriever 是否同类，以及在不改协议的前提下哪些问题仍值得先完善。

## Goal

对照目录产品、表引擎 SPI 与控制面实践，澄清两件事：

1. 本仓称为 ingestion control 的运行面，是 published HEAD 的派生对账，不是采集写面；
2. 本仓 Retriever 只定位候选，不是 RAG 框架里带正文的 Retriever。

在此基础上记录业界机制对本仓已选定结构的支持，以及仍开放、需要回到既有 owner 裁决或登记缺口的问题。

## Non-Goals

- 不拥有投影控制算法；由 [投影控制](PROJECTION_CONTROLLER.md) 拥有。
- 不拥有 SEARCH 代数、AccessSpec 与 RetrievalPlan；由 [检索设计](RETRIEVAL.md) 拥有。
- 不拥有 Collector / Observer / Resource Access；由 [外部资源访问](CONNECTORS.md) 拥有。
- 不拥有 Binding、observation 与时效语义；由 [动态物化](LIVE_MATERIALIZATION.md) 拥有。
- 不拥有发现/正文授权；由 [权限设计](PERMISSIONS.md) 拥有。
- 不把目录产品、Agent 框架或查询引擎的能力表复制成本仓协议；不把通用知识底座改成检索应用。
- 不维护实现台账，也不把研究建议顺序写成协议边界。产品缺口仍由 [MVP 验收](MVP_ACCEPTANCE.md) 拥有。

## 硬性约束 / Invariants

本研究沿用已有不变量，不新增协议不变量 ID：

- `W-01`：写面只有 COMMIT/PROPOSAL；采集输出仍走 Writer，不能在面 3 另开写口。
- `P-01`：投影可删、可重建；失败不回滚 Canonical。
- `PC-01`：派生消费者独立对账 published HEAD；静态 Snapshot 与动态 observation 不混 basis。
- `C-01` / `R-01` / `R-02`：Retriever 只返回候选；SEARCH/RELATIONS 在 exact-basis 上 hydrate；无 READY 失败关闭。
- `V-01`：消费使用本次解开的 commit，不回绕 live HEAD。
- `S-01`：Schema 只声明 `text/filter/sort`；能力由本次 Probe 证明，不由「支持搜索」开关宣称。

这些约束不禁止参考业界的 reconcile、generation 发布或 query pushdown；它们禁止把那些机制解释成新的权威层或新的写面。

## 选定方案 / 被否决方案

**研究取向：**把业界「ingestion」映射到 Collector → Writer，把面 3 映射到 level-based reconcile；把本仓 Retriever 映射到表引擎 SPI 与「搜到身份再回权威读」，而不是 RAG Document 口。研究结论用来解释已选定分层，不批准新 API。

**不采纳的推论：**

- 不从 DataHub / OpenMetadata 产品名叫 ingestion，推出本仓面 3 应接收 ChangeSet。
- 不从 LlamaIndex 的 Retriever 返回带正文节点，推出本仓 SEARCH 应交付索引 `_source`。
- 不从目录 UI 普遍有 Facet、`*` browse、VECTOR，推出必须扩 SEARCH 代数。
- 不从 Kafka 可靠日志或 ES ingest pipeline，推出 AfterSnapshot 必须变成写事务的一环。
- 不从「适配器曾有过完整性谎言」，推出当前 OpenSearch 主线仍未修；那一轮已由探索调研与 `RETRIEVAL-01` 收口。

**仍开放：**查询覆盖与时效是否同一句 complete（`TASK.md` REVIEW-04）；检索是按仓可达性过滤还是搜宽读严（REVIEW-05）；单对象编译失败时整仓不可搜还是隔离后标 coverage。本文给出业界旁证，不代替 owner 裁决。

## 接口契约 / 状态机

本文不定义新字段、错误码、命令或状态机。现有缝以公开类型与包 README 为准：

- 候选定位与维护端口：[`index.Retriever`](../index/engine.go)、[`ProjectionMaintainer`](../index/engine.go)；
- 派生对账：[`index` 控制器合同](../index/README.md)、`ChangeNotice`、`SnapshotConsumer`；
- 查询代数与结果：[`retrieval`](../retrieval/README.md)；
- 正文回读：`knowledge.Hydrator`，装配见 [服务架构](SERVICE_ARCHITECTURE.md)；
- 采集与通知：[`CONNECTORS.md`](CONNECTORS.md)。

研究中的「ingestion control」只是运行面 3 的对照名，不成为公开产品名词；公开叙述继续用投影控制、Collector、Observer、Retriever。若现有入口不足以支持某类业界能力，应记录缺口并回到相应 owner，不能暗改已有端口。

---

## 1. 先对齐名字

业界同一词经常指三件不同的事。不先拆开，对照会把本仓已经否决的流水线重新引进来。

| 业界常说的词 | 来源机制 | 本仓对应 | 不是 |
|---|---|---|---|
| ingestion / recipe / connector | 从源抽出元数据并写入目录服务 | Collector → Writer → Snapshot | 面 3 |
| search index consumer / reindex | 权威提交之后更新搜索或图索引 | 投影控制（面 3）→ `ProjectionMaintainer` | 写事务的下一站 |
| Retriever（RAG） | 返回带正文的 Node/Document | 无直接对应；正文走 ② hydrate | `index.Retriever` |
| TableProvider / Connector SPI | 对谓词声明 Exact/Inexact 并保留 residual | `Retriever.Probe` + RetrievalPlan | 「此引擎支持搜索」 |

本仓整理稿把运行面写成 1 写治理、2 Snapshot、3 派生订阅、4 发现投影、5 Access。编号 3、4 是运行面，不是协议层 ⓪–③。协议层 ③ 同时包含检索代数、Retriever 与投影维护；面 3 只拥有「谁在何时 Rebuild/Apply」。本文沿用这个区分，不把运行面升格成新的协议层。

**机制：** DataHub、OpenMetadata、Amundsen 的用户文档都把从源到目录的作业叫 ingestion。
**推论：** 那是写面产品名。本仓若把面 3 也叫 ingestion，只适合对内对照，不适合对外替换 Collector。

---

## 2. 面 3：派生订阅者，不是采集写面

### 2.1 Kubernetes：正确性靠对账，不靠把通知做可靠

[Kubernetes controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 把对象 `spec` 当 desired，持续把实际状态推近它。[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/reconcile/reconcile.go) 写明 reconcile 是 **level-based**：事件只入队一个名字，Reconcile 重新读取当前状态；事件内容本身不是真相。因此通知可丢、可重复，控制器仍须幂等。

[Kubebuilder](https://book.kubebuilder.io/reference/good-practices.html) 另要求用 Conditions 表达复杂状态，让使用者和工具能读懂「未就绪 / 落后 / 失败」，而不是只看见对象存在。

**对本仓：** AfterSnapshot 可丢、Desire(HEAD)、`published HEAD ≟ provider READY` 与这条线同向。产品上仍缺的是 Conditions 那种**查询范围级**追平说明，而不是再做一条可靠消息总线。

### 2.2 Iceberg：CAS 结束写入，读者钉住快照

[Apache Iceberg spec](https://iceberg.apache.org/spec/)：表状态在 metadata 文件里；提交用原子替换；读者使用加载当时的 snapshot，直到主动 refresh。写入成功不等于下游物化视图已经追上，只表示该 snapshot 已成为可钉住的权威。

**对本仓：** Snapshot CAS 结束写入，投影失败不回滚权威，消费钉住本次 pin。Iceberg 的「当前 snapshot」对外可见；本仓 live 投影是否追上 published HEAD，还需要同样可引用的 READY/lag，而不能只存在于控制器内部 `Meta`。

### 2.3 DataHub：权威提交后才改搜索，但产品仍把前半段叫 ingestion

DataHub 的权威写是 Metadata Change Proposal → Metadata Service 提交；提交后发 [Metadata Change Log](https://docs.datahub.com/docs/architecture/metadata-serving)，由 [mae-consumer-job](https://docs.datahub.com/docs/metadata-jobs/mae-consumer-job) 应用到 graph 与 search index。官方说明：该消费者「把 metadata graph 的变更转换成对次级搜索/图索引的更新」。主路径读走 document store，全文与二级索引走 search index。MCL 按 entity URN 分区，保证同一实体有序。

**机制：** 搜索索引是权威提交之后的派生消费者。
**推论：** 派生关系与本仓面 3 同类。差别是 DataHub 用 Kafka 日志作为变更输送，本仓用可丢通知 + HEAD 对账；本仓不能据此要求把 AfterSnapshot 升级成写事务。DataHub 的「ingestion recipe」仍然是 MCP 写面，对应 Collector，不对应 mae-consumer。

[GMA search index](https://docs.datahub.com/docs/what/search-index) 还承诺部分更新和零停机换索引。这对应本仓 `IX-03` 暖 rebuild：旧 READY generation 在 Publish 前继续服务。

### 2.4 OpenMetadata：ingestion 写服务 API，搜索由服务侧维护

[OpenMetadata metadata ingestion](https://docs.open-metadata.org/v1.13.x/developers/contribute/codebase-deep-dives/metadata-ingestion) 是 Source → `MetadataRestSink`，对服务做 PUT/PATCH。[code layout](https://docs.open-metadata.org/v1.13.x/developers/architecture/code-layout) 写明服务把实体变更记为 events，再由搜索索引处理器更新 Elasticsearch。历史上曾有 ingestion 直接以 ES 为 sink 的 reindex 作业；上游已把这条路径收成服务侧 reindex API。

**机制：** 采集作业不再直写搜索。
**推论：** 与本仓「Collector / Observer / runtime 都不直写 OpenSearch」同向。OpenMetadata 的 ingestion 一词仍指写服务，不是投影控制器。

### 2.5 Amundsen：先写图，再从权威抽搜索

[Amundsen Databuilder](https://github.com/amundsen-io/amundsendatabuilder/) 明确分步：先把元数据发到 Neo4j，再 `Neo4jSearchDataExtractor` + `ElasticsearchPublisher` 更新搜索。Publisher 建新 index，再用 alias 原子切换，之后删旧 index。这是 **generation 发布**，不是与 Neo4j 同事务双写。

**机制：** 搜索以图里已有数据为输入，用 alias 切换 generation。
**推论：** alias 切换支持 `IX-03`。不要把「两个 datastore 都写了」理解成本仓应让 Writer 同步写索引；Amundsen 的第二步已经是派生作业。本仓比它多的是：派生作业由平台控制器对账 HEAD，而不是由 Airflow DAG 顺序约定。

### 2.6 Elasticsearch ingest pipeline：写路径上的变换，不是派生控制面

[Elasticsearch ingest pipelines](https://www.elastic.co/docs/manage-data/ingest/transform-enrich/ingest-pipelines) 在文档进入索引前变换 `_source`。它属于搜索引擎的写路径，成功意味着「索引里有了」，失败会回压或进 dead letter。

**对本仓：** 这是明确反例。选定结构是 CAS 结束写入；投影失败不回压权威。面 3 若做成 ingest pipeline，会把「4 里有了」做成发布成功。

### 2.7 小结

| 实践 | 权威何时结束 | 搜索如何追上 | 对本仓 |
|---|---|---|---|
| Kubernetes reconcile | API 对象已持久化 | 控制器读 desired，收敛 actual | 对账模型 |
| Iceberg snapshot | catalog CAS | 读者钉 snapshot；派生作业另做 | 写与读隔离 |
| DataHub mae-consumer | GMS commit + MCL | 次级索引消费者 | 派生消费者；输送用日志 |
| OpenMetadata SearchIndexHandler | 服务 API 写库 | 服务内事件更新 ES | 采集不直写 ES |
| Amundsen ES publisher | Neo4j 已有数据 | 另作业 + alias | generation 发布 |
| ES ingest pipeline | 文档进索引 | 与写入同一路径 | 否决模板 |

本仓已经选定的是第一行加 Iceberg 式 CAS，而不是最后一行。与 DataHub 的主要产品差距不在「要不要派生索引」，而在：**追平是否对外可判定**，以及动态观察是否另有一条不移动 HEAD 的 lane。

---

## 3. Retriever：定位口，不是正文口

### 3.1 表引擎 SPI：按谓词声明能力，而不是按引擎自称

[DataFusion TableProvider](https://datafusion.apache.org/library-user-guide/custom-table-providers.html) 对每个 filter 返回：

- `Exact`：源保证不会产出使谓词为假的行，上层不再套 Filter；
- `Inexact`：源能缩小数据，但仍可能多返回，上层必须 residual；
- `Unsupported`：源忽略该谓词，完全由上层处理。

[Trino Connector SPI](https://trino.io/docs/current/develop/connectors.html) 按调用返回剩余条件与 guarantee，而不是一次声明「此 connector 支持过滤」。

**对本仓：** `Probe` 的 Exact / Superset / Unsupported 直接沿这条线。本仓另加 `Approximate`：可能漏候选，不能靠 residual 补回，结果只能 partial。这比 DataFusion 更严，因为倒排召回的漏项与「多返回」不是同一类错误。

Source-side Retriever 只实现定位、不实现 `ProjectionMaintainer`，对应「能下推的源不必伪造 rebuild」。这是 [ADR-026](KNOWLEDGE_CATALOG_DESIGN.md#adr-026) 已选定的。接这类提供方之前，Superset 的 MATCH 补判必须能证明与分析语义一致；否则会得到最差的 connector：自称 Inexact，上层用字符串包含补错。

### 3.2 DataHub：搜索命中再解析实体

DataHub GraphQL 的 [`SearchResult.entity`](https://github.com/datahub-project/datahub/blob/master/datahub-graphql-core/src/main/resources/search.graphql) 是「与查询匹配的、已解析的 Metadata Entity」，不是把 ES 文档原样交给调用方。主路径读仍路由到 document store。搜索性能讨论把 entity hydration 当作独立成本。

**机制：** 发现走搜索索引，正文/实体走权威存储。
**推论：** 这是目录产品里最接近 `CandidateRef` + hydrate 的实现。本仓更硬的一点是 Candidate 不得携带公开正文，stored fields 不能代替 Canonical；DataHub 的搜索文档模型仍服务排序、facet 与 matched fields，实体再 hydrate。

### 3.3 LlamaIndex Retriever：返回带正文的节点

[LlamaIndex `BaseRetriever.retrieve`](https://github.com/run-llama/llama_index/blob/main/llama-index-core/llama_index/core/base/base_retriever.py) 返回 `List[NodeWithScore]`；节点含 `get_content()` 正文。官方 [from-scratch retriever](https://developers.llamaindex.ai/python/examples/low_level/retrieval/) 示例同样把向量命中的 node 文本交给后续合成。

**对本仓：** 这是产品名冲突，不是可抄的端口。本仓 Retriever 若返回正文，会直接违反 `C-01`。RAG 合成若需要正文，应在 hydrate 与交付链之后进行，不能让索引载荷冒充知识。

### 3.4 目录查询面：两车道，不是通用 SQL

MATCH + typed filter + PREFIX/CONTAINS 的目录覆盖，以及为何不把 `*`、假 NOT、Facet 写进定位原语，已在 [检索设计](RETRIEVAL.md) §7 核对 Dataplex、DataHub、OpenMetadata、Purview、Unity Catalog 与 OpenSearch。本文不重复那张表。

需要单独记下的 Retriever 层含义：

- 有界 BROWSE（源卡片 + 类型目录）是产品面，不是 SEARCH 空扫描；owner 是 [知识产品](KNOWLEDGE_PRODUCT_AND_SCHEMA.md)。
- Facet/total count 若需要，是独立 projection capability，并标 exact/approximate，不是本代数缺项。
- 异构 score 不归一成全局概率；联邦只统一 identity 与 evidence。

### 3.5 OpenSearch 默认允许部分结果

[OpenSearch Search API](https://docs.opensearch.org/latest/api-reference/search-apis/search/) 默认 `allow_partial_search_results=true`；超时或分片失败时仍可能 HTTP 200 并带 hits。[集群设置](https://docs.opensearch.org/latest/install-and-configure/configuring-opensearch/search-settings/) 同样默认允许部分结果。另有 [超时分片仍计为 successful](https://github.com/opensearch-project/OpenSearch/issues/21087) 的观测缺口。

**机制：** HTTP 成功 ≠ 检索完整。
**推论：** Probe=Exact 的提供方必须显式拒绝部分搜索。本仓 OpenSearch 主线已带 `allow_partial_search_results=false`，并在 `RETRIEVAL-01` 收口完整性传播；新的 source-side Retriever 不得回到默认部分成功。

[Elasticsearch Bulk `refresh`](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-bulk) 写明：refresh 只影响本请求触及的分片。增量分批若只等最后一批，就会在未刷新分片上提前宣布 READY。本仓增量路径已按批次 `refresh=wait_for`；这是控制面正确性，不是检索语法。

---

## 4. 已选定结构：业界实际在支持什么

下列不是新决定，而是对照后认为应当保住的分层。

1. **写在 CAS 结束。** Iceberg、DataHub GMS commit、OpenMetadata REST sink 都把权威提交与搜索更新分开。把两者做成一次写事务，会让投影失败回压发布。
2. **面 3 是 level-based reconcile。** Kubernetes 证明通知可丢；正确性是 desired ≟ actual。本仓 desired 是 published HEAD（静态）或已 pull 的 observation（动态），actual 是 provider READY。
3. **Retriever 与 Maintainer 拆开。** Trino/DataFusion 的源不必实现 rebuild；DataHub mae-consumer 才是维护者。只读下推的 Binding 不应伪造 `Rebuild/Apply`。
4. **Candidate 再 hydrate。** DataHub 搜索命中后解析实体；本仓把这条做成不变量，并禁止 stored payload 穿透。
5. **两条 lane 不共用 READY。** 目录产品通常只有「元数据已摄入」一条进度。本仓静态 commit 与动态 observation 分钥，避免行级 State 更新移动知识 HEAD。这是相对 DataHub/OpenMetadata 的加强，不是遗漏。
6. **查询代数保持有界发现。** 目录主路径已是 keyword + typed filter；VECTOR、Facet、空扫描属于别的产品面或独立 capability。

LlamaIndex Retriever、ES ingest pipeline、Amundsen 由 DAG 约定的双存储写入，都不应回写成协议模板。

---

## 5. 仍值得完善的问题

实现是否落后于设计，以 [MVP 验收](MVP_ACCEPTANCE.md) 与 [验证目录](TEST_CATALOG.md) 为准。本节只说明**为什么**这些问题在业界对照下仍然重要，以及该回哪个 owner。不把「尚未实现」写成 Non-Goal。

### 5.1 追平要对使用者和查询可见

Kubernetes 用 Conditions，Iceberg 用 current snapshot id，DataHub 至少能区分主存储读写与搜索索引是否已应用 MCL。本仓控制器内部已有 `Meta.State` / coverage，但产品面仍缺查询范围级的 READY/lag claims。`MVP_ACCEPTANCE.md` 已登记 README 热状态与 discovery 关闸缺这一项。

**机制：** 派生状态若不对外，调用方只能把「不可检索」理解成「知识不存在」。
**推论：** 完善点在投影控制的对外承诺与知识产品的发现关闸，不在新增 SEARCH 算子。消费请求仍不得同步 build。

### 5.2 complete 不自动等于「足够新」

Prometheus 把查询覆盖和 scrape/lag 分开；Kafka 把 consumer lag 与 topic 完整性分开；Iceberg 的 complete 相对某个 snapshot。`LIVE_MATERIALIZATION.md` §6 已要求覆盖与时效不能互相推出。`TASK.md` REVIEW-04 的反例是：通知丢失时，旧观察上的 complete 零命中会被当成「当前没有失败服务」。

**研究旁证倾向：** 先把查询覆盖与时效分成两个维度（选项 A），要求「当前」的调用显式选择时效承诺。这不是裁决。DYN-01 即使跑通真实 Observer，也不能在未裁 REVIEW-04 时宣称实时完整。

### 5.3 动态 lane 需要部署证明，不需要新协议

notice-and-pull、双 basis、消费不得 `RefreshState` 已在投影控制合同里。缺的是独立 Observer、runtime、权威仓、KC 重启续追的整条证据（`TASK.md` DYN-01 / 投影控制 §11.3）。业界对应 DataHub mae-consumer 与源 CDC 的联调，而不是再设计一条写口。

### 5.4 搜宽读严会泄露存在性

Purview / Unity / DataHub 的检索路径通常在查询时就考虑权限；DataHub Cloud 甚至有 search access-control pushdown。本仓现行是候选定位较宽、hydrate 之后再按仓读权遮罩。`TASK.md` REVIEW-05 已写出：对象 ID / 仓 / commit 会成为存在性泄露，且 `LIMIT` 未定义数的是壳还是可见正文。

**研究旁证倾向：** 仓级可达性过滤放在候选定位之前（选项 A），交付链仍只改正文。这与「索引不按 principal 复制」不冲突：过滤的是本次请求的成员集，不是为每个用户建一份投影。

### 5.5 Source-side Retriever 的前置是补判诚实，不是多一个引擎

端口已允许只实现 Retriever。当前 planner 以托管 OpenSearch 为主路径。业界 Trino 的代价是每个 connector 必须如实报告 residual。本仓若在 MATCH 补判不能证明分析语义时接入外部源，会把 Approximate 漏项和 Superset 多返回混在一起。

**推论：** 先有整棵 All/Any 与 MATCH 的可证明 residual，再接第一个 source-side 提供方。不要用「业界都有下推」跳过这一步。CACHE-01 的同版本正文缓存是 Retriever 之上的派生消费者，不能实现成第二个 Retriever。

### 5.6 单对象失败策略还没有业界默认值可抄

ES ingest 有 dead letter；DataHub 失败 MCP 可进失败主题；Kubernetes 可以让单个 Pod 失败而不删除 Job 对象。本仓 lookup 失败不发布新 revision，偏保守，也尚未写清：一个坏 Aspect 是挡住整仓 READY、标 coverage&lt;1 继续服务，还是隔离该 object。

**推论：** 这是投影控制的运行策略问题，规模到来之前就需要显式选择，否则第一次脏数据会把「可丢投影」变成「全仓不可搜」。不在本文选定。

### 5.7 不建议用本调研推动的方向

这些在目录或 RAG 里很常见，但会改错层或抢先未冻结的合同：

- VECTOR / HYBRID / 跨 lane rerank：探索调研已说明语义理解不要求向量索引；
- Facet 作为 SEARCH 返回：独立 capability，不是定位原语；
- Stream 窗口查询：`LIVE_MATERIALIZATION.md` §8.3 仍开放；
- 多实例 worker lease、成本优化器：规模与部署选择，不是本两层的协议缺口；
- 把 AfterSnapshot 换成 Kafka 以「做可靠」：与 Kubernetes level-based 以及本仓可丢通知选择相反。

---

## 6. 建议的完善顺序

下表是研究建议：先让「追平」和「查全」可证明，再扩展提供方。它不是 TASK 排期，也不改变各 owner 的待办编号。

| 顺序 | 问题 | 回到谁 | 业界旁证 |
|---|---|---|---|
| 1 | 查询范围级 READY/lag claims | 投影控制 + 知识产品 + MVP 缺口 | Kubernetes Conditions；Iceberg snapshot id |
| 2 | complete 与时效分列或合并 | REVIEW-04；动态物化 §6 | 覆盖 ≠ lag |
| 3 | 真实 Observer 闭环与重启续追 | DYN-01 | mae-consumer 联调，不是新写面 |
| 4 | 候选定位与仓可达性 | REVIEW-05 | 目录产品查询时裁权 |
| 5 | MATCH/All/Any residual 证明后的 source-side Retriever | 检索设计 + 投影控制 | DataFusion Inexact 必须真 residual |
| 6 | 单对象失败 / coverage 政策 | 投影控制 | dead letter vs 整批失败 |

OpenSearch 主线的部分结果、批次可见性、标量精度已由 `RETRIEVAL-01` 按探索调研 §6 收口；新提供方重复这些谎言时，应回到同一组失败反例，而不是再开一条「补齐检索能力」叙事。

---

## 7. 核对范围

- 未部署 DataHub、OpenMetadata、Amundsen 或 Iceberg 对照集群；机制以核对日的官方文档与源码为准。
- 未重复测量本仓 OpenSearch 主线；适配器诚实性以探索调研 §6.5、`RETRIEVAL-01` 与当前测试为准。
- 未取得千万级投影追平或检索质量数字；容量仍由 [规模架构](SCALE_ARCHITECTURE.md) / [规模基准](SCALE_BENCHMARK.md) 拥有。
- 采用外部机制前应固定版本复核链接；不能把本节表格当成协议能力清单。
