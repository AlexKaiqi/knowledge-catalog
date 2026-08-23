# Aspect：写入单元 vs 读/检索形态

日期：2026-08-20  
对照：DataHub、Unity Catalog、Apache Atlas / Ranger、OpenMetadata  
范围：**第 ② 层**（知识内容）的写粒度与读/检索形态；③ 的 IndexPlan 从这里的 AccessHints 编译。  
不在本文：挂 git、Catalog pin（⓪ / ①，见 `LAYERS.md`）。Aspect 从 ② 才感知。

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
4. **检索另选编。** Projection 只定位 `object_id`，命中后回读 Canonical（K-19）。默认编哪些字段看 `schema/*` 的 AccessHints（`DESCRIBE_SCHEMA`）；`AspectSelector` 仍可再裁。GRANT 正文不要当表的 `text` 面（Unity `DESCRIBE TABLE` 不含 GRANT 同构）；编不编进 IndexPlan 看这份知识自己的 Hints，不要按 aspect 名做成第二种对象。Workspace 当前解析上的配方是 `IndexPlan`（每成员一份），不是把联邦结果抄进一个大索引。
5. **`permissions` 是 SOURCE 知识，与 `structure` 同构。** Writer `COMMIT`、进 Canonical、可落后（所有入站镜像的通性）。真正 SELECT 放行在 Ranger / Unity / 内控；仓内 digest 不是 GT。Agent 读它是在读「源系统当时对谁开了」，不是在问「我能不能 `kc read`」——后者见 `PERMISSIONS.md`。消费方可以用这份知识过滤候选，绕不过引擎。检索面走 AccessHints：GRANT 正文通常不声明 `text`，所以不是表文档的 BM25；声明了 `filter` 就和其他 Aspect 一样进 IndexPlan。
6. **不把 Reader.search 当生产检索。** `Repository.search` 是整包 JSON 包含。生产走 Projection + `AspectSelector`。不新增第十二三个 Core Operation；`READ` 的 target 从「只有 Ref」扩成「Ref 或 Address」。
7. **索引声明在属性上，写检索面，不写算子表。** `schema/*` 字段 `access[]` + `type` 是声明（BM25F / ES `text`≠`keyword` / DataHub `@Searchable.fieldType`）。`MATCH`/`EQ`/`IN`/… 是查询用法，由 `AllowsOp` 从声明推出。不要在属性上枚举 `eq, in, gt`。同一属性要全文又要精确，写 `access: [text, filter]`（两张脸，不是两套谓词许可）。

编译（工作投影）：

```text
schema/* access[] + type     声明（知识对象，走 Writer）
  → DESCRIBE_SCHEMA          丢掉 HNSW/GIN
  → IndexSpec / IndexPlan    配方（digest 变 ⇒ 重建）
  → index/ SQLite            只抽声明过的 path；无 text Hint 则 MATCH 为 CAPABILITY_UNSATISFIED
  → DESCRIBE_INDEX           露出本次 basis 上的 fields / lanes
```

不要另做 IndexDefinition 写面。不要无 Hint 时把整包 JSON 当检索文档。对象 `schema_ref` 绑定用哪份 schema；`schema/*` 自身不进文档集。

不抄：DataHub 十五个 Aspect 全编进搜索；把 GRANT 当表字段 FTS；把 `permissions` 做成 Catalog 内第二套 ACL；通用 PATCH；运行时跟随 `latest`；在 schema 上罗列查询算子。

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

仓储场景约定：`permissions` 的 `schema/*` 不声明 `text`；第一刀 `Projection.build(..., { exclude: ["permissions"] })` 仍可用。selector 先空着也没关系——无 Hint 就不会进 BM25。消费方要过滤候选时再 `READ` 这份 Aspect（或 `include`）。

```text
Hive DDL / 作业定义 / Ranger GRANT
  → connector Preview → Writer COMMIT（SOURCE）
  → 同一 Repository 上的不同 Aspect
  → READ 拼装；SEARCH 只编 AccessHints 声明过的 path
  → 引擎仍问源系统（Catalog 不在查询路径上）
```
