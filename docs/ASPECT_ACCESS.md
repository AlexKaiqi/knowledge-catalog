# Aspect：写入单元 vs 读/检索形态

日期：2026-08-20  
对照：DataHub、Unity Catalog、Apache Atlas / Ranger、OpenMetadata；检索查询面业界覆盖见 `RETRIEVAL.md` §7。  
范围：**第 ② 层**（知识内容）的写粒度与读/检索形态；③ 的 AccessSpec / RetrievalPlan 从这里的字段访问声明编译。

不在本文：挂 git、Catalog pin（⓪ / ①，见 `LAYERS.md`）。Aspect 从 ② 才感知。

本文当前冻结的是 Snapshot Aspect 的读与检索。Aspect 通过 State/Stream Binding 指向墙外物化、以及 Snapshot 与动态 lane 的统一检索见 `LIVE_MATERIALIZATION.md`。

写冲突靠 Address（一单元一文件）已经定了。本文只回答：**读和检索要不要、以及怎样按 Aspect 走不同形态。** 完整读协议（ReadContext、LOG/DIFF/GET_PROVENANCE 分责、SEARCH 与 Projection、零结果）见 `KNOWLEDGE_CATALOG_DESIGN.md` 第 7 章。

---

## Goal

冻结 Snapshot Aspect 的写粒度与读/检索形态：写入按 Address 拆单元，读取可拼装，检索按 Schema 访问声明定位候选，命中后回同一 basis 的 Canonical。

## Non-Goals

- 不拥有 ⓪/①（挂 git、Catalog pin）；Aspect 从 ② 才感知（文首）。
- 不拥有 State/Stream Binding 物化（`LIVE_MATERIALIZATION.md`）与 SEARCH 代数（`RETRIEVAL.md`）。
- 不把 `permissions` 做成 `kc knowledge read` 闸门或 SELECT 放行（`PERMISSIONS.md`）。

## 硬性约束 / Invariants

- `I-01` KnowledgeRef 是 `(repository, object_id)`；Aspect 不是另一套 Ref。
- `S-01` Schema 只声明 `text/filter/sort`，禁止 provider / stored / summary / key。
- `C-01` / `R-01` SEARCH 返回 CandidateRef，hydrate 回权威；`K-26` 见系统设计 §9.3。
- 字段身份是 `(schema, aspect, path)`；裸 path 有歧义必须拒绝。

## 选定方案 / 被否决方案

- 选定：写单元 ≠ 默认读形态 ≠ 检索文档 ≠ 权限强制路径（[ADR-006](KNOWLEDGE_CATALOG_DESIGN.md#adr-006) / [ADR-023](KNOWLEDGE_CATALOG_DESIGN.md#adr-023)）。
- 否决（本文边界）：DataHub 十五 Aspect 全编进搜索；GRANT 当表字段 FTS；Reader.search 当生产检索；semantic overlay 写进 `access[]`。系统级拒绝见 [R-07](KNOWLEDGE_CATALOG_DESIGN.md#r-07) / [R-10](KNOWLEDGE_CATALOG_DESIGN.md#r-10)。

## 接口契约 / 状态机

编译链见本文「推导」节：Schema access[] → AccessSpec → Probe → RetrievalPlan → CandidateRef → hydrate。Schema 是 Writer 入库的 `schema/*`。SEARCH 代数由 `RETRIEVAL.md` 拥有。参考实现可落在 `knowledge/reader/`、`retrieval/`、`index/`。


## 业界怎么读

写入按变化源拆，是主流。读并不跟文件/Aspect 一一对应：搜哪些、默认带哪些、权限走哪条 API，各家分开做。

**DataHub**  
写：MCP `(entityUrn, aspectName)` 一次一个 Aspect。  
读：GET Entity 可 `?aspects=` 只要子集；另有按 Aspect 的 GET。  
搜：OpenSearch 只编带 searchable 标注的 Aspect，不是整实体 JSON。`datasetProfile` / usage 是 timeseries，不进实体文档检索。
ACL：Policy 是另一类实体，不挂在 Dataset 上，也不当表字段搜。

**Unity Catalog**  
schema / owner / GRANT 三条 API。`DESCRIBE TABLE` 不带权限；权限是 `SHOW GRANTS` / `information_schema`。检索对象是表，不是 GRANT 行。血缘又是另一服务。最能说明：**权限不是表文档的检索面。**

**Atlas + Ranger**  
Atlas 搜实体与 classification。Ranger 是独立特权库，有自己的 cache / 拉取周期。Atlas 索引里没有 Ranger ACL 原文。对不上以 Ranger 为准。

**OpenMetadata**  
GET 可用字段投影。Policy 单独实体。`tableType` 等封闭枚举不抄；「权限不进表检索」这一点同。

重合：写的原子单位 ≠ 搜的文档形状 ≠ 权限的**强制**路径。分叉：DataHub 允许按 Aspect 拉；Unity 的 GRANT 根本不进表文档。我们把 GRANT 快照写成 `permissions` Aspect（知识，进 Canonical），检索面仍不把特权正文当表的 BM25；强制仍在 Ranger。

---

## 推导

**Reader 必须能按 Address 读。** `RESOLVE` / `READ` 可打到 Entity（拼装）或 `KnowledgeAddress`（单 Aspect / 单 Member）。这是 DataHub GET Entity vs GET Aspect。

**拼装是读策略，不是存储形状。** 默认 `READ(object_id)` 仍拼 `{ aspectName: value }`。调用方可 `include` / `exclude`。Authority 怎样编码 unit，调用方不必知道。

**检索另选编。** Projection 只定位 typed `CandidateRef`，命中后在同一 basis 回读完整 Canonical（K-19、K-25）。`AspectSelector` 只属于显式 READ；SEARCH 不用它裁结果。调用方信封是否含全文见 `PERMISSIONS.md` 交付链首段。默认编哪些字段看 `schema/*` 的访问声明（`DESCRIBE_SCHEMA`）。GRANT 正文不要当表的 `text` 面（Unity `DESCRIBE TABLE` 不含 GRANT 同构）；是否可检索只看这份知识自己的字段声明，不按 aspect 名做成第二种对象。Workspace 当前解析只提供成员 pin；RetrievalPlan 按请求扇出，不把联邦结果抄进一个大索引。

**`permissions` 是 SOURCE 知识，与 `structure` 同构。** Writer `COMMIT`、进 Canonical、可落后（所有外部 STATE 同步的通性）。真正 SELECT 放行在 Ranger / Unity / 内控；仓内 digest 不是 GT。Agent 读它是在读「源系统当时对谁开了」，不是在问「我能不能 `kc knowledge read`」——后者见 `PERMISSIONS.md`。GRANT 正文通常不声明 `text`，所以不是表文档的 BM25；需要过滤发现时给明确字段声明 `filter`，并在命中后回读完整对象。

**不把 Reader.search 当生产检索。** `Repository.search` 是整包 JSON 包含。生产走 RetrievalPlan + provider + hydrate；`AspectSelector` 只用于显式 READ。不新增第十二三个 Core Operation；`READ` 的 target 从「只有 Ref」扩成「Ref 或 Address」。

编译链（形状不在本文）：`schema/*` 声明 → `DESCRIBE_SCHEMA` → `AccessSpec` → Probe → `RetrievalPlan` → `CandidateRef` → 同一 basis hydrate。物化投影是某个 provider 对 AccessSpec 的实现，可重建，不是 Writer 的 IndexDefinition。

仓储约定：`permissions` 的 schema 通常不声明 `text`；强制仍在源系统。GRANT 快照进 Canonical 后，Catalog 不在查询路径上。

公开类型：`knowledge/`（Ref、Address、AspectSelector）、`knowledge/reader/README.md`、`retrieval/README.md`。