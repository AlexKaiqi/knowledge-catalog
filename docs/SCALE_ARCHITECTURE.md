# Knowledge Catalog 规模化存储与访问设计

> 状态：native Dolt 规模路线已随 Dolt adapter 退役关闭；规模目标介质为 lakeFS（Graveler），资格线与实测入口待按 [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) 重新登记。本文保留为历史设计记录。

日期：2026-08-27
定位：规模 profile 的演进决策与迁移原则，不是当前通用协议或实现状态台账。当前证据见
`TEST_CATALOG.md`，负载与资格门槛见 `SCALE_BENCHMARK.md`。

本文定义百万级数仓表、每天约 10,000 次逻辑表变化下的实现改造。它只讨论架构、数据模型、接口和迁移；负载、执行方法和验收门槛见 [`SCALE_BENCHMARK.md`](SCALE_BENCHMARK.md)。

---

## Goal

定义百万级数仓表、每天约 10,000 次逻辑表变化下的实现改造：架构、数据模型、接口和迁移，而不是用几条 SQL 优化冒充规模 profile。

## Non-Goals

- 不是当前通用协议或实现状态台账（文首；证据见 `TEST_CATALOG.md`）。
- 不反向定义当前通用协议（`docs/README.md` 权威冲突表）。
- 负载、执行方法和验收门槛不在本文（`SCALE_BENCHMARK.md`）。

## 硬性约束 / Invariants

- `IX-01`..`IX-05`：硬分页、physicalDigest 含 shard/replica/refresh、暖 rebuild、增量成本、历史 Engine 生命周期。
- Dolt scale 不以 `kc_files` 保存知识；`kc_units` 是 Canonical 内容表（本文 §1）。
- 公开「返回整个 Repository」的 API 不进入 scale profile。

## 选定方案 / 被否决方案

- 选定：layer ② `knowledge/dolt` 增量 ChangeSet；每源事件可 commit；Repository generation 归档旧仓。
- 否决：周期扫描换吞吐；Writer 重写全部 `writer.json`；返回 receipt 前同步刷新 OpenSearch；押注单个 Dolt commit graph 无限增长。

## 接口契约 / 状态机

规模 profile 的接口边界：精确读取与 Relation 查询保持有界；全 Snapshot 遍历只供维护扫描，导出与重建使用分页或流式处理；Writer 增量写入、投影按键追赶，并以 Repository generation 管理长期容量。公开消费面不提供对象实例 LIST，分页不能改变这条边界。资格门槛见 `SCALE_BENCHMARK.md`。Dolt `kc_units` 是该 profile 的选定权威表，不是把通用协议改成「只有 Dolt」。


## 1. 结论

规模 profile 不能通过“优化几条 SQL”达成目标，必须同时具备五条基础通路：

1. **Dolt scale Repository 不再用 `kc_files` 保存知识。** `kc_units` 是唯一 Canonical 内容表；`kc_objects` 只是同 commit 的对象清单。Relation endpoint/type/role 不在 authority 建定位表。
2. **Writer/Reader 不再经过全树解释。** Dolt 由 layer ② `knowledge/dolt` 直接实现增量 ChangeSet、点读、分页和历史能力；Gitea 使用文件 codec。
3. **每个源事务或逻辑表变更立即 commit。** 不增加固定时间窗口，不用周期扫描换吞吐；只有队列已经积压时才允许零等待合并，且必须保留每个源事件证据。
4. **Relation 查询、维护扫描和宿主物化必须有界。** Relation 查询使用候选分页；重建、导出与内部 checkout 使用维护侧分页或流式处理。公开消费面不提供对象实例 LIST，也不以“已经分页”为理由开放它。
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

## 3. 规模设计要消除的成本

### 3.1 成本反例与必要条件

以下反例解释规模方案的必要性，不宣称它们仍是当前实现。具体实现与验收证据分别由包 README、代码、`TEST_CATALOG.md` 与 `SCALE_BENCHMARK.md` 维护。

| 通路 | 不可接受的成本反例 | 导出的条件 |
|---|---|---|
| Writer 与 Schema 校验 | 一个小 ChangeSet 为定位单元或既有 Schema 重建全树 | 工作量受变更对象及实际引用关系约束 |
| 对象与路径点读 | 为一个目标读取或解释整个 Repository | 原生主键或固定版本定位能力，不以全扫补点读 |
| Relation 查询 | 从 authority 枚举全部关系后过滤 | exact-basis Retriever 候选分页，再按页回读 |
| Schema 描述 | 为少量 Schema 遍历全部普通对象 | 使用独立、可定位的声明 namespace |
| 变化识别 | 比较两个完整对象清单才能识别少量变化 | 在固定版本间取得受影响对象，规模路径不能退化为双全量 |
| Rebuild | 一次聚合全库文档，或回退到非流式 provider | 分批构建新 generation，发布前保留旧可用 generation |
| 增量投影 | 每次全索引计数、强制刷新，或一次载入全部积压 | 从变化识别、编译到提交都保持有界批次 |
| Writer 与 Hook | 返回 Receipt 前同步访问或重建检索引擎 | 权威提交与派生追赶分开，索引故障不拖住已接受写入 |
| 幂等账本 | 每次重写全部历史并常驻完整 ChangeSet | 按键读写重放证据，启动不载入全部历史 |
| 导出与内部 checkout | 一次返回或物化整个 Workspace | 明确的维护扫描与流式物化，不开放对象实例 LIST |
| Dolt transport | 每次数据操作新建进程或容器 | 服务运行态复用连接，启动成本不进入每次点读或提交 |
| 数仓 Collector | 收到事件后仍扫描全部表、列和任务 | 按受影响 source family 拉取，明确区分稳态与全量修复 |

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
  -> ResolveKnowledgeSet 一次
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

精确知识读取不包含对象枚举。原生增量写入、批量读取、声明定位和维护扫描是各自独立的能力；缺少其中一种，不能通过另一条全量路径掩盖。底层全 Snapshot 遍历进入 `knowledge/maintenance` 的显式 SPI；Relation 候选发现属于③，Schema 使用声明定位能力，避免让消费 Reader 对所有 Repository 强制扫描。公开能力类型由 [`knowledge/repository.go`](../knowledge/repository.go) 与 [`knowledge/maintenance`](../knowledge/maintenance) 拥有。

`SnapshotScanner` 只供 projection rebuild、迁移、显式 export 和 conformance；不能被 READ、SEARCH、Schema 或 Relations 当 fallback。Relations 合同位于 `retrieval/`，continuation 绑定 provider、repository、basis、query 与 generation，候选在同一 basis 回读 Canonical。

Reader 优先使用明确提供的完整原生知识能力，否则在具备固定版本文件访问能力时使用文件解释器；两者都缺失时明确报告能力不足。Writer 同样区分原生增量写入与文件编码，但 COMMIT/PROPOSAL 的公开语义、CAS 与 Receipt 不变。具体能力选择由 Reader/Writer 的公开类型和实现拥有，不在本文复制装配代码。

Gitea 可以提供维护扫描，但该能力不进入消费 Repository、Serving、CLI 或 Knowledge HTTP API。缺少 Relation 投影或 Schema 定位能力时必须明确失败，不能把维护扫描升格为消费后备路径。

### 5.3 抽出 provider-neutral unit 代数

PUT/REMOVE 的单元布局、前置条件、对象删除与 Schema/ValueSource 继承规则必须独立于文件序列化；否则新增原生 provider 就会复制第二套知识代数。职责应拆成：

- provider-neutral `Unit`/`ObjectState` 和 Operation apply；
- `internal/repofile` 只负责文件编码、path hint 和 tree changes；
- `knowledge/dolt` 负责行编码和 SQL changes。

两种 provider 对同一 ChangeSet 序列必须产生相同 KnowledgeValue、Resolution、Diff 和 Provenance，这是迁移的核心差分测试。

### 5.4 Dolt backend 机制

连接、branch/ref、commit、merge、archive 和 SQL transaction 由不认识知识语义的底层机制承担。`snapshot/dolt` 与 `knowledge/dolt` 复用这些机制，但只有后者定义知识编码；具体包拆分不能改变这一所有权。

scale profile 使用 `dolt sql-server` 的 MySQL-compatible 长连接和连接池；CLI/Docker-per-call 只用于本地 conformance。Dolt 官方支持 SQL Server、历史 `AS OF`、row diff 和 SQL `DOLT_COMMIT()`，分别见 [SQL Server](https://www.dolthub.com/docs/sql-reference/server/)、[Querying history](https://www.dolthub.com/docs/sql-reference/version-control/querying-history/)、[SQL functions](https://www.dolthub.com/docs/sql-reference/version-control/dolt-sql-functions/) 与 [SQL procedures](https://www.dolthub.com/docs/sql-reference/version-control/dolt-sql-procedures/)。并发设计不假设 `SELECT FOR UPDATE`，其当前支持状态见 [Supported statements](https://www.dolthub.com/docs/sql-reference/sql-support/supported-statements/)。

生产固定 Dolt 版本；不得使用 `dolthub/dolt:latest`。

---

## 6. Dolt 原生物理模型

### 6.1 唯一 Canonical：`kc_units`

一行对应一个独立维护的 Address，保存完整身份、业务值、Schema 引用、ValueSource 与来源信封。必须能按 Address 点读，也能按对象定位其组成单元；正文不再同时编码为第二份权威文件。

摘要基于 KC canonical JSON 语义，不依赖数据库 JSON 重排。路径提示只用于可读表示和导出，不参与身份。具体列名、SQL 类型、键编码和索引定义由 `knowledge/dolt/repository.go` 与 `knowledge/dolt/codec.go` 拥有；本节约束它们需要保证的语义，不冻结另一份 SQL schema。

hash 只是物理键。每次命中必须比对完整 Address/ObjectID；同 hash 不同完整身份时失败关闭，不静默覆盖。

### 6.2 版本化对象清单：`kc_objects`

对象清单与组成单元处于同一 commit，提供对象存在状态、组成数量与完整性摘要，支持维护分页、Schema 定位和对象级差分。删除状态必须能与从未存在区分，历史读取仍解释所请求 commit。

正文只在 `kc_units`；清单不成为第二份正文。清单与 units 不一致时 Reader 必须失败关闭。清单的具体列与写入编码仍由原生 provider 代码拥有。

### 6.3 Relation endpoint 投影

每个 Canonical Relation 在 layer ③ 生成检索文档；端点必须保留角色与完整对象引用之间的关联，不能把多个端点的字段误配。具体物理映射由 provider 拥有。authority 不保存 endpoint locator，也不提供关系枚举或过滤方法。消费路径只检查 exact-basis 投影是否可用，不同步构建投影。

### 6.4 Layout 元数据

原生布局必须能判断编码版本与解释约束是否兼容，必要时识别创建工具版本；不能从“表存在”推断全部知识能力有效。具体版本标记形状由 provider 定义，本文不预设一张尚未由公开实现选定的元数据表。所有知识表在同一 Dolt commit 中更新，不另设补偿双份正文的完成标记。

scale Repository 中不创建 `kc_files`。这避免“文件正文 + 结构化正文”双份权威。

---

## 7. 数仓对象模型调整

若为每条 `table -> column` 边创建一个 Relation object，30 列时每表约 62 objects、63 units，纯 containment Relation 会占近一半对象和 OpenSearch 文档。

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

纯 containment Relation 通常不声明全文字段，但其类型、方向与 endpoints 仍进入 layer ③ 的 Relation 投影，以支持有界的一跳查询。Schema 访问声明只决定额外的业务字段怎样参与检索，不决定 Relation 是否获得定位能力。authority 保留完整 Canonical Relation，不另建 endpoint locator；查询在 exact-basis Retriever 返回候选后回读该 Relation。

---

## 8. 写路径

### 8.1 一次 native commit

1. Writer 完成 ChangeSet、Provenance、ValueSource 和 Relation 形状校验。
2. 耐久命令账本原子取得本次命令的执行权。
3. 取得该 Repository 的 active-writer lease；检查目标 ref 仍是调用方预期的版本。
4. 按受影响对象分组，在固定基础版本上批量读取其 manifest 与 units。
5. 用 provider-neutral unit 代数应用 PUT/REMOVE/precondition。
6. 对已有 Schema 做 object point read；同 ChangeSet 的 Schema 从 batch 解析。
7. 计算受影响 units 与 object manifest 的增删改；Relation 的 endpoints 作为 Canonical unit 内容随同写入。
8. 在一个 SQL transaction 更新这些权威行和同 commit 清单，提交前再次检查 ref；不在 authority 写入 Relation 定位表。
9. 产生一个 Dolt commit，并使该提交与命令身份可关联，供结果不确定时核对。
10. command ledger 写入 Receipt，随后把 projection desired target 持久化。
11. 返回 Receipt；不等待 OpenSearch refresh。

所有表要么随同一个 commit 可见，要么都不可见。不得在事务外先写 locator。

### 8.2 并发与 CAS

规模写路径不依赖行锁提供跨请求的 ref 协调，而是对每个 active Repository 使用单 active-writer lease。多实例部署由共享 lease/leader election 保证；同一进程内再用短队列串行 target ref。COMMIT、PROPOSAL、merge、archive 等所有 ref mutation 都必须经过同一 lease，不能只保护 Writer COMMIT。

这不是吞吐瓶颈的默认假设：目标平均仅 `0.116 commit/s`，主要需承受短时 `11.6/s`。若压测证明单 commit 延迟不足：

- 先优化 SQL batch、prepared statements 和 commit 固定开销；
- 队列已有多条消息时，可立即合并当前已到达事件，不启动等待 timer；
- 合并后必须保存全部 event IDs/source revisions，不能把多源证据压成一个不可追踪时间戳；
- 不通过每 N 秒 group commit 牺牲低流量时效。

### 8.3 幂等账本

规模路径使用按键的耐久事务账本，不能靠重写完整 JSON 历史保存幂等性。单条命令需要保留足够的请求身份、摘要、目标与预期版本依据，以区分尚未确定结果和已接受结果，并在重放时返回原 Receipt。具体字段和状态由 [`snapshot/commandlog`](../snapshot/commandlog) 的公开类型拥有。

不保存完整 Operations，也不在启动时把全部历史载入内存。取得命令执行权与保存完成结果都是单键事务。介质可采用 bbolt 或共享控制数据库；跨进程协调能力须按部署要求验证，不能从单机存储选择推断出来。

若进程在 Dolt commit 后、ledger Complete 前崩溃，恢复器用 pending entry 的 expected parent 和 HEAD commit trailer 判定是否已应用；匹配则重建 Receipt，不匹配则失败关闭等待人工核对。

---

## 9. 读、变化与历史

### 9.1 精确读

对象点读先在固定 commit 定位清单，核对完整身份，再读取同 commit 的组成单元；数量与摘要一致后才拼装结果。Address 点读直接定位该单元，不经过全对象或全仓枚举。

批量读按有上限的对象批次执行，工作量只与这些目标对象的组成单元相关。摘要键仅用于定位，不能取代完整身份和一致性检查。原生读取实现见 `knowledge/dolt/read.go`。

### 9.2 分页

维护扫描使用稳定键序分页，续页必须绑定原 Repository、固定 commit、原扫描范围与位置。若维护任务跨多个成员物化，还必须保持原 pin 与成员位置，不能续到另一组数据。具体扫描类型和 token 编码由维护 SPI 与 provider 拥有。

Relation 查询必须有服务端最大页大小，并按候选页回读 Canonical。全 Snapshot 枚举只属于维护 SPI，不进入 `knowledge.Repository`、Serving、CLI 或 Knowledge HTTP 消费面；即使带分页，也不新增公开对象实例 LIST。维护任务和 local conformance 如需全量值，由明确的维护调用方或测试 helper 循环消费扫描页。

### 9.3 Relation

authority 不保存 endpoint 倒排表，也不提供 type/role/direction 枚举。Relation object 的保留字段进入
layer ③ 投影；消费请求先要求指定 commit 的 projection READY，再由 Retriever 分页返回 CandidateRef，
随后仅对当前页做同 basis `ReadMany` 和 Canonical 复核。无 provider 或 exact-basis projection 时明确失败，
不得扫描 Dolt/Gitea 或在请求内追赶投影。

### 9.4 Schema

Schema 描述从原生对象清单定位声明 namespace，不遍历普通知识对象。编译后的 AccessSpec 可以复用，但必须固定 Repository、commit 与声明摘要，不能把旧编译结果带到另一版本。具体定位能力见 `knowledge/dolt/schema.go`。

### 9.5 变化识别

在两个固定 commit 的对象清单之间求差，只返回内容、声明或存在状态发生变化的对象身份；不为一次追赶逐 commit 回放或解释全仓正文。Dolt 原生差分的实现入口见 `knowledge/dolt/history.go`。

### 9.6 Object LOG

Object LOG 必须在版本历史中按目标身份定位并有界返回；Diff 仍是两个固定 commit 的点读。不得先拉完整 commit log，也不得逐 commit 重建 Repository。具体历史查询及续页合同由 Knowledge 公开类型与原生 provider 拥有。

历史接口始终有 limit/continuation。普通消费者默认无 archived generation 的访问授权。

---

## 10. Projection Controller

投影采用耐久 desired-target 模式，避免同步 Hook 把检索成本带入 Writer：

1. Writer 成功后只把 `{repository, desiredCommit}` 原子写入 controller store；同 repo 新事件覆盖为更后的 target。这是快路径，可以丢失。
2. Controller 从当前 OpenSearch basis 到 desired target 求 object diff。
3. 分页 ReadMany、compile、bulk apply。
4. 一次发布新的 projection basis。
5. 长寿命 serve 启动时、以及之后的周期 tick，比较 projection basis、controller desired 与 Repository published HEAD；不同则 `Desire(HEAD)` 再追赶。这是坐标比较，不是源系统扫描，也不把 `controller.db` 的 READY 当成真相。

provider 必须支持分批建立新 generation，在发布前完成追赶，并允许构建失败时放弃候选 generation。切换前旧 generation 继续服务，发布后新 generation 的 basis 必须可验证。具体 generation writer 合同由 `index/` 与 `retrieval/opensearch/` 拥有。

增量 Apply 不得为每个 commit 执行全索引计数或强制刷新。完整计数与校验由重建发布或明确的运维任务承担；写入可以等待正常刷新策略使结果可见，但不能以一次全索引操作换取该承诺。具体请求参数由 provider 拥有，度量仍需反映 commit 到可检索状态的延迟。

消除增量计数或强制刷新，只解决固定的全索引成本；规模资格还要求变化识别、编译、提交端到端有界，以及跨进程 Controller 的协调与恢复。局部优化通过不能替代完整负载验收。物理配置、代际接口与实现证据分别见 [`retrieval/opensearch`](../retrieval/opensearch)、[`index/README.md`](../index/README.md) 和 `SCALE_BENCHMARK.md`。

Schema AccessDigest 或 physical mapping 变化时构建新 generation；旧 active index 在 Publish 前持续服务。

---

## 11. 事件驱动 Collector

### 11.1 稳态

稳态 Collector 按受影响 source family 工作：

```text
源事件与源版本
  -> 点查受影响表及其列
  -> 必要时拉该 family 的 lineage/job
  -> 读取该 family 的已发布 checkpoint
  -> 翻译并预检该 family 的 ChangeSet
  -> Writer COMMIT
  -> 持久化该 family 的源版本、已发布单元与 Receipt 依据
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

### 12.3 切换顺序（待评审候选）

以下六步是容量隔离方向下的候选顺序，尚不是可直接执行的迁移协议。保留历史、限制热写规模
的目标已经明确，但身份延续、写入交接和查询就绪的决定仍需先补齐。

1. 在新 Repository generation 的 candidate ref bootstrap 旧 active HEAD 当前态。
2. 记录切换 watermark，追赶此后事件。
3. 校验同一 object_id 的当前值与声明 digest。
4. 原子更新 KnowledgeSet 选择新 Repository。
5. 将旧 Repository 只读归档，停止写入。
6. Projection 为新 Repository 构建/切换；旧 projection 可删除并按需重建。

这个顺序本身还不能证明安全切换：

- **身份、引用与授权尚未闭环。** KnowledgeRef 包含 Repository；新仓中相同的 object_id、值和
  digest 不等于原知识身份。必须先决定既有引用、Relation 端点、Schema 引用、来源坐标与按仓
  授权怎样延续或显式迁移。本文不据此预选新的逻辑身份层，也不默认跨仓复制会保留授权。
- **最终写入边界尚未固定。** 记录水位、追赶事件之后，旧仓在停止写入前仍可能接受合法提交；
  仅追赶源事件也不能证明已经包含所有 Writer 发布。需要选定写入冻结、在途请求处理与最终
  已接受版本的核对协议，才能证明新仓不会漏掉切换窗口中的修改。
- **消费切换先于投影就绪。** 上述第 4 步已经让新任务选择新仓，第 6 步才构建投影，不能保证
  新仓查询能力已经可用。还需决定就绪验证、切换失败恢复和旧任务继续读取的条件；不得在
  失败时混用旧仓候选与新仓正文，或把缺能力当成零命中。

这些问题完成评审与验证后，才能把候选顺序收敛为迁移操作合同。下面的历史保留要求在任何
被选中的切换方案中都必须成立。

旧 generation 保留全部 Dolt commits。归档只改变写入与生命周期边界，不意味着版本图或磁盘自动缩小；若需要冷存，运维层把整个旧 Dolt database 备份/迁移出热 SQL Server，并在显式历史访问时重新挂载。垃圾回收不能被描述成仍可达历史的压缩承诺。

普通应用只解析 active Workspace；管理应用可限制或拒绝 archived repo。这样总历史不删除，同时热写和普通读只面对有界 generation。

---

## 13. 迁移策略

不做 `kc_files` 到 native rows 的长期双写，也不做第一次 miss 时隐式全仓迁移。

尚无需要保留的正式生产权威仓时：

1. 新建 native Dolt Repository generation；
2. 从当前 provider/source 重新 bootstrap；
3. 用同一组 conformance 与数仓 cases 做差分；
4. 更新 KnowledgeSet；
5. 旧测试仓归档或删除（是否删除由人工决定）。

迁移已有权威仓时：

1. 固定旧 commit；
2. 流式读取旧文件，不一次载入；
3. 解码为 Operations，分批写新 candidate；
4. 比对 object/unit/relation count、全量 digest accumulator 和抽样正文；
5. 追赶切换期间的新事件；
6. Gate 后切 Workspace；
7. 旧 Repository 保留原 commit 坐标。

native layout 变化也通过新 generation 迁移，不在数千万行 active 表上执行高风险原地 schema rewrite。

---

## 14. 迁移依赖与资格条件

迁移顺序由风险依赖决定，不是当前实现进度。具体工作项记入 `TASK.md`，完成事实与负载证据分别由 `TEST_CATALOG.md` 和 `SCALE_BENCHMARK.md` 维护。

| 前置决定 | 必须先成立的条件 | 原因 |
|---|---|---|
| 有界通路先于介质替换 | 按键幂等账、Relation 候选分页、维护扫描与流式导出、分批投影、异步追赶 | 只替换 Dolt 编码无法消除上层的全量内存和同步索引成本 |
| 共同知识代数先于原生编码 | 文件与原生 provider 对同一 Operation 序列产生相同值、解析结果、差分与来源；保持既有 Conformance | 避免存储迁移变成第二套知识语义；性质测试和差分测试应覆盖组合变化 |
| 原生机制先于规模资格 | 连接复用、单仓提交与 ref 前置条件、点读与批量读、声明定位、变化与历史定位均满足其边界 | 普通 TreeStore conformance 不能证明原生知识路径的复杂度，Relation 定位也不能回退到 authority |
| 提供方改模先于稳态压测 | 有界 grouped Relation、family 点查与 checkpoint、candidate bootstrap、watermark 追赶均可验证 | 对象粒度和源访问方式会决定整个系统的写放大；应覆盖增删改名、重复、乱序与事件缺口 |
| 全链资格先于容量承诺 | 按负载规范逐档校准，验证变化识别、编译、增量提交、暖重建与长期历史，并由实测确定分代线 | 局部优化或短历史测试不能证明持续运行能力 |
| 恢复演练先于归档运行 | 演练 generation 切换、旧仓只读与恢复、显式旧 pin 读取，并核对授权、备份目标与容量告警 | 归档后仍需履行旧坐标的可读取承诺，不能把历史留存等同于在线恢复能力 |

包拆分与命令装配属于实现方案，必须通过分层守卫；不能因迁移暂时困难而放宽 Writer、Catalog 或消费读取边界。

---

## 15. 失败语义

| 情况 | 行为 |
|---|---|
| manifest 与 units 不一致 | 拒绝读取不一致结果 |
| hash 相同但完整身份不同 | 失败关闭，不覆盖 |
| 任一 Operation/precondition 失败 | 整批无 Dolt commit |
| target ref 被推走 | 拒绝过期前置条件；按新 HEAD 重拉受影响 families 并重做 diff |
| 同一命令身份对应不同摘要 | 拒绝复用该命令身份，不能覆盖原结果 |
| commit 已成功但账本结果未确定 | 用预期父版本与提交的命令证据核对；无法证明则停止该仓写入 |
| OpenSearch 不可用 | Canonical commit 正常返回；desired target 保留并异步追赶 |
| Projection event 丢失 | 启动/恢复时比较 basis 与 HEAD，直接求 commit diff |
| 源事件重复 | checkpoint + command id 去重，不产生额外知识 revision |
| 源事件乱序 | source revision/LSN 拒绝旧观察 |
| event gap | 暂停受影响 source partition，执行明确的 targeted/full recovery |
| archived repo 普通访问 | 权限层拒绝；显式管理挂载后才允许 |

具体错误码、命令账状态和返回形状由公开 API、包 README 与 Conformance 拥有；上表只规定失败不得破坏的语义。

---

## 16. 决策记录

- **S-01**：scale Dolt 以 `kc_units` 为唯一 Canonical，不再保存 `kc_files` 正文。
- **S-02（废止）**：Dolt 不再维护 `kc_relation_endpoints` 或 `kc_objects.relation_type`；历史定位表只能在显式迁移流程中清理。接入与恢复只读验证已有 authority，不能顺带修改表或推进 ref；缺失或不兼容的知识能力必须明确报告。
- **S-03**：规模能力位于 layer ② `knowledge/dolt`；`snapshot.Store` 和 Catalog 不认识知识语义。
- **S-04**：Gitea 保留文件 codec；Dolt/Gitea 共享 unit 代数和 conformance。
- **S-05**：每个源事务/逻辑表变化立即 commit；不设置固定 batching interval。
- **S-06**：signal 后 FULL scan 不算事件驱动；稳态必须 table-family targeted pull。
- **S-07**：containment Relation 按有界集合分组，避免一 edge 一 object 的无意义放大。
- **S-08**：公开消费面不提供对象实例 LIST；Relation 查询使用候选分页，维护扫描、导出与内部 checkout 使用分页或流式处理。
- **S-09**：幂等账本按 key 持久化且不保存完整 ChangeSet。
- **S-10**：Projection 异步追赶，Writer receipt 不等待 OpenSearch。
- **S-11**：历史不删除，但 active Dolt database 主动分代；Archive 本身不等于物理压缩。
- **S-12**：单 generation 生产安全线由压测最大通过档位的安全折扣决定，不凭经验写死。
- **S-13**：若 native Dolt 在 target/historical gate 失败，不回退 `kc_files + 伴随表`；优先降低 generation 上限，仍失败再评估新的 MVCC Snapshot adapter。
- **S-14**：过亿索引请求必须硬分页；physicalDigest 包含 shard/replica/refresh；暖 rebuild 原子换代且旧 generation 延迟到 PIT 窗口后清理；稳态 Apply 禁止全索引 count/refresh。
