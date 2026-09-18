# Aspect：写入单元 vs 读/检索形态

日期：2026-08-20  
对照：DataHub、Unity Catalog、Apache Atlas / Ranger、OpenMetadata；检索查询面业界覆盖见 `RETRIEVAL.md` §7。  
范围：**第 ② 层**（知识内容）的写粒度与读/检索形态；③ 的 AccessSpec / RetrievalPlan 从这里的字段访问声明编译。

不在本文：挂 git、Catalog pin（⓪ / ①，见 `LAYERS.md`）。Aspect 从 ② 才感知。

本文当前冻结的是 Snapshot Aspect 的读与检索。Aspect 通过 State/Stream Binding 指向墙外物化、以及 Snapshot 与动态 lane 的统一检索见 `LIVE_MATERIALIZATION.md`。

写入和冲突检查按 Address 单元进行；一个单元如何编码，由 authority 与 codec 承担。本文只回答：**读和检索要不要、以及怎样按 Aspect 走不同形态。** 完整读协议（ReadContext、LOG/DIFF/GET_PROVENANCE 分责、SEARCH 与 Projection、零结果）见 `KNOWLEDGE_CATALOG_DESIGN.md` 第 7 章。

---

## Goal

冻结 Snapshot Aspect 的写粒度与读/检索形态：写入按 Address 拆单元，读取可拼装，检索按 Schema 访问声明定位候选，命中后回同一 basis 的 Canonical。

## Non-Goals

- 不拥有 ⓪/①（挂 git、Catalog pin）；Aspect 从 ② 才感知（文首）。
- 不拥有 State/Stream Binding 物化（`LIVE_MATERIALIZATION.md`）与 SEARCH 代数（`RETRIEVAL.md`）。
- 不把 `permissions` 做成 `kc read` 闸门或 SELECT 放行（`PERMISSIONS.md`）。

## 硬性约束 / Invariants

- `I-01` KnowledgeRef 是 `(repository, object_id)`；Aspect 不是另一套 Ref。
- `S-01` 字段 AccessHints 只声明 `text/filter/sort`，禁止 provider / stored / summary / key。`origin` 是 Schema Canonical frontmatter 上的 resource-access 原点，不是 `access[]` 词，也不是实例文件。
- `C-01` / `R-01` Retriever 返回 CandidateRef，SEARCH 在同一 basis hydrate 后交付知识结果；`K-26` 见系统设计 §9.3。
- 字段身份是 `(schema, aspect, path)`；裸 path 有歧义必须拒绝。

## 选定方案 / 被否决方案

- 选定：写单元 ≠ 默认读形态 ≠ 检索文档 ≠ 权限强制路径（[ADR-006](KNOWLEDGE_CATALOG_DESIGN.md#adr-006) / [ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023)）。
- 否决（本文边界）：把实体的全部 Aspect 编进搜索；GRANT 当表字段 FTS；用读取端全量扫描替代检索能力；semantic overlay 写进 `access[]`。系统级拒绝见 [R-07](KNOWLEDGE_CATALOG_DESIGN.md#r-07) / [R-10](KNOWLEDGE_CATALOG_DESIGN.md#r-10)。

## 接口契约 / 状态机

编译链见本文「推导」节：Schema access[] → AccessSpec → Probe → RetrievalPlan → CandidateRef → hydrate。Schema 是 Writer 入库的 `schema/*`。SEARCH 代数由 `RETRIEVAL.md` 拥有。参考实现可落在 `knowledge/reader/`、`retrieval/`、`index/`。


## 行业参照与设计推论

DataHub、Unity Catalog、Atlas / Ranger 和 OpenMetadata 是对照写粒度、字段选择、检索与授权分工的
调研方向。本文没有逐项核验其接口和版本的一手证据，不把“各家都这样做”作为决定依据。
检索机制的可定位来源及适用说明由 `RETRIEVAL.md` §7 维护。

本项目的选择来自以下约束：

- 不同来源可以独立维护同一对象的不同方面，因此写入和冲突检查需要细到 Address。
- 消费任务往往需要把多个方面一起理解，因此默认读取可以拼装对象，同时保留按 Address 精确读的能力。
- 只有业务声明过的字段才有明确查询含义，因此检索文档由 Schema 的访问声明编译。
- 一份授权快照可能落后于外部系统，因此不能拿它代替实际业务操作的权限检查。

由此得到本项目的推论：写的原子单位、搜的文档形状和权限强制路径需要分别设计。
例如，把源系统 GRANT 捕获为 `permissions` Aspect 后，它是可追溯的知识；外部业务操作仍由
相应源系统执行授权，KC 的正文读权则由 `PERMISSIONS.md` 拥有。

---

## 推导

**Reader 必须能按 Address 读。** `RESOLVE` / `READ` 可打到 Entity（拼装）或 `KnowledgeAddress`（单 Aspect / 单 Member）。这样读取范围可以匹配维护单元，也可以满足整对象消费。

**拼装是读策略，不是存储形状。** 默认 `READ(object_id)` 仍拼 `{ aspectName: value }`。调用方可 `include` / `exclude`。Authority 怎样编码 unit，调用方不必知道。

**检索另选编。** Projection 只定位 typed `CandidateRef`，命中后在同一 basis 回读完整 Canonical（K-19、K-25）。`AspectSelector` 只属于显式 READ；SEARCH 不用它裁结果。调用方信封是否含全文见 `PERMISSIONS.md` 交付链首段。默认编哪些字段看 `schema/*` 的访问声明（`DESCRIBE_SCHEMA`）。GRANT 正文不要自动当表的 `text` 面；是否可检索只看这份知识自己的字段声明，不按 aspect 名做成第二种对象。Workspace 解析只提供成员 pin；RetrievalPlan 按请求扇出，不把联邦结果抄进一个大索引。

**`permissions` 是 SOURCE 知识，与 `structure` 同构。** Writer `COMMIT`、进 Canonical、可落后（所有外部 STATE 同步的通性）。真正 SELECT 放行在 Ranger / Unity / 内控；仓内 digest 不是 GT。Agent 读它是在读「源系统当时对谁开了」，不是在问「我能不能 `kc read`」——后者见 `PERMISSIONS.md`。GRANT 正文通常不声明 `text`，所以不是表文档的 BM25；需要过滤发现时给明确字段声明 `filter`，并在命中后回读完整对象。

**读取能力不能替代检索能力。** 精确读取支持对象与 Address；生产检索走 RetrievalPlan + provider + hydrate。缺少检索能力时明确失败，不能扫描权威正文再做整包 JSON 包含匹配。`AspectSelector` 只用于显式 READ。

编译链（形状不在本文）：`schema/*` 声明 → `DESCRIBE_SCHEMA` → `AccessSpec` → Probe → `RetrievalPlan` → `CandidateRef` → 同一 basis hydrate。物化投影是某个 provider 对 AccessSpec 的实现，可重建，不是 Writer 的 IndexDefinition。

仓储约定：`permissions` 的 schema 通常不声明 `text`；强制仍在源系统。GRANT 快照进 Canonical 后，Catalog 不在查询路径上。

访问声明必须与逻辑类型一起校验：复合正文可以保留为知识，但没有定义标量解释的复合值不能声明标量访问。
已有多值标量与引用列表沿用各自语义。只声明文本发现时，不隐式要求整值匹配或排序索引；后端的词项长度
和物理字段限制不能通过静默漏值改变发现、存在性或完整性。声明有效、提供方支持与指定版本就绪分别证明。

公开类型：`knowledge/`（Ref、Address、AspectSelector）、`knowledge/reader/README.md`、`retrieval/README.md`。
