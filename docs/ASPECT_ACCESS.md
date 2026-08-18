# Aspect：写入单元 vs 读/检索形态

日期：2026-08-18  
对照：DataHub、Unity Catalog、Apache Atlas / Ranger、OpenMetadata  
范围：协议读 API（Reader）与 Projection。写粒度见 `KNOWLEDGE_CATALOG_DESIGN.md` A.3 / Entity–Aspect。  
场景侧 Table/Column/ETLTask 切分见 `.scenes/data-warehouse/.data/decisions/physical-layer-industry.md`（gitignored）。

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

重合：写的原子单位 ≠ 搜的文档形状 ≠ 权限的服务路径。分叉：DataHub 允许按 Aspect 拉；Unity 权限根本不进 Catalog 文档。

---

## 决策

1. **KnowledgeRef 仍是对象。** `(repository, object_id)` 是长期身份。Aspect 是对象内的维护单元，不是另一套 Ref。
2. **Reader 必须能按 Address 读。** `RESOLVE` / `READ` 可打到 Entity（拼装）或 `KnowledgeAddress`（单 Aspect / 单 Member）。这是 DataHub GET Entity vs GET Aspect。
3. **拼装是读策略，不是存储形状。** 默认 `READ(object_id)` 仍拼 `{ aspectName: value }`。调用方可 `include` / `exclude`。FileGit 怎么拆文件，调用方不必知道。
4. **检索另选编。** Projection 只定位 `object_id`，命中后回读 Canonical（K-19）。编进 FTS 的文本用同一套 `AspectSelector`，默认由调用方声明。ACL / 特权投影 **不得** 进 lexical 索引。
5. **`permissions` 是特权库的 cache，不是检索文档。** 有 basis + lag；过期重建；对不上以 Ranger / Unity / 内控为准。可以不写进 Catalog；写了也：不进 FTS、默认拼装可 `exclude`。不要做成第二 ACL 权威。
6. **不把 Reader.search 当生产检索。** `Repository.search` 是整包 JSON 包含。生产走 Projection + `AspectSelector`。不新增第十二三个 Core Operation；`READ` 的 target 从「只有 Ref」扩成「Ref 或 Address」。

不抄：DataHub 十五个 Aspect 全编进搜索；把 GRANT 当表字段 FTS；通用 PATCH；运行时跟随 `latest`。

---

## 抽象

```text
KnowledgeRef          = (repository, object_id)          # 长期对象
KnowledgeAddress      = (kind, object_id, aspect?, member?)
AspectSelector        = { include?: name[], exclude?: name[] }

READ(ref, commit, selector?)     → 拼装后按 selector 裁
READ(address, commit)            → 单单元 Canonical
SEARCH / Projection.build        → 用 selector 生成 value_text
                                 → 命中后 READ Canonical（可再用同一 selector hydrate）
```

`KnowledgeValue.units`：对象由 Aspect/Member 组成时带上各单元 Address，投影据此裁剪。Entity blob 无 `units`，selector 不改值。

仓储场景约定：`Projection.build(..., { exclude: ["permissions"] })`。第一刀不写 permissions，selector 先空着也没关系。
