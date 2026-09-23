# 知识探索路线调研

核对日期：2026-09-08。本文保存一手资料、架构推论与待验证路线，不是新增协议或实现排期。
调研采用官方文档、部分公开源码和论文，并对照本仓设计、公开类型及已有测试；没有部署对照项目，
也没有取得本项目的检索质量或千万级性能结果。外部链接指向核对时的内容，采用前应固定具体版本复核。

## Goal

研究知识底座怎样让模型按问题有效地发现、阅读和引用知识：比较规范词表与查询改写、模型直接阅读、
动态渐进披露、关系导航及可选的向量召回，保存适用条件、代价和选择依据。
让后续实现由实际知识形态与任务证据驱动，不因某种索引流行就提前固定消费方式。

## Non-Goals

- 不拥有 SEARCH 代数与字段访问合同；它们分别由 [检索](retrieval.md) 和
  [声明式索引](declarative-index.md) 拥有。
- 不重定义消费发现、Schema 生命周期或语义文件视图；由
  [知识读写](knowledge.md) 拥有。
- 不重定义发现与正文授权、Writer 边界或投影发布；沿用 [权限体系](permissions.md)、
  [核心架构](core-architecture.md) 与 [投影控制](index-control.md)。
- 不把数据库、Agent 框架或论文的能力表复制成 Catalog 的协议承诺；不把通用知识底座改成检索应用。
- 不拥有派生投影控制与 Retriever 的业界对照；由 [派生投影控制与 Retriever 业界对照](ingestion-retrieval-research.md) 拥有。

## 硬性约束 / Invariants

本研究沿用已有不变量，不新增协议不变量 ID：

- `I-01`：知识引用保持 Repository 与对象身份，不能按文本相等或模型相似判断合并身份。
- `S-01`：字段访问声明仍按现有合同解释；Schema 是经 Writer 发布的知识，不变成引擎配置或项目源码。
- `C-01` / `R-01` / `V-01`：候选与知识正文分责，交付保持固定依据的回读及来源；导航摘要不能替代证据。
- `P-01`：投影可丢、可重建；模型消费不触发隐式索引维护，不回写知识权威。
- `KS-02` / `AUTH-01` / `AUTH-03`：探索不扩权，发现与正文交付仍分层；不能把正文读权直接改成搜索候选过滤。

这些约束不意味着消费只能经过一次关键词 SEARCH；模型可以组合已获授权的发现、精确读取、关系和文件视图能力。

## 选定方案 / 被否决方案

**研究取向：**优先比较模型直接阅读、动态渐进披露，以及规范知识上的词表辅助与查询改写。
向量保留为可选实验，不是默认前提，也不预设最终必需。选择实验对象不代表批准某种实现。

**取向更新（2026-09-23）：**语义召回已按另行合同升格为 SEARCH 的扩展路线——召回策略闭集、
候选资格先于召回、结果一律近似，见 [检索](retrieval.md) 的召回策略扩展一节（原 §8 / ADR-028）。
本段保留的是当时的实验取向记录；embedding 与融合策略的具体选型仍以本研究的实验证据为准，
不因合同路线的存在而预设结论。

**不采纳的推论：**不以“行业已经放弃 SQL”“五原语已最小完备”作为设计依据；不从
“重排不能扩大候选”推出“必须建向量索引”；不从“知识规范”推出“词表一定覆盖全部问题”。
不预先固定一棵适用于所有问题的披露树，也不把常规模型内部 MoE 等同于外部知识路由。

**仍开放：**导航材料怎样随知识演进、哪些任务适合直接读、词表如何帮助而不限制探索、
何时需要增加候选召回方式、怎样判断探索覆盖与成本。本文给出实验条件，不裁决新 API。

## 接口契约 / 状态机

本文不定义新字段、错误码、命令或状态机。现有具体合同见 [retrieval 包](../../retrieval/README.md)、
[AccessPlan](../../retrieval/accessplan.go)、[RelationQuery](../../retrieval/relation.go)、
[索引端口与就绪检查](../../index/README.md)、[Reader](../../knowledge/reader/README.md) 和
[交付链](../../delivery/README.md)；逐项验证入口由 [验证目录](test-catalog.md) 维护。

研究中的“模糊锚定”不改变精确 RESOLVE；“模型阅读”不改变 Refine 的引用守恒；
“渐进披露”不新增无界对象 LIST 或 authority 扫描兜底。若现有导航不足以支持任务，应记录缺口、
评审有界访问方案，而非暗改已有入口，或仅凭当前实现缺少入口否决潜在路线。
采纳新能力前，回到相应 owner 选择语义，再落公开类型和失败反例。

## 1. 已核实的方向与需要纠正的前提

### 1.1 语义理解不要求向量索引

模型可以在查询端理解问题、读取领域定义、映射术语和生成多个查询，后端继续采用词法与类型化过滤。
也可以在材料已经可寻址时直接阅读，用上下文判断相关性与答案。索引降低材料定位成本，
并不需要穷尽模型可以回答的语义问题。

受控词表有成熟知识组织标准：[W3C SKOS](https://www.w3.org/TR/skos-reference/) 区分概念、
首选名称、别名、范围说明及上下位/相关关系。[Query2doc（EMNLP 2023）](https://aclanthology.org/2023.emnlp-main.585/)
在所测公开数据集上以 LLM 查询扩展改善 BM25，提供了不依赖向量索引的实证先例；
它不是受控词表实验，也不能证明本项目收益。

### 1.2 语义层没有淘汰底层查询语言

[Databricks Genie](https://docs.databricks.com/aws/en/genie-agents/concepts) 仍结合语义上下文生成只读 SQL；
[Snowflake Cortex Analyst](https://docs.snowflake.com/en/user-guide/snowflake-cortex/cortex-analyst) 仍生成 SQL；
[LlamaIndex 属性图](https://developers.llamaindex.ai/python/framework/module_guides/indexing/lpg_index_guide/)
同时提供自由生成与模板参数化的 Cypher 检索器。
这些反例支持受约束执行与语义定义的重要性，不支持“业界基本放弃 Text-to-SQL/Cypher”的断言。
它们也不是给本项目新增 SQL/Cypher 消费面的理由。

### 1.3 五类能力不是已确立的统一原子代数

实体消歧是可能组合多个查询的任务；精确条件限定候选资格；BM25、相似度和融合涉及召回与排序；
关系一跳与路径算法又有不同预算及覆盖要求。可以用这些类别检查需求，但不能只凭 API 名称
认为它们已正交、最小或完备。稳定引用、来源、版本和维护责任还需要独立设计。

## 2. 可参考的开源项目

“成熟”在这里指有持续维护的公开实现、文档和发布记录，不表示所有子能力均适合本项目。
核心实现可分别检查 [DataHub](https://github.com/datahub-project/datahub)、[Vespa](https://github.com/vespa-engine/vespa)、
[DataFusion](https://github.com/apache/datafusion)、[OpenSearch](https://github.com/opensearch-project/OpenSearch)、
[Qdrant](https://github.com/qdrant/qdrant)、[Weaviate](https://github.com/weaviate/weaviate)、
[LlamaIndex](https://github.com/run-llama/llama_index)。不以 stars、产品总规模或托管服务能力替代开源部署验证。

### 2.1 DataHub：知识声明到派生投影

**已核实机制：**Aspect 上的 Searchable / Relationship 声明驱动搜索字段与关系构建；
声明由类型化解析器校验，搜索 transformer 按声明提取字段。
常规版本化 Aspect 的索引可从主存储重放；重放到旧索引不会自动清除主存储已删除的旧文档。
资料：[模型声明](https://github.com/datahub-project/datahub/blob/master/docs/modeling/extending-the-metadata-model.md)、
[声明解析](https://github.com/datahub-project/datahub/blob/master/entity-registry/src/main/java/com/linkedin/metadata/models/annotation/SearchableAnnotation.java)、
[索引恢复](https://docs.datahub.com/docs/how/restore-indices)。

**本项目推论与边界：**借鉴声明编译、字段撤销、删除传播和干净重建。保留知识 Schema 的 Writer 发布方式，
不复制其源码 PDL 模型、数据目录实体与采集器体系。Relation 仍是有身份、来源和版本的知识对象，
不能因为后端可生成边就只保留索引关系。不同 Aspect 存储方式需分别核实，不能概括为 DataHub 所有数据都在同一主库。

### 2.2 Vespa：候选产生与排序分责

**已核实机制：**Schema 可以派生索引字段；查询选择候选，rank-profile 计算排序。
`rank(a,b)` 由第一个操作数决定候选，第二个提供排序特征；过滤近邻查询可按选择率采用不同物理策略。
配置变化可能需要重跑已有文档的索引链。
资料：[Schema](https://docs.vespa.ai/en/basics/schemas.html)、[查询算子](https://docs.vespa.ai/en/reference/querying/yql.html)、
[近邻过滤](https://docs.vespa.ai/en/querying/nearest-neighbor-search-guide.html)、[重建](https://docs.vespa.ai/en/operations/reindexing.html)。

**本项目推论与边界：**参考候选、过滤、特征、排序与重建的明确分工，不引入其排名 DSL 或物理参数作为知识协议。
其 [parent/child](https://docs.vespa.ai/en/schemas/parent-child.html) 有弱引用、全局父文档及循环限制，
不能据此承诺任意知识图路径能力。

### 2.3 Apache DataFusion：下推的正确性证明

**已核实机制：**TableProvider 按条件报告 Exact / Inexact / Unsupported；Inexact 仍需上层过滤。
其接口明确指出，尚有 Inexact filter 时不能下推 LIMIT，否则过滤后可能缺少结果。
资料：[自定义提供方](https://datafusion.apache.org/library-user-guide/custom-table-providers.html)、
[TableProvider 合同](https://docs.rs/datafusion/latest/datafusion/datasource/trait.TableProvider.html)。

**本项目推论与边界：**现有逐条件 Probe 已采用相近思路；继续借鉴剩余条件、排序和 limit 保证。
基数与成本估计用于选择计划，不能代替正确性证据。无需为此采用完整 SQL/关系执行引擎，
也不能把可能多返回的 Superset 与可能漏项的 Approximate 混同。

### 2.4 OpenSearch：现有提供方上的实验与诊断

**已核实机制：**multi-fields 可对同一来源字段建立不同物理解释；field capabilities API 可检查跨索引类型及能力。
hybrid 支持分数归一化或按排名融合；近邻内部过滤与外部 post_filter 的候选范围不同，后者可能不足 k。
资料：[字段能力](https://docs.opensearch.org/latest/api-reference/search-apis/field-caps/)、
[混合查询](https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/index/)、
[过滤](https://docs.opensearch.org/latest/vector-search/filter-search-knn/index/)、
[混合解释](https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/explain/)。

**本项目推论与边界：**已有适配器适合作为词法、查询改写和未来候选实验的对照，先复用再决定是否增加引擎。
引擎支持某种查询不等于协议已选定；底层 DLS 不替代本项目发现/正文读分层与 Writer 边界。
索引 refresh 或 alias 切换也不能单独证明固定知识版本的回填、覆盖和可回读性。

### 2.5 Qdrant：显式多阶段候选范围

**已核实机制：**Query API 可嵌套 prefetch，子查询先产生候选，主查询在其结果上进一步处理或融合；
子阶段 limit 决定后续可见范围。payload 索引参与过滤基数估计，物理规划按 segment 选择执行方式。
资料：[多阶段查询](https://qdrant.tech/documentation/search/hybrid-queries/)、
[索引](https://qdrant.tech/documentation/manage-data/indexing/)、[查询规划](https://qdrant.tech/documentation/search/#query-planning)。

**本项目推论与边界：**参考阶段输入范围和截断语义；向量路线获实测支持后，才评估独立提供方是否值得其更新、
存储与一致性成本。HNSW 是物理近邻结构，不是知识 Relation；collection 健康或近似计数也不是知识版本覆盖证明。

### 2.6 Weaviate：能力配置与就绪不是同一事实

**已核实机制：**词法排序、匹配过滤和范围过滤可以分别配置；hybrid 提供不同融合方式。
新增属性不会自动补齐既有对象所有相关索引与向量；异步向量构建也存在写入到可召回之间的间隔。
资料：[倒排索引](https://docs.weaviate.io/weaviate/concepts/indexing/inverted-index)、
[集合演进](https://docs.weaviate.io/weaviate/manage-collections/collection-operations)、
[监控](https://docs.weaviate.io/deploy/configuration/monitoring)。

**本项目推论与边界：**必须分清声明、引擎能力、版本覆盖和运行就绪。
其 [cross-reference](https://docs.weaviate.io/weaviate/manage-collections/cross-references) 有查询成本与建模限制，
不能泛化成通用图执行保证；托管 Query Agent 等能力也不能因核心数据库开源就算成开源部署默认能力。

### 2.7 LlamaIndex：可组合的探索方式

**已核实机制：**属性图提供同义词、向量上下文、自由 Cypher 与模板 Cypher 等检索器。
核对时 PGRetriever 汇总子检索结果后按文本去重；RRF 存在于单独的融合检索器，
不是属性图检索默认附带的成本优化器。
资料：[属性图指南](https://developers.llamaindex.ai/python/framework/module_guides/indexing/lpg_index_guide/)、
[PGRetriever 源码](https://github.com/run-llama/llama_index/blob/main/llama-index-core/llama_index/core/indices/property_graph/retriever.py)、
[融合源码](https://github.com/run-llama/llama_index/blob/main/llama-index-core/llama_index/core/retrievers/fusion_retriever.py)。

**本项目推论与边界：**借鉴按任务组合定位、读取与关系导航；不按文本去重替代 KnowledgeRef，
不把 LLM 同义词生成等同于完成实体消歧。框架集成不提供底层数据库的全部保证，
LLM 抽取结果也不能绕过既有写入及来源流程成为知识事实。

## 3. 潜在消费路线

### 3.1 规范词表辅助查询

适用于概念、名称、类型与定义较规范的领域。模型读取相关概念后，把用户表达映射为规范词、别名、
多个候选解释或字段约束，再使用词法检索；阅读结果后可以改写查询或探索关系。

词表是可选辅助知识，不要求所有问题先完成映射。概念身份、名称、别名、范围说明与上下位/相关关系
应分开；领域内多义词保留多个候选，原问题和限制条件不能被改写丢失。
同义替换与放宽到上位概念不是同一操作；模型也不应增加用户未表达的筛选限制。

[OpenSearch synonym_graph](https://docs.opensearch.org/latest/analyzers/token-filters/synonym-graph/) 是多词同义词
扩展或规范化的可检查实现先例；它的 token graph 不是知识关系图。若将词表编译为引擎规则，
应由投影配置承担并保持与词表版本的对应，不把物理 analyzer 写进业务 Schema。
概念覆盖不足、映射歧义和维护成本是实验对象，不是预先选择向量的理由。

### 3.2 直接阅读与动态渐进披露

材料范围较小且已获授权、可寻址时，模型可以直接阅读并作出相关性判断或回答，不必先执行 SEARCH 或词表映射。
范围较大时，按已经获得的线索逐步选择知识源、概念、对象、关系或文件视图；每次阅读改变后续选择，
允许返回上一步、换词、跨源或保留多个探索分支。

知识域可以提前组织，问题域要求的阅读视图在任务中形成。同一批材料回答不同问题，
可能需要不同入口与粒度。README、Schema、领域词表、导读和引用提供导航线索，
不要求模型沿预制的统一分类树逐级下降。导读是可丢的线索，涉及结论时应能回读来源正文。

这是一条完整消费路径，模型负责探索循环，而非只能在固定 Top-K 之后充当重排器。
[Anthropic 的上下文工程实践](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
描述了保存轻量引用、按需加载和渐进探索的方式，同时指出运行时探索有成本且需要可理解的导航工具。
这是工程先例，不是本项目的端到端保证。

现有产品设计提供 README、Schema 浏览与语义文件视图方向。直接读使用既有读取授权与固定依据；
有界 BROWSE 不被解释成对象实例全集；文件视图不伪装成完整 Knowledge SEARCH，
也不能把 Snapshot 文件内容当成动态 State 的当前值。若只有贫乏源摘要而无法决定下一步，
应评测并研究更有用的导航材料，而不是偷偷扫描 authority 或断言模型探索不可行。

### 3.3 语义判断、重排与候选扩展

已在上下文的材料可以由模型理解、比较和筛选，不需要预先索引所有可问的问题。
但输入窗口之外的对象不会因为执行重排自动出现。核对时 [Refine](../../retrieval/README.md)
保持输入引用集合；其候选内完整评判不是语料召回完整性。

扩大候选可以依靠词表辅助、查询改写、多次查询、关系导航和进一步读取，并不必然要求向量。
若选择新增语义候选提供方，则需另行明确候选产生与回读合同；不能借 Refine 或既有 MATCH 暗中加入。

### 3.4 向量与多路检索：可选实验

在前述路线反复遗漏有用材料，或读取/模型成本明显过高时，可以评估向量、稀疏语义召回和多路融合。
向量可能帮助词汇差异大的材料定位，但相关程度、成本和可维护性要由实际任务验证。
既不假设必须增加 embedding，也不根据知识规范就宣布它无价值。

评估需计入输入字段、分块与预处理、模型变更、索引重建和版本回读成本；物理选项不进入业务 Schema。
现有 OpenSearch 可以作为候选实验后端；增加另一引擎必须有可复现的质量或资源收益。
这些均为潜在路线，不给当前 SEARCH 增加 VECTOR/HYBRID 或新的 AccessHint。

### 3.5 领域路由与 MoE 的边界

按问题选择知识范围或领域专长，是值得研究的动态路由。常规模型内部 MoE 选择的是参数专家，
并不会自动获得外部知识库的引用、读取能力或额外上下文。
[Switch Transformers](https://arxiv.org/abs/2101.03961) 提供参数稀疏路由的研究依据，
不证明外部知识的渐进披露已被解决。可以研究专门训练的知识路由，但消费协议不应依赖某种模型内部结构。

## 4. 组合语义与探索证据

过滤、候选截断和排序不能任意交换：通常 `Filter(TopK(S))` 不等于 `TopK(Filter(S))`。
模型改写为多个查询也不天然保持原问题语义。需要知道每阶段面向什么范围、应用什么限制、
在哪里截断以及为何继续或停止；具体记录形状应在采纳路线时选定，不在本文预设。

两类规划应分责：模型按问题和新证据选择下一次访问；检索执行器在一次已确定请求内优化物理路径。
后者只能作保持合同的转换，或如实暴露近似/覆盖限制；前者不能将任一成功的局部查询解释为探索穷尽。

时间、计算和上下文容量是主要成本约束，但还需验证证据利用与探索覆盖。
[Lost in the Middle](https://arxiv.org/abs/2307.03172) 在所测模型中观察到证据位置影响表现；
不能据此量化当前模型的误差，但它说明“材料能放入上下文”不足以证明材料被可靠使用。
渐进探索也可能过早丢弃支路，摘要或记忆压缩可能损失后续问题需要的细节。

应区分以下判断：

| 判断 | 依据与限制 |
|---|---|
| 一次查询满足其合同 | 该请求固定范围、版本、能力、覆盖与截断证明 |
| 候选已全部评判 | 仅针对输入候选集合，不包含窗口之外的对象 |
| 模型已完成当前任务 | 任务评测与引用支持，不能代替全集证明 |
| 在已读材料中没有发现 | 有限阅读下的结论，不等于知识域中不存在 |

最终结论应保留可核对的引用与版本；导航摘要、模型判断、物理 score 都不能成为新的知识权威。
能力自省也应区分 Schema 声明、提供方支持、指定版本的索引就绪与调用授权，不能由提示词代替服务端检查。

## 5. 比较实验与进入实现的条件

以下是选型所需的实验建议，不是已执行测试、强制实现顺序或新的 MVP 完成条件。

| 对照路线 | 需要回答的问题 |
|---|---|
| 可容纳的明确材料集合直接读 | 模型能否可靠利用全部给定证据；材料量与位置怎样影响质量和成本 |
| 模型渐进阅读 | 不强制先 SEARCH；动态选源、读取与关系导航能否完成任务，哪里走入死路或漏掉分支 |
| 原始词法查询 | 不借助模型改写时的可复现基线 |
| 词表辅助查询 | 定义、别名及领域范围是否改善定位；映射错误和维护成本有多大 |
| 模型有限查询组合 | 保留原意与限制的扩展是否找到新增证据；与直接阅读相比节省了什么 |
| 按任务组合读取与检索 | 模型能否决定什么时候直接读、搜索、换词、展开关系或停止 |
| 可选向量/稀疏语义/融合 | 仅在需要时加入对照，测增益是否值得额外物化与维护成本 |

实验使用相同知识 basis 与授权范围，记录各路线可用导航、词表和模型配置；
不能让所有基线先经过同一个 Top-K 门槛，再宣称直接阅读没有优势。
同时报告相同预算下的质量与达到目标质量所需成本，而非只比较引擎毫秒数。

问题集应覆盖：规范名称与编码、多义词、同义/跨语言表达、未收录术语、跨源证据、
需要沿关系追问的问题，以及“未发现”与“确实不存在”容易混淆的问题。
协议用例和领域夹具分别遵循各自已有组织规范，不把数仓实体搬入通用协议场景。

| 维度 | 建议观察 |
|---|---|
| 任务质量 | 任务完成率、证据支持的答案正确性、引用可复核性、错误的穷尽/不存在判断 |
| 查询理解 | 原意与限制保留、多义词处理、未覆盖术语、无依据新增条件 |
| 定位与排序 | 标注相关材料的 Recall@K、nDCG@K；对开放式阅读另计相关证据覆盖，不强行化约为 Top-K |
| 探索过程 | 读取对象/字节/token、模型轮数、重复访问、分支遗漏、导航不足与错误停止 |
| 总成本 | 端到端耗时分位数、模型费用、索引/词表维护与更新成本 |
| 协议正确性 | 身份、固定依据回读、精确条件、授权、删除传播、分页和失败语义 |

采用向量时，ANN 相对于同一度量下精确近邻的召回率，应与“材料是否回答用户问题”分开测量。
外部项目成熟、论文报告有收益或模型给出自信答案，都不能替代本项目证据。

进入实现前需要：有代表性失败案例与对照结果；选定承担该能力的层；说明沿用或修改哪些 owner 合同；
确认导航/词表/投影的维护成本与失效处理；先写会失败的契约证据，再修改公开类型和实现。
若现有能力组合已经满足目标，则保留研究记录，不因存在潜在路线而扩大协议。

## 6. 本仓索引层审阅与 MVP 建议

本节记录 2026-09-08 对当前工作树的审阅，给出建议优先级，不代表已经修改协议、修复实现或新增验收门槛。
此处保留实现前审阅记录；随后获准实施的结果与范围见 §6.5。
已阅读声明、编译、执行、OpenSearch 适配器与相关测试；通过临时小程序复核了 Schema 类型接受差异和
Go JSON 大整数往返精度。其余结论来自代码与后端官方合同核对，尚未做真实集群故障注入或性能实验。
后续代码变化可能使定位失效，实施时应重新核对。

### 6.1 声明和原语：保留范围，补齐语义

建议 MVP 继续保留现有三个逻辑访问声明，具体定义由 Aspect 访问与检索 owner、
[Schema 公开类型](../../knowledge/schema.go) 和 [查询公开类型](../../retrieval/searchop.go) 拥有。

- `text` 用于分词后的文本发现；名称、别名和说明可按需要选用。
- `filter` 用于类型化条件；编码、概念引用、枚举与范围条件不必依赖向量。
- `sort` 用于明确需要排序的字段，避免所有可过滤字段都被无差别物化为排序结构。

不建议为此增加 `keyword`、`lexical`、`prefix`、`range` 等声明。前两者容易混淆全文发现与整值匹配，
后两者已经属于查询算子。`relation` 由关系知识及其查询承担；`provider`、`stored`、`summary`、
分片数、分析器名称等不进入业务访问声明。向量、融合和图路径仍按前文条件评估。

MVP 的工作在于建立统一的“逻辑类型 × 访问方式”校验，贯通 Schema 发布、能力发现、投影编译、
查询规范化与比较。逻辑上无意义的组合应在知识合同层明确处理；某个后端暂不支持的有效组合，
仍由能力检查处理，不能让 Writer 依赖 OpenSearch。给维护者的诊断应能区分声明有效、后端支持、
版本就绪及预计派生的索引结构；已有 AccessDigest 与 PhysicalDigest 分责可继续使用。

现有文本匹配、类型化谓词、有界布尔组合、稳定分页和一跳关系查询已经覆盖这轮 MVP 建议。
精确引用的 RESOLVE/READ 与模型把口语映射到概念是两种工作，后者可组合已有入口，不宜改写前者的合同。
需要补清或验证的边界包括：

- 中文、缩写、连字符、大小写与 Unicode 的分析行为；精确规范化、全文分析和词表扩展分别处理。
- 空值、空数组、空字符串、零值、false 与字段不适用的区别，不能由后端静默丢值规则决定逻辑缺失。
- 短语是否允许跨字段、Aspect 或数组元素；查询分析后没有 term 时的行为。
- 多值排序如何选值、缺失值排位，以及整数、时间和稳定身份的比较。
- 多条件是否要求命中同一个复合成员。例如“成员名为甲且角色为审核人”不能仅凭两个独立存在条件
  推出同一成员满足。现有多值 existential 语义有其用途；只有实际 MVP 场景要求相关匹配时，
  才评审显式成员作用域，不能暗改已有 AND。参照 [OpenSearch nested](https://docs.opensearch.org/latest/field-types/nested/)
  和 [Vespa sameElement](https://docs.vespa.ai/en/reference/querying/yql.html)。

### 6.2 先修当前正确性缺口

**声明到类型化投影没有完全贯通。** [Schema 解析](../../knowledge/schema.go) 接受的类型列表没有
date/datetime/timestamp，[标量规范化](../../retrieval/scalar.go) 却已支持这些类型；临时程序分别调用两个
公开入口，确认前者拒绝、后者接受相同时间类型。解析器还接受 object/array 搭配 filter，
而 [投影编译](../../index/extract.go) 无法把这些类型当作受支持标量处理。后一点是静态路径核对，
未将该样例完整走过 Writer。修复需要从真实 Schema 发布开始验证，不能只用手工构造的 AccessSpec
证明能力存在，也不能用实现缺口收窄 owner 已选定的时间范围能力。

**text-only 仍产生整值 keyword。** [projectionCell](../../index/extract.go) 对所有 string 填入
StringValue，[适配器编码](../../retrieval/opensearch/projection.go) 将其写入
[keyword mapping](../../retrieval/opensearch/client.go)，即使只声明了 text。
除多建索引外，长正文还可能超过 Lucene 单词项的字节限制，导致整个文档写入失败。
参照 [OpenSearch keyword](https://docs.opensearch.org/latest/mappings/supported-field-types/keyword/)
和 [Lucene 常量](https://lucene.apache.org/core/10_4_0/core/constant-values.html)。
建议按 access 生成所需物理槽位；对 filter/sort 长字符串另行证明支持范围，不能简单静默截断或
设置忽略阈值后继续报告精确。先测仅 text 的长正文及 UTF-8 多字节边界。

**HTTP 成功被等同于检索完整。** [搜索响应解析](../../retrieval/opensearch/search.go) 与
[关系响应解析](../../retrieval/opensearch/relations.go) 未处理超时和分片失败信息，可能把部分 hits
按完整候选处理；请求也未显式禁止部分搜索结果。
[OpenSearch Search API](https://docs.opensearch.org/latest/api-reference/search-apis/search/)
默认允许部分返回。建议统一响应完整性校验，遵循当前必需部分失败与显式 best-effort 的区别；
覆盖 HTTP 200 下超时、分片失败、空结果及非空结果，不能把缺少候选当普通零命中。

**整数和时间在链路中失去精度。** [OpenSearch sort 解码及游标](../../retrieval/opensearch/search.go)
使用普通 JSON 解码到 any，临时程序确认整数 9007199254740993 往返后变为 9007199254740992。
[知识集比较](../../cli/dataset_search.go) 和 [剩余条件范围比较](../../index/residual.go) 也有转 float64
的路径。另有时间规范化保留纳秒而物理 date 使用毫秒的差异；目前时间 Schema 入口尚有上述阻断，
不能把它描述为已正常发布的时间字段必现问题。建议使用贯穿解码、游标和联邦排序的无损类型化比较；
时间物理表示同时满足选定精度与日期范围，不能只换 date_nanos 就忽略支持范围。
参照 [Go JSON 数值解码](https://pkg.go.dev/encoding/json#Decoder.UseNumber)、
[OpenSearch date](https://docs.opensearch.org/latest/mappings/supported-field-types/date/)。
验证 2^53 两侧、int64 边界、同毫秒不同纳秒，以及分页不漏不重；本轮没有跑真实引擎分页复现。

**增量发布的可见性屏障不完整。** [增量 bulk](../../retrieval/opensearch/projection.go) 每 500 条分批，
只等待最后一批 refresh，随后发布新 basis 就绪；
[官方 Bulk 合同](https://docs.opensearch.org/latest/api-reference/document-apis/bulk/#refresh)
说明一次请求的刷新只覆盖它涉及的分片。前批涉及、末批不涉及的分片尚未可搜索时，就可能提前发布。
应在发布前证明全部变更可见，补跨批、跨分片反例；无需把每次增量退回全库计数。
全量重建已有显式 refresh 与计数检查，不能将这个增量缺口泛化到所有发布路径。

### 6.3 执行设计：保留分层，收紧可证明的边界

现有 AccessSpec、ProjectionSpec、Probe、RetrievalPlan、CandidateRef 与固定 basis 回读有实际代码支撑，
无需因为代码由 AI 辅助生成就整体推倒。值得检查的是跨层承诺能否由失败反例证明。
当前 fragment 主要记录能力解释，不能据此宣称已经实现通用多引擎物理调度或成本优化器。

**预算与取消优先于成本优化器。** [搜索循环](../../index/search.go) 限制输出命中数，但没有完整的
候选总数、页数和总时间预算；[Retriever 端口](../../index/engine.go) 与后端 HTTP 也未贯通调用方取消。
先为持续前进但始终被补判淘汰的候选流，以及取消后不再访问后端建立证据；按 owner 既有语义处理
预算耗尽与可继续位置。单次 HTTP 超时不能替代整个查询的预算。

**剩余条件补判必须保持完整表达式语义。** 当前只要存在 Superset，
[residual](../../index/residual.go) 就重判整棵原请求，包括本来 Exact 的 MATCH；其字符串包含与自行
处理标点不能一般地等价于后端 analyzed term/phrase。例如 cat 与 concatenate、hello world 与
hello-world 的判断会出现差异。当前 OpenSearch 主线宣称 Exact，不经过这一分支；这是加入
Superset/source-pushdown 提供方前必须解决的扩展风险。应形成明确补判计划；无法证明等价时拒绝支持，
也不能简单只重判 Superset 叶子而忽略 OR 的组合。
参照 [DataFusion 下推合同](https://datafusion.apache.org/library-user-guide/custom-table-providers.html)，
特别是 Inexact filter 与 LIMIT 的执行关系。

**把批量化落实到真实 I/O。** [知识集合并](../../cli/dataset_search.go) 每次向成员取一条，
初始成员串行访问；[tree ReadMany](../../knowledge/reader/repository_service.go) 内部仍逐对象定位，
默认 locator 每次读取并解码完整 manifest。建议成员小批缓冲、有界并发首批读取，并在同一固定版本
批次内复用定位数据。验收统计后端搜索、manifest 与正文读取次数，不能只统计是否调用了 ReadMany。
这些调用放大已由代码定位，性能收益仍需测量。

**兑现无变化不重写。** [编译器](../../index/extract.go) 已计算投影摘要，但
[增量同步](../../index/sync.go) 没有据此排除内容相同的 upsert。可复用旧投影摘要或批量摘要读取，
让非索引内容变化只推进 basis，检索内容变化才重写；这是投影 owner 已有要求。
同时验证删除、撤销字段访问和 rebuild/apply 最终文档集一致。

### 6.4 参考实现与建议实施顺序

| 参照 | 本轮值得具体借鉴的部分 | 应保留的本项目边界 |
|---|---|---|
| DataHub | Searchable 声明解析、类型校验、字段提取和投影恢复 | Schema 经 Writer 发布，不迁入其源码模型与业务实体体系 |
| OpenSearch / Lucene | text 与整值索引分工、类型编码、PIT 分页、部分失败和 refresh 合同 | 后端就绪不自动等于知识版本完整；物理选项不进入业务 Schema |
| DataFusion | Exact/Inexact/Unsupported、补判与 LIMIT 的顺序约束 | 复核已有 Probe，不为此引入 SQL 引擎或完整成本优化器 |
| Vespa | 候选与排序分责、声明编译、sameElement 的显式作用域 | 不照搬排名 DSL 或把同成员匹配扩成任意图路径 |

一手资料与边界见 §2；优先复用现有 OpenSearch 提供方，第二引擎没有本轮所需证据。
建议将实现拆成以下批次，具体任务仍需按 TASK 与 owner 规则单独认领：

1. **正确性批次：**Schema 到查询的真实纵向测试；按声明建索引；部分结果识别；无损标量与分页；
   跨批增量可见性。先写上述会失败的证据，再修复，不以抽象接口的自洽测试代替真实链路。
2. **可控执行批次：**总预算和取消；补判语义；能力诊断；无变化不重写；有界批量读取。
   用调用次数、索引体积、候选/命中比和延迟分位数证明改善，再考虑更复杂规划。
3. **消费验证批次：**用一小组规范知识验证“模型映射词表后精确/词法检索”及“直接读、按问题渐进探索”。
   词表保持可维护知识，记录未覆盖和歧义；消费侧选择读取与查询，索引层不强制嵌入 LLM。
   两条路线都保留版本与证据，沿用 §5 的公平预算比较。向量、自动本体生成、固定披露树和通用图路径
   不进入本轮建议的优先实施范围。

### 6.5 获准实施后的收口

本轮已按 RETRIEVAL-01 实施声明/类型贯通、text-only 物理槽位、无损数字与时间、后端完整性校验、
增量发布屏障、游标依据复核、共享执行预算与取消、明确的全文补判证明、批量读取及无变化不重写。
真实发布链进一步发现并补齐了整值概念引用的后端支持，以及入口、正文 codec、摘要和客户端回复中的
大整数舍入。当前公开实现与精确形状以各包 README 和测试为准；本节不复制 API 表。

选定时间精度为纳秒，不能精确表示的更细输入明确拒绝；物理时间键升级通过既有重建机制生效。
旧游标在发布依据变化后明确失效；旧无 context 的 Schema/Canonical 端口仍只能在调用边界检查取消。
这些限制没有被定向成功测试或后端 Exact 声明抹去。

最终反例还覆盖了 Workspace 预算不足以取得全部成员排序头部的情况：初始预取避免单成员挤占预算，
无命中且无真实续页进展时明确失败；补判推进与旧偏移回放仍可继续。恢复已有批内偏移仍使用有界批次，
避免用逐条往返实现重放。对应低预算、101 成员、重放 I/O 和并发检查纳入实现测试。

[真实 Schema 到索引的集成](../../index/mvp_consumption_test.go) 覆盖长正文、相邻大整数、纳秒与稳定分页，
并用明确的消费决策验证词表定位、概念过滤、关系导航及 HEAD 前进后的固定版本回读。
它证明既有原语能组成这些消费步骤，尚未证明模型会选对词、分支或停止点。
模型质量、查询策略比较和规模容量仍按 §5 的实验条件推进；未引入向量或第二检索引擎。
正式验证记录与完成状态由 TASK、验证目录及其来源绑定的运行产物承担。
