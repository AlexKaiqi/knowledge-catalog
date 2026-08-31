# Store Adapter 与派生介质

日期：2026-08-27

本文回答 Snapshot 权威、检索索引、缓存和分析投影分别应落到什么介质。实时 State/Stream 的运行与存储已移交上层 Materialization 产品，不属于 Knowledge Catalog Store Adapter。

---

## 1. 问题

协议需要同时适应本机 Git、远程 Git 托管、规模化 Snapshot、全文和列过滤，同时保持身份、版本、来源与读写结果不随引擎变化。

错误做法是让 Repository 接口同时承担全文、缓存、事件流和分析投影，或把某一种数据库提升为知识协议本体。

目标是：

```text
稳定 Snapshot/Knowledge 语义 × 可替换介质
```

---

## 2. 协议层与介质职责正交

```text
⓪ Snapshot 操作语义
① Catalog 组合
② Knowledge 解释
③ Retrieval Projection
```

| 介质角色 | 含义 | 丢失后果 |
|---|---|---|
| Snapshot 权威 | 已接受的 commit/ref 历史 | 知识不可恢复 |
| 索引 | 从固定 Snapshot 或外部 Binding 派生的检索结构 | 可重建，查询降级 |
| 缓存 | 已有结果或 hydrate 的加速副本 | miss 后回 provider |
| 分析投影 | 面向消费计算的派生形态 | 可重算，不反写权威 |

Catalog Registry 即使落 Git 仍是 ①；OpenSearch projection 即使与 Dolt 同机仍是 ③。外部 Stream 即使被 Retrieval 索引，也不会成为 ⓪。

---

## 3. 底座目标介质

Store 选择与 Client/Server 边界正交。“本地”只表示 KC Server 与介质在同一台机器或同一个开发拓扑中；Connector、`kc` 和 `kcfs` 仍分别经 Writer、Knowledge/Catalog 和 Workspace File typed API 进入 Server。更换为本机 Git、Dolt 或轻量检索实现时，它们也只能作为 Server 后面的 adapter，不能产生另一套直连语义。

| 能力 | 目标介质 | 明确不用 |
|---|---|---|
| 本机 Snapshot | Dolt | 内存模拟作正式权威 |
| 远程 Snapshot | Gitea Git 对象 API | 远程共享工作区 |
| 规模化 Snapshot | Dolt | 普通关系表冒充版本图 |
| 单实例精确读取/VFS | Server 后的 Snapshot adapter；可不配检索 provider | 让 Client 直开 Home，或伪造与正式 AccessSpec 不一致的搜索语义 |
| 服务检索 | OpenSearch | 把 `_source` 当 Canonical |
| 分析消费 | 上层产品选择的可重建 projection | 反向成为 Writer target |

State/Stream 的 log、cursor、retention、热尾缓存和回放引擎由 Materialization 产品选择，不再冻结在本底座的 Snapshot/Retrieval adapter 组合里。

---

## 4. 第一性原理

### 4.1 Snapshot 需要版本图

`snapshot.Store` 必须表达不可变版本、Ref、expected-old CAS 和归档；可选 `TreeStore` 表达 path/blob 读写，`HistoryStore` / `ChangeStore` 提供纯坐标加速。Git、Dolt 与 Gitea Adapter 使用不同机制，但只通过 Snapshot Conformance；同一套 Knowledge Conformance 在其上层 Reader/Writer 组合上运行。

普通关系表若没有版本图和 CAS 语义，不能只因“能存 JSON”就声明实现 `snapshot.Store`。

### 4.2 Dynamic runtime 不是 Store Adapter

State/Stream 的核心是 observation basis、cursor、window、retention、late data 和 source capability。这些语义与 git Snapshot 不同，也不需要 Catalog 组合。

因此底座只保存 Binding 声明。上层产品可以选择 Kafka、数据库 CDC、日志系统、对象存储段或源侧查询，但这些都不进入 `snapshot.Store` 或 Snapshot Adapter。

这里不禁止动态运行时制作自己的持久 checkpoint、WAL 或 savepoint。它们绑定输入 offset、
operator state、generation 和恢复生命周期，是 Materialization Runtime 的恢复产物；Knowledge
Snapshot 则绑定 Repository commit、知识内容和治理历史。两者都可能是 durable snapshot，
但不能因此共用 Store 接口、Catalog pin 或 Conformance。

### 4.3 检索按查询形态选引擎

- MATCH 需要倒排、分词和相关性模型。
- filter/sort/range/aggregate 需要可比较列值。
- 外部 Binding 可以 query-time pushdown，也可以维护 managed projection。

Schema 只声明 `text/filter/sort` 访问语义，不绑定 OpenSearch 或上层 Stream 产品。`stored`、`summary`、doc value、`_source` 等若存在，只是 provider 的私有物理优化：它们不进入 Schema、Candidate 或公开 SEARCH 结果。Candidate 只保留 typed identity 与证据，最终结果从 Snapshot 或固定 Binding hydrate 完整知识及版本。

### 4.4 Snapshot 索引按 Repository basis 共享

一把 Snapshot 工作索引对应 Repository、basis commit 与物理 provider revision，不对应 Workspace。Workspace 只给出本次 pin；`AccessPlan` 给出每仓逻辑 AccessSpec，单次请求再按 provider Probe 选择检索路径。它不是物理索引定义，也不是长期保存的 RetrievalPlan。

因此物理文档不保存 `workspace_id/workspace_ids`。同一 Repository basis 可被多个 Workspace
复用；Workspace SEARCH 在请求时按固定 pin 选择投影。OpenSearch 多 index、`_msearch` 或
PinID 级短期 alias 只属于可丢的执行优化，不能成为组合、版本或授权权威。

动态 projection 则对应 Binding generation 与 observation basis，由上层产品共享和治理；不能塞进 Repository commit 索引表后假装两者同一 basis。

### 4.5 权威成功先于派生成功

Snapshot 写入成功后，投影可以异步追赶。投影失败不回滚已接受 commit，也不反写权威。

对于动态 Binding，源侧观察成功与投影成功同样分开；读侧必须报告 basis、lag、coverage 和 degradation。

---

## 5. Local 与 Scale

底座 Local：

```text
Dolt Snapshot
no retrieval projection（精确 READ / VFS）
```

底座 Scale：

```text
Dolt Snapshot
OpenSearch projection
optional lake projections
```

上层 Materialization 产品可以独立提供本机/规模化运行形态，但不应借用底座 Store 配置把 Stream 重新注册为 Repository。

---

## 6. Adapter 不变量

以下是介质维度的设计解释；机器校验的稳定 ID 与证据以
[`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md) 的 A-01、C-01、P-01、R-01、CA-01 为准。

1. Adapter 替换不改变 RepositoryIdentity；同一 Knowledge Reader/Writer 在其上解释出相同 KnowledgeRef 和读写结果。
2. Repository Store 只承担 Snapshot；没有 Stream/APPEND capability。
3. Catalog 与 Writer/Reader 核心不 import 具体引擎或动态运行时。
4. Projection 可丢、可重建，并报告 basis/lag/coverage。
5. 索引命中后在同一声明 basis 上回 Snapshot 或固定 Binding provider，返回完整知识与版本；物理索引载荷不得冒充结果。
6. Gitea、Dolt 共享同一 Snapshot/Knowledge Conformance；私有 memory fake 只服务 provider-independent 单测。
7. 凭证只通过运行环境注入，不进入 layout、Schema 或知识正文。
8. capability 不满足时明确失败，不做含糊 fallback。
9. 底座不缓存 `object_id → KnowledgeValue`。对象缓存属于上层 retriever lane；Snapshot Adapter 只能保留不解释 `object_id`/Aspect 的数据库连接、原始 tree/blob 或 transport cache。

---

## 7. 具体协议位置

- Snapshot capability：`snapshot/`；Knowledge 声明解释与写入：`knowledge/reader`、`knowledge/writer`；消费侧 State exact hydrate：`knowledge/serving` + 墙外 provider。
- Snapshot Adapter Conformance：`internal/testkit/`。
- 本机与远程 Snapshot：`snapshot/dolt/`、`snapshot/gitea/`；唯一装配入口为 `cli/authority_drivers.go`。
- 规模化 Dolt 的②原生 unit/object 解释位于 `knowledge/dolt/`；Relation 候选只由③ provider 产生；`snapshot/dolt/` 仍只拥有 ref/commit/AS OF 与字面 raw tree capability。
- Snapshot Projection：`index/`；物理 provider：`retrieval/`。
- Dynamic Materialization：`LIVE_MATERIALIZATION.md` 所描述的上层产品边界。
