# Knowledge Catalog 规模化存储与访问设计

日期：2026-08-27
状态：实施中（Relation authority locator 已废止）

本文定义百万级数仓表、每天约 10,000 次逻辑表变化下的实现改造。它只讨论架构、数据模型、接口和迁移；负载、执行方法和验收门槛见 [`SCALE_BENCHMARK.md`](SCALE_BENCHMARK.md)。

---

## 1. 结论

当前实现不能通过“优化几条 SQL”扩展到目标规模。必须同时替换五条基础通路：

1. **Dolt scale Repository 不再用 `kc_files` 保存知识。** `kc_units` 是唯一 Canonical 内容表；`kc_objects` 只是同 commit 的对象清单。Relation endpoint/type/role 不在 authority 建定位表。
2. **Writer/Reader 不再经过全树解释。** Dolt 由 layer ② `knowledge/dolt` 直接实现增量 ChangeSet、点读、分页和历史能力；Gitea 使用文件 codec。
3. **每个源事务或逻辑表变更立即 commit。** 不增加固定时间窗口，不用周期扫描换吞吐；只有队列已经积压时才允许零等待合并，且必须保留每个源事件证据。
4. **公开 LIST、Relations、checkout/rebuild 改成分页或流式合同。** “返回整个 Repository”的 API 不进入 scale profile。
5. **幂等账本和 Projection Controller 改为按键持久化、异步追赶。** Writer 不再重写全部 `writer.json`，也不在返回 receipt 前同步刷新 OpenSearch。

历史默认保留。持续运行能力不押注单个 Dolt commit graph 无限增长，而是使用 **Repository generation**：active generation 到达实测安全线后切新仓，旧仓只读归档。旧 `{repository, commit}` 坐标仍可显式挂载读取，普通应用只暴露 active generation。

---

## 2. 目标与边界

首个目标部署：

- 1,000,000 张物理表，典型每表 30 列；
- 物理层保存 Table、Column、DataJob、Schema 和 Relation；
- 语义层在独立 Repository 保存 Metric、SemanticModel、Dimension/Measure 等知识；
- 每天约 10,000 次逻辑表变化，平均约 `0.116/s`；
- 验收 `100x` 突发，即约 `11.6 logical changes/s`；
- 普通读固定在 Workspace 本次解析出的 commit；
- 全文/条件检索使用 OpenSearch 候选，命中后回读同一 commit 的 Canonical；
- 五年不删历史约产生 18,250,000 个 steady-state commit，因此长期档位按 20,000,000 commit 设计。

不做：

- 不把源事件流登记为 Repository 或 Workspace 成员；
- 不新增 APPEND/PATCH Surface；
- 不让 Catalog 感知 `object_id`、Aspect 或 SQL 表；
- 不把 OpenSearch、对象清单、relation endpoint 表当成知识正文；
- 不用定时 FULL scan 作为正常一致性通路；
- 不承诺一个物理 Dolt database 永不分代。

---

## 3. 当前实现审计

### 3.1 热路径实际复杂度

| 通路 | 当前代码 | 当前行为 | 规模后果 |
|---|---|---|---|
| Writer | `knowledge/writer/treecodec.go` | 每个 ChangeSet 先 `ListFiles`，再逐文件 `ReadFile` 重建全树 | 单个小 PUT 是 `O(全仓 units)` |
| Schema 校验 | `knowledge/writer/schema.go` | 每个已有 schema ref 再重建知识树 | 带 Schema 的正常写同样全扫 |
| Reader | `knowledge/reader/repository_service.go` | Read/Resolve/Address/List 通过 `readKnowledgeTree` 全量解释 | 点读无法随对象数扩展 |
| Dolt ReadFile | `snapshot/dolt/tree.go` | `snapshotFiles` 先读取该 commit 的整张 `kc_files` | 单 path 读取也是全表读取 |
| Relations | `index.RelationsAt` | exact-basis Retriever 候选页后 `ReadMany` 回读 | 无索引明确缺能力，不扫描 authority |
| Schema 描述 | `knowledge/reader/schema.go` | 为列出 Schema 调用 `repo.List()` | 少量 Schema 被数千万普通对象淹没 |
| 变化识别 | `knowledge/changed.go` | 无 FastChanges 时比较两个完整 List | Projection 增量可能退化为双全量 |
| Rebuild | `index/sync.go` | OpenSearch 已走 500-doc streaming generation；非 streaming provider 仍有全量兼容分支 | scale profile 必须拒绝非 streaming provider，不能进入全量分支 |
| OpenSearch Apply | `retrieval/opensearch/projection.go` | 已改为 bulk `refresh=wait_for` 并用 bulk item result 维护 control count | object diff 与编译输入仍是整批 slice，超大 backlog 尚未端到端分页 |
| Hook | `snapshot/event.go`、`cli/sidecar.go` | Writer 线程同步调用 Index Ensure | 检索故障或 rebuild 会拖住 receipt |
| 幂等账本 | `snapshot/commandlog/*` | 每次 reserve/complete 都重写全部 entries，并保存完整 ChangeSet | 时间、内存和文件大小随总 commit 线性增长 |
| LIST/checkout | `knowledge/reader/serving.go`、`cli/workspace_checkout.go` | 一次返回/物化整个 Workspace | API 响应和内存不可界定 |
| Dolt transport | `snapshot/dolt/command.go` | 每个操作启动 CLI，缺 binary 时启动 Docker | 无法稳定提供低延迟服务 |
| 数仓 Collector | `.data/data-warehouse/connector/collector.py` | signal 后仍执行 tables/全部 columns/jobs FULL scan | 只是“事件触发全扫”，不是 targeted pull |

### 3.2 原方案为什么不再采用

上一版建议保留 `kc_files` 为 Canonical，再维护 Address/Relation 伴随表。该方案在真实代码上不合理：

- 同一知识被保存为 frontmatter 文件和结构化行，写放大且存在双份事实；
- native 写仍要解析、生成并批量更新文件正文；
- `path` 只是表示提示，却继续承担规模存储主键；
- 迁移、完整性标记和 raw tree 绕写会长期增加两套分支；
- 解决了定位后，幂等账本、分页、同步索引等更早的瓶颈仍未解决。

因此 scale Repository 采用新的物理编码，不再修补 `kc_files`。`kc_files` 只保留为 `snapshot/dolt` 的 layer ⓪ conformance 表示和旧仓读取能力。

---

## 4. 目标架构

```text
源 DDL/元数据事件
  -> provider event consumer
  -> 按 table family 拉当前态
  -> keyed checkpoint 取该 family 的已发布 Address
  -> connector.Preview(PATCH/局部 RECONCILE)
  -> Writer COMMIT
       -> keyed command ledger reserve
       -> knowledge/dolt 原生事务
            kc_units                 Canonical
            kc_objects               object manifest
            Dolt commit/ref
       -> command ledger complete
       -> durable projection target = new commit
  -> receipt 立即返回

Projection Controller
  -> from basis 到 desired target 的 object diff
  -> 分页 ReadMany + compile
  -> OpenSearch bulk
  -> publish basis

Consumer
  -> ResolveWorkspace 一次
  -> native point/page read AS OF pinned commit
  -> SEARCH/RELATIONS 先走 exact-basis Retriever，再 ReadMany 回读 Canonical
```

一个 Repository 对应一个 Dolt database。物理知识和语义知识仍按治理/写责任拆仓，由 Catalog 的 Workspace 组合，不因容量在 Catalog 内做覆盖或正文复制。

---

## 5. 分层与接口改造

### 5.1 `snapshot.Store` 保持纯净

`snapshot.Store` 仍只定义 commit/ref/CAS/archive。Catalog 继续只依赖这一层，不加入知识或 SQL 方法。

`snapshot/dolt` 保留为通用 layer ⓪ TreeStore 参考实现，但不再是 scale profile 的知识读写入口。

### 5.2 新增 layer ② `knowledge/dolt`

`knowledge/dolt.Repository` 是组合对象：

- 对外实现 `snapshot.Store`，ref/merge/archive 委托 Dolt backend；
- 原生实现 `knowledge.Repository` 和批量读；
- 实现 Writer 可发现的增量 ChangeSet 能力；
- 实现精确知识读取、SchemaLocator、FastChanges 和对象历史能力；
- 不实现 `snapshot.TreeStore`，避免 raw path 写绕过知识不变量。

`knowledge.ReadStore` 只保留 point read，不包含对象枚举。底层全 Snapshot 遍历进入 `knowledge/maintenance` 的显式 SPI；Relation、Schema 发现分别使用小能力接口，避免让消费 Reader 对所有 Repository 强制扫描。建议形状：

```go
type ReadStore interface {
    // Resolve / Read / ResolveAddress / ReadAddress / Provenance / Log / Diff ...
}

type ChangeStore interface {
    ApplyKnowledgeChange(commandID string, cs ChangeSet) (kernel.CommitID, error)
}

type SnapshotScanner interface {
    ScanSnapshotPage(commit kernel.CommitID, req ScanRequest) (ScanPage, error)
}

type SchemaLocator interface {
    SchemaObjectIDs(commit kernel.CommitID) ([]ObjectID, error)
}
```

`SnapshotScanner` 只供 projection rebuild、迁移、显式 export 和 conformance；不能被 READ、SEARCH、Schema 或 Relations 当 fallback。Relations 合同位于 `retrieval/`，continuation 绑定 provider、repository、basis、query 与 generation，候选在同一 basis 回读 Canonical。

`Reader.Wrap` 的选择顺序改成：

1. Store 已实现 `knowledge.Repository`：使用原生能力；
2. Store 只有 `snapshot.TreeStore`：使用文件解释器；
3. 两者都没有：返回 capability error。

Writer 同样先选择 `knowledge.ChangeStore`，否则走现有 tree codec。COMMIT/PROPOSAL 的公开语义、CAS、错误码和 Receipt 不变。

Gitea 可以为维护任务提供 `ScanSnapshotPage`，但该能力不进入 `knowledge.Repository`、Serving、CLI 消费面或 Knowledge HTTP API。OpenSearch relation projection 或 Dolt SchemaLocator 缺失时必须明确返回 capability error。

### 5.3 抽出 provider-neutral unit 代数

当前 PUT/REMOVE 的 sibling layout、precondition、entity remove、Schema/ValueSource 继承规则与文件序列化混在 `internal/repofile.Apply` 中。应拆成：

- provider-neutral `Unit`/`ObjectState` 和 Operation apply；
- `internal/repofile` 只负责文件编码、path hint 和 tree changes；
- `knowledge/dolt` 负责行编码和 SQL changes。

两种 provider 对同一 ChangeSet 序列必须产生相同 KnowledgeValue、Resolution、Diff 和 Provenance，这是迁移的核心差分测试。

### 5.4 Dolt backend 机制

连接、branch/ref、commit、merge、archive 和 SQL transaction 下沉到不认识知识语义的 `internal/doltdb`。`snapshot/dolt` 与 `knowledge/dolt` 共用它，但只有后者定义知识表。

scale profile 使用 `dolt sql-server` 的 MySQL-compatible 长连接和连接池；CLI/Docker-per-call 只用于本地 conformance。Dolt 官方支持 SQL Server、历史 `AS OF`、row diff 和 SQL `DOLT_COMMIT()`，分别见 [SQL Server](https://www.dolthub.com/docs/sql-reference/server/)、[Querying history](https://www.dolthub.com/docs/sql-reference/version-control/querying-history/)、[SQL functions](https://www.dolthub.com/docs/sql-reference/version-control/dolt-sql-functions/) 与 [SQL procedures](https://www.dolthub.com/docs/sql-reference/version-control/dolt-sql-procedures/)。并发设计不假设 `SELECT FOR UPDATE`，其当前支持状态见 [Supported statements](https://www.dolthub.com/docs/sql-reference/sql-support/supported-statements/)。

生产固定 Dolt 版本；不得使用 `dolthub/dolt:latest`。

---

## 6. Dolt 原生物理模型

### 6.1 唯一 Canonical：`kc_units`

一行对应一个 Address。示意字段：

```sql
kc_units (
  address_hash        BINARY(32) PRIMARY KEY,
  object_hash         BINARY(32) NOT NULL,
  object_id           TEXT NOT NULL,
  address_kind        VARCHAR(16) NOT NULL,
  aspect_name         TEXT,
  member_key          TEXT,
  value_json          LONGBLOB NOT NULL,
  value_digest        BINARY(32) NOT NULL,
  schema_ref          TEXT,
  value_source_json   LONGBLOB,
  provenance_json     LONGBLOB,
  declaration_digest BINARY(32) NOT NULL,
  path_hint           TEXT,
  KEY by_object (object_hash)
)
```

`value_json` 使用 KC canonical JSON bytes；不依赖数据库 JSON 重排来计算 digest。`path_hint` 仅用于导出可读树，不参与身份。

hash 只是物理键。每次命中必须比对完整 Address/ObjectID；同 hash 不同完整身份时失败关闭，不静默覆盖。

### 6.2 版本化对象清单：`kc_objects`

```sql
kc_objects (
  object_hash         BINARY(32) PRIMARY KEY,
  object_id           TEXT NOT NULL,
  object_kind         VARCHAR(16) NOT NULL,
  unit_count          INT NOT NULL,
  object_digest       BINARY(32) NOT NULL,
  declaration_digest  BINARY(32) NOT NULL,
  is_schema           BOOLEAN NOT NULL,
  KEY schema_page (is_schema, object_hash)
)
```

它支持对象存在判断、keyset pagination、Schema 枚举和 object-level diff。正文只在 `kc_units`；清单与 units 不一致时 Reader 失败关闭。

### 6.3 Relation endpoint 投影

每个 Canonical Relation 在 layer ③ 生成一个 OpenSearch 文档。`relation_endpoints` 使用 nested `{role, repository, object_id}` 映射；authority 不保存 endpoint locator，也不提供关系枚举或过滤方法。消费路径只检查 READY exact basis，不同步构建投影。

### 6.4 Layout 元数据

`kc_layout` 保存格式版本、创建工具版本和不变量版本。所有知识表在同一 Dolt commit 中更新，不需要上一版的 `complete` 双写标记。

scale Repository 中不创建 `kc_files`。这避免“文件正文 + 结构化正文”双份权威。

---

## 7. 数仓对象模型调整

当前夹具为每条 `table -> column` 边创建一个 Relation object。30 列时每表约 62 objects、63 units，纯 containment Relation 占近一半对象和 OpenSearch 文档。

改为有界的 grouped relation：

- 每张表一个 `schema contains table` Relation；
- 每张表一个 `table contains columns` Relation，包含一个 container endpoint 和该表所有 column member endpoints；
- Column 仍是独立 object，继续支持 lineage、权限和独立引用；
- DataJob lineage 同理按一个 job/一次加工的输入输出集合分组，不按单 edge 建 object。

grouped relation 必须有 endpoint 上限。首版建议每个 Relation 最多 256 endpoints；超过上限的超宽表进入稳定 bucket 分片，relation id 含 table id 与 bucket id。跨越分片阈值会产生一次显式模型迁移，不能让单个 Relation 随列数无界增长。

典型 30 列表由此变为：

```text
objects = 1 table + 30 columns + 2 relations = 33
units   = 2 table aspects + 30 column aspects + 2 relations = 34
relation endpoints = 2 + 31 = 33
```

这不是存储层偷偷折叠身份，而是提供方显式改变 Relation object 粒度；Canonical Relation 仍符合协议。若某类关系有独立审核、来源或生命周期，仍可保持一 edge 一 object。

纯 containment Relation 默认不进入全文索引。Relation 查询由 native endpoint locator 完成；只有 Schema 明确声明检索字段的 Relation 才生成 OpenSearch 文档。

---

## 8. 写路径

### 8.1 一次 native commit

1. Writer 完成 ChangeSet、Provenance、ValueSource 和 Relation 形状校验。
2. keyed command ledger 原子 Reserve `command_id`。
3. 取得该 Repository 的 active-writer lease；检查 target ref 等于 `ExpectedTargetCommit`。
4. 按 touched object 分组，`AS OF BaseCommit` 批量读取这些对象的 manifest 和 units。
5. 用 provider-neutral unit 代数应用 PUT/REMOVE/precondition。
6. 对已有 Schema 做 object point read；同 ChangeSet 的 Schema 从 batch 解析。
7. 计算 units、object manifest 和 relation endpoints 的增删改。
8. 在一个 SQL transaction 写三张表，提交前再次检查 ref。
9. `CALL DOLT_COMMIT(...)` 产生一个 Dolt commit，并把 `command_id` 写入 commit trailer。
10. command ledger 写入 Receipt，随后把 projection desired target 持久化。
11. 返回 Receipt；不等待 OpenSearch refresh。

所有表要么随同一个 commit 可见，要么都不可见。不得在事务外先写 locator。

### 8.2 并发与 CAS

Dolt 当前不提供可依赖的行级 `SELECT FOR UPDATE` 语义，因此 scale Writer 对每个 active Repository 使用单 active-writer lease。多实例部署由共享 lease/leader election 保证；同一进程内再用短队列串行 target ref。COMMIT、PROPOSAL、merge、archive 等所有 ref mutation 都必须经过同一 lease，不能只保护 Writer COMMIT。

这不是吞吐瓶颈的默认假设：目标平均仅 `0.116 commit/s`，主要需承受短时 `11.6/s`。若压测证明单 commit 延迟不足：

- 先优化 SQL batch、prepared statements 和 commit 固定开销；
- 队列已有多条消息时，可立即合并当前已到达事件，不启动等待 timer；
- 合并后必须保存全部 event IDs/source revisions，不能把多源证据压成一个不可追踪时间戳；
- 不通过每 N 秒 group commit 牺牲低流量时效。

### 8.3 幂等账本

`writer.json` 只保留 local profile。scale profile 使用按键 ACID Store（参考实现可用 bbolt；服务部署也可接共享控制数据库）：

```text
command_id -> digest, surface, repo/ref, base/expected,
              status(PENDING/APPLIED), result commit, receipt
```

不再保存完整 Operations，也不在启动时把全部历史载入内存。Reserve/Complete 都是单 key transaction。

若进程在 Dolt commit 后、ledger Complete 前崩溃，恢复器用 pending entry 的 expected parent 和 HEAD commit trailer 判定是否已应用；匹配则重建 Receipt，不匹配则失败关闭等待人工核对。

---

## 9. 读、变化与历史

### 9.1 精确读

```text
Read(object, commit)
  -> kc_objects AS OF commit WHERE object_hash=?
  -> 校验完整 object_id
  -> kc_units AS OF commit WHERE object_hash=?
  -> 校验 unit_count/digests
  -> Assemble
```

ReadAddress 直接按 `address_hash`；ReadMany 使用有上限的 hash batch。目标复杂度只与目标对象 units 数相关。

### 9.2 分页

`kc_objects` 使用 `(object_hash, object_id)` keyset pagination。continuation 绑定 repository、commit、filter 和 last key；Workspace continuation 还绑定 ResolvedWorkspace digest 与 member index。

公开 LIST/Relations 必须有 server-side 最大 page size。`knowledge.Repository`、Serving、CLI 和 HTTP 不再暴露无界 `List()`；local conformance 如需全量值，也必须由测试 helper 循环消费 pages。

### 9.3 Relation

authority 不保存 endpoint 倒排表，也不提供 type/role/direction 枚举。Relation object 的保留字段进入
layer ③ 投影；消费请求先要求指定 commit 的 projection READY，再由 Retriever 分页返回 CandidateRef，
随后仅对当前页做同 basis `ReadMany` 和 Canonical 复核。无 provider 或 exact-basis projection 时明确失败，
不得扫描 Dolt/Gitea 或在请求内追赶投影。

### 9.4 Schema

`SchemaReadStore.ListSchemas` 通过 `kc_objects.is_schema` 枚举，`specAtCommit` 不再调用全仓 List。Schema 数量小，可在 `(repo, commit, schema digest)` 上缓存编译后的 AccessSpec。

### 9.5 变化识别

`FastChangedObjectIDs(from,to)` 对 `kc_objects` 使用 Dolt `dolt_commit_diff_kc_objects` 或 `DOLT_DIFF()`，只取 from/to object IDs。Dolt 官方说明 row diff 基于版本化表并支持两个 commit 间的合并 diff；不用逐 commit 回放。

### 9.6 Object LOG

Object LOG 使用 `dolt_diff_kc_objects` 按 object hash 过滤、按 commit 排序和 limit；Diff 仍是两个固定 commit 的点读。不得先拉完整 commit log，也不得逐 commit 重建 Repository。

历史接口始终有 limit/continuation。普通消费者默认无 archived generation 的访问授权。

---

## 10. Projection Controller

当前同步 Hook 改成 durable desired-target 模式：

1. Writer 成功后只把 `{repository, desiredCommit}` 原子写入 controller store；同 repo 新事件覆盖为更后的 target。
2. Controller 从当前 OpenSearch basis 到 desired target 求 object diff。
3. 分页 ReadMany、compile、bulk apply。
4. 一次发布新的 projection basis。
5. 进程启动时比较 projection basis 与 Repository HEAD；不同立即追赶。这是坐标比较，不是源系统扫描。

OpenSearch provider 接口从 `Rebuild([]CompiledDoc)` 改成 generation writer：

```text
BeginGeneration -> WriteBatch* -> CatchUp* -> Publish
                                     \-> Abort
```

增量 Apply 不再每个 commit 执行全索引 count 或显式强制 `_refresh`。完整 count/校验在 rebuild 发布与周期运维任务中执行；当前最后一个 bulk 使用 `refresh=wait_for` 等待正常 refresh policy，指标直接记录 commit-to-searchable lag。

当前参考实现已经移除增量 `_count` 与显式 `_refresh`，并把 shard/replica/refresh 配置纳入 `physicalDigest`；暖 rebuild 也不会把旧 READY generation 改成 BUILDING。仍未完成的是分页 object diff → bounded compile → streaming incremental apply，以及跨进程 Controller lease/checkpoint。因此这些改动消除了固定的总索引扫描成本，但不等于 S5 已通过。

Schema AccessDigest 或 physical mapping 变化时构建新 generation；旧 active index 在 Publish 前持续服务。

---

## 11. 事件驱动 Collector

### 11.1 稳态

当前 FULL collector 拆为：

```text
event(event_id, source_revision, schema, table, operation)
  -> adapter.describe_table(schema, table)
  -> adapter.describe_columns(schema, table)
  -> 必要时拉该 table family 的 lineage/job
  -> checkpoint.get(family_key)
  -> translate one family
  -> connector.Preview(PATCH 或局部 RECONCILE)
  -> Writer COMMIT
  -> checkpoint.put(family_key, source_revision, addresses, digests, receipt)
```

Checkpoint 是按 source family 的 keyed store，不再是一份包含数千万 Address 的 JSON。删除事件用旧 family checkpoint 生成 REMOVE；rename 必须携带 old/new key 或由 source revision 映射解决。

重复事件复用稳定 command id；乱序事件按 source revision/LSN 拒绝；cursor 只在 Receipt 成功后推进。

### 11.2 Bootstrap

Bootstrap 与稳态是两条路径：

- 在 candidate ref 上按 source key 做 keyset page；
- 每批 500–5,000 tables，具体大小由内存/事务压测确定；
- 先记录 source watermark，bootstrap 期间缓冲增量事件；
- 基础遍历结束后只重拉缓冲事件触及的 families；
- 校验对象/units/relation counts 与抽样 digest；
- 经现有 Preview/Gate/Merge 一次发布 candidate；
- 不在 main 上暴露半完成 bootstrap。

FULL reconcile 只用于首次 bootstrap、明确 event gap 修复和管理员校验，不承担日常时效。

---

## 12. Repository generation 与归档

### 12.1 为什么需要 generation

每天 10,000 commit 意味着：

| commit 数 | 对应持续时间 |
|---:|---:|
| 1,000,000 | 100 天 |
| 5,000,000 | 500 天，约 1.37 年 |
| 10,000,000 | 约 2.74 年 |
| 20,000,000 | 约 5.48 年 |

固定写“支持 5m commit”不足以覆盖长期运行。另一方面，也不应把无限 commit graph 当成未经验证的前提。

### 12.2 安全线

压测得到单 generation 的最大通过档位 `Hmax` 后，生产 rollover 线不高于 `50% * Hmax`，并同时满足磁盘、备份、恢复和点读退化门槛。例如 Hmax=20m 时，默认 10m 左右主动切换。

### 12.3 切换

1. 在新 Repository generation 的 candidate ref bootstrap 旧 active HEAD 当前态。
2. 记录切换 watermark，追赶此后事件。
3. 校验同一 object_id 的当前值与声明 digest。
4. 原子更新 WorkspaceDefinition 选择新 Repository。
5. 旧 Repository 执行现有 `archive-repo`，停止写入。
6. Projection 为新 Repository 构建/切换；旧 projection 可删除并按需重建。

旧 generation 保留全部 Dolt commits。当前 `Archive()` 只是只读标记，不减少 commit graph 或磁盘；若需要冷存，运维层把整个旧 Dolt database 备份/迁移出热 SQL Server，并在显式历史访问时重新挂载。不得把 `dolt gc` 描述成已达 commit 的历史压缩。

普通应用只解析 active Workspace；管理应用可限制或拒绝 archived repo。这样总历史不删除，同时热写和普通读只面对有界 generation。

---

## 13. 迁移策略

不做 `kc_files` 到 native rows 的长期双写，也不做第一次 miss 时隐式全仓迁移。

当前尚无正式生产大仓时：

1. 新建 native Dolt Repository generation；
2. 从当前 provider/source 重新 bootstrap；
3. 用同一组 conformance 与数仓 cases 做差分；
4. 更新 WorkspaceDefinition；
5. 旧测试仓归档或删除（是否删除由人工决定）。

若以后迁移已有权威仓：

1. 固定旧 commit；
2. 流式读取旧文件，不一次载入；
3. 解码为 Operations，分批写新 candidate；
4. 比对 object/unit/relation count、全量 digest accumulator 和抽样正文；
5. 追赶切换期间的新事件；
6. Gate 后切 Workspace；
7. 旧 Repository 保留原 commit 坐标。

native layout 变化也通过新 generation 迁移，不在数千万行 active 表上执行高风险原地 schema rewrite。

---

## 14. 实施顺序

### Phase 0：先拆除与 Dolt 无关的硬阻塞

- `snapshot/commandlog` 增加 keyed Store，scale profile 不再用全文件 Save/Load；
- command entry 缩为 replay basis，不保存完整 ChangeSet；
- `ReadStore.List`、Relations、Serving、HTTP 改为 page contract 和 continuation；
- checkout/export 改为 page iterator；
- ProjectionMaintainer 改为 streaming generation/batch；
- Hook 改为 durable desired-target，Writer 不同步跑索引。

### Phase 1：抽出 unit 代数

- 从 `internal/repofile` 拆出 provider-neutral ObjectState apply/assemble；
- Gitea/Dolt conformance 全部保持，provider-independent 单测使用私有 memory fake；
- 增加 operation sequence property/differential tests。

### Phase 2：Dolt backend 与 native Repository

- 新增 `internal/doltdb` 长连接、事务、ref/commit/merge；
- 新增 `knowledge/dolt` 和四张表；
- CLI `dolt` scale driver 打开 `knowledge/dolt`，旧 TreeStore 明确为 legacy/conformance；
- 实现 point read、batch read、Schema list、object diff/history；Relation page 属于 layer ③；
- 更新 `docs/LAYERS.md`、`docs/STORE_ADAPTERS.md` 和 `internal/arch` 守卫。

### Phase 3：数仓 provider 改模与 targeted collector

- containment relation 改为 grouped relation；
- Adapter 增加 table-family 点查，不再 `describe_all_columns`；
- checkpoint 改 keyed family state；
- bootstrap candidate、watermark、增量 catch-up；
- 覆盖 create/alter/drop/rename、重复、乱序和 event gap。

### Phase 4：Projection 与完整规模验收

- Dolt object diff 驱动 OpenSearch 增量；
- Relation retrieval 必须依赖 exact-basis OpenSearch projection，不允许 native fallback；
- streaming rebuild + catch-up + atomic publish；
- 先执行 1m-table 校准，再执行 S5（2m tables / 106m 主体 objects）和 20m-commit history；
- 按实测确定 generation rollover 线。

### Phase 5：归档运行演练

- active -> new generation 切换；
- 旧 generation 只读、detach、restore、显式旧 pin 读取；
- 验证应用授权不暴露 archived repo；
- 固化备份/RPO/RTO 与容量报警。

---

## 15. 失败语义

| 情况 | 行为 |
|---|---|
| manifest 与 units 不一致 | 失败关闭，返回 integrity/capability error |
| hash 相同但完整身份不同 | 失败关闭，不覆盖 |
| 任一 Operation/precondition 失败 | 整批无 Dolt commit |
| target ref 被推走 | `NON_FAST_FORWARD`，按新 HEAD 重拉 touched families 并重做 diff |
| 同 command id 不同 digest | `IDEMPOTENCY_CONFLICT` |
| commit 已成功但 ledger 仍 PENDING | 用 expected parent + HEAD trailer 恢复；无法证明则停止该 repo 写入 |
| OpenSearch 不可用 | Canonical commit 正常返回；desired target 保留并异步追赶 |
| Projection event 丢失 | 启动/恢复时比较 basis 与 HEAD，直接求 commit diff |
| 源事件重复 | checkpoint + command id 去重，不产生额外知识 revision |
| 源事件乱序 | source revision/LSN 拒绝旧观察 |
| event gap | 暂停受影响 source partition，执行明确的 targeted/full recovery |
| archived repo 普通访问 | 权限层拒绝；显式管理挂载后才允许 |

---

## 16. 决策记录

- **S-01**：scale Dolt 以 `kc_units` 为唯一 Canonical，不再保存 `kc_files` 正文。
- **S-02（废止）**：Dolt 不再维护 `kc_relation_endpoints` 或 `kc_objects.relation_type`；旧结构打开时安全删除。
- **S-03**：规模能力位于 layer ② `knowledge/dolt`；`snapshot.Store` 和 Catalog 不认识知识语义。
- **S-04**：Gitea 保留文件 codec；Dolt/Gitea 共享 unit 代数和 conformance。
- **S-05**：每个源事务/逻辑表变化立即 commit；不设置固定 batching interval。
- **S-06**：signal 后 FULL scan 不算事件驱动；稳态必须 table-family targeted pull。
- **S-07**：containment Relation 按有界集合分组，避免一 edge 一 object 的无意义放大。
- **S-08**：公开无界 LIST、Relations、checkout 被分页/流式合同替换。
- **S-09**：幂等账本按 key 持久化且不保存完整 ChangeSet。
- **S-10**：Projection 异步追赶，Writer receipt 不等待 OpenSearch。
- **S-11**：历史不删除，但 active Dolt database 主动分代；Archive 本身不等于物理压缩。
- **S-12**：单 generation 生产安全线由压测最大通过档位的安全折扣决定，不凭经验写死。
- **S-13**：若 native Dolt 在 target/historical gate 失败，不回退 `kc_files + 伴随表`；优先降低 generation 上限，仍失败再评估新的 MVCC Snapshot adapter。
- **S-14**：过亿索引请求必须硬分页；physicalDigest 包含 shard/replica/refresh；暖 rebuild 原子换代且旧 generation 延迟到 PIT 窗口后清理；稳态 Apply 禁止全索引 count/refresh。
