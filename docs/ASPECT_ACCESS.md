# Aspect：写入单元 vs 读/检索形态

日期：2026-08-20  
对照：DataHub、Unity Catalog、Apache Atlas / Ranger、OpenMetadata  
范围：**第 ② 层**（知识内容）的写粒度与读/检索形态；③ 的 AccessSpec / RetrievalPlan 从这里的字段访问声明编译。

不在本文：挂 git、Catalog pin（⓪ / ①，见 `LAYERS.md`）。Aspect 从 ② 才感知。

本文当前冻结的是 Snapshot Aspect 的读与检索。Aspect 通过 State/Stream Binding 指向墙外物化、以及 Snapshot 与动态 lane 的统一检索见 `LIVE_MATERIALIZATION.md`。

写冲突靠 Address（一单元一文件）已经定了。本文只回答：**读和检索要不要、以及怎样按 Aspect 走不同形态。** 完整读协议（ReadContext、LOG/DIFF/GET_PROVENANCE 分责、SEARCH 与 Projection、零结果）见 `KNOWLEDGE_CATALOG_DESIGN.md` 第 7 章。

---

## 业界怎么读

写入按变化源拆，是主流。读并不跟文件/Aspect 一一对应：搜哪些、默认带哪些、权限走哪条 API，各家分开做。

**DataHub**  
写：MCP `(entityUrn, aspectName)` 一次一个 Aspect。  
读：GET Entity 可 `?aspects=` 只要子集；另有按 Aspect 的 GET。  
搜：Elasticsearch 只编带 searchable 标注的 Aspect，不是整实体 JSON。`datasetProfile` / usage 是 timeseries，不进实体文档检索。  
ACL：Policy 是另一类实体，不挂在 Dataset 上，也不当表字段搜。

**Unity Catalog**  
schema / owner / GRANT 三条 API。`DESCRIBE TABLE` 不带权限；权限是 `SHOW GRANTS` / `information_schema`。检索对象是表，不是 GRANT 行。血缘又是另一服务。最能说明：**权限不是表文档的检索面。**

**Atlas + Ranger**  
Atlas 搜实体与 classification。Ranger 是独立特权库，有自己的 cache / 拉取周期。Atlas 索引里没有 Ranger ACL 原文。对不上以 Ranger 为准。

**OpenMetadata**  
GET 可用字段投影。Policy 单独实体。`tableType` 等封闭枚举不抄；「权限不进表检索」这一点同。

重合：写的原子单位 ≠ 搜的文档形状 ≠ 权限的**强制**路径。分叉：DataHub 允许按 Aspect 拉；Unity 的 GRANT 根本不进表文档。我们把 GRANT 快照写成 `permissions` Aspect（知识，进 Canonical），检索面仍不把特权正文当表的 BM25；强制仍在 Ranger。

---

## 决策

1. **KnowledgeRef 仍是对象。** `(repository, object_id)` 是长期身份。Aspect 是对象内的维护单元，不是另一套 Ref。
2. **Reader 必须能按 Address 读。** `RESOLVE` / `READ` 可打到 Entity（拼装）或 `KnowledgeAddress`（单 Aspect / 单 Member）。这是 DataHub GET Entity vs GET Aspect。
3. **拼装是读策略，不是存储形状。** 默认 `READ(object_id)` 仍拼 `{ aspectName: value }`。调用方可 `include` / `exclude`。FileGit 怎么拆文件，调用方不必知道。
4. **检索另选编。** Projection 只定位 typed `CandidateRef`，命中后在同一 basis 回读完整 Canonical（K-19、K-25）。`AspectSelector` 只属于显式 READ；SEARCH 不用它裁结果，裁剪交给更上层的上下文组装。默认编哪些字段看 `schema/*` 的访问声明（`DESCRIBE_SCHEMA`）。GRANT 正文不要当表的 `text` 面（Unity `DESCRIBE TABLE` 不含 GRANT 同构）；是否可检索只看这份知识自己的字段声明，不按 aspect 名做成第二种对象。Workspace 当前解析只提供成员 pin；RetrievalPlan 按请求扇出，不把联邦结果抄进一个大索引。
5. **`permissions` 是 SOURCE 知识，与 `structure` 同构。** Writer `COMMIT`、进 Canonical、可落后（所有外部 STATE 同步的通性）。真正 SELECT 放行在 Ranger / Unity / 内控；仓内 digest 不是 GT。Agent 读它是在读「源系统当时对谁开了」，不是在问「我能不能 `kc read`」——后者见 `PERMISSIONS.md`。GRANT 正文通常不声明 `text`，所以不是表文档的 BM25；需要过滤发现时给明确字段声明 `filter`，并在命中后回读完整对象。
6. **不把 Reader.search 当生产检索。** `Repository.search` 是整包 JSON 包含。生产走 RetrievalPlan + provider + hydrate；`AspectSelector` 只用于显式 READ。不新增第十二三个 Core Operation；`READ` 的 target 从「只有 Ref」扩成「Ref 或 Address」。
7. **Schema 声明访问语义，不声明索引。** `schema/*` 字段 `access[]` + `type` 是逻辑声明（BM25F / ES `text`≠`keyword` / DataHub `@Searchable.fieldType`）。第一版只允许 `text / filter / sort`：`text` 推出带 mode 的 `MATCH`；`filter` 推出 typed `EQ/IN/NEQ/EXISTS/MISSING`、数值/时间比较以及 string `PREFIX`；`sort` 推出一个显式业务排序。`PREFIX` 不新增 pattern lane，provider 是否高效或支持仍由逐请求 `Probe` 决定。不要在属性上枚举算子，也不要写 provider、analyzer、物理表或 index name。同一属性要全文又要精确，写 `access: [text, filter]`。完整 MVP 契约见 `LIVE_MATERIALIZATION.md` 第 5 节。
8. **`stored`、`summary`、`key` 不属于访问声明。** SEARCH 固定返回完整知识与版本，所以 stored/summary 既不决定可回答的查询，也不决定结果形状；provider 可私下采用类似优化。对象身份与精确读取已经由 `KnowledgeRef/Address` 表达；快速等值发现用 `filter`，不得再造含义不清的 `key`。
9. **字段引用是 `(schema, aspect, path)`。** 裸 path 不是全局字段身份。查询省略 schema/aspect 时，Planner 只能在无歧义时解析；有多个匹配必须展开成明确语义或报错，不能选择第一项。

编译（工作投影）：

```text
schema/* access[] + type     声明（知识对象，走 Writer）
  → DESCRIBE_SCHEMA          丢掉 provider/物理参数
  → AccessSpec               固定 (repository, commit, schema, aspect, path)
  → provider.Probe           对具体 clause 报 exact/superset/approximate/unsupported
  → RetrievalPlan            结合请求、provider inventory、budget/freshness 编译
  → CandidateRef             只携带 identity/basis/evidence
  → READ/Hydrate authority   返回完整 KnowledgeValue + KnowledgeVersion
```

物化投影只是某个 provider 对 AccessSpec 的实现，可以另有 `ProjectionSpec(provider, basis, accessDigest, physicalDigest)`，但它是可重建运行态，不是 Writer 的 IndexDefinition。`accessDigest` 与 `physicalDigest` 分开：Schema 不变而 analyzer/provider revision 改变时仍必须重建。不要无声明时把整包 JSON 当检索文档。对象 `schema_ref` 绑定用哪份 schema；`schema/*` 自身不进文档集。

不抄：DataHub 十五个 Aspect 全编进搜索；把 GRANT 当表字段 FTS；把 `permissions` 做成 Catalog 内第二套 ACL；通用 PATCH；运行时跟随 `latest`；在 schema 上罗列查询算子。

---

## 抽象

```text
KnowledgeRef          = (repository, object_id)          # 长期对象
KnowledgeAddress      = (kind, object_id, aspect?, member?)
AspectSelector        = { include?: name[], exclude?: name[] }

READ(ref, commit, selector?)     → 拼装后按 selector 裁
READ(address, commit)            → 单单元 Canonical
SEARCH                           → AccessSpec + provider capability 编译 RetrievalPlan
                                 → CandidateRef → READ 完整 Canonical + version
```

`KnowledgeValue.units`：对象由 Aspect/Member 组成时带上各单元 Address，投影据此裁剪。Entity blob 无 `units`，selector 不改值。

仓储场景约定：`permissions` 的 `schema/*` 不声明 `text`。无 `text` 声明就不会进 BM25；如需按授权快照过滤，给明确字段声明 `filter`，Planner 在 hydrate 后仍返回完整对象。

```text
Hive DDL / 作业定义 / Ranger GRANT
  → connector Preview → Writer COMMIT（SOURCE）
  → 同一 Repository 上的不同 Aspect
  → READ 拼装；SEARCH 只编声明过的 FieldRef，命中后回读完整对象
  → 引擎仍问源系统（Catalog 不在查询路径上）
```
