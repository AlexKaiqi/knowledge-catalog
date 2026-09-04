# Store Adapter 与派生介质

日期：2026-08-27

本文回答 Snapshot 权威、检索索引、缓存和分析投影分别应落到什么介质。实时 State/Stream 的运行与存储已移交上层 Materialization 产品，不属于 Knowledge Catalog Store Adapter。

---

## Goal

回答 Snapshot 权威、检索索引、缓存和分析投影应落到什么介质，使身份、版本、来源与读写结果不随引擎变化。

## Non-Goals

- 不让 Repository 接口同时承担全文、缓存、事件流和分析投影。
- 不把某一种数据库提升为知识协议本体。
- 实时 State/Stream 不属于 Knowledge Catalog Store Adapter（文首）。

## 硬性约束 / Invariants

- `A-01` 只改变 provider 绑定，不改变 Reader/Writer/Catalog 语义。
- `C-01` 公开结果不得返回 OpenSearch `_source` 充当 Canonical。
- `P-01` 投影可删可重建；`R-02` 无 READY provider 时 SEARCH 失败关闭。
- ⓪–③ 与介质梯子不得混名（`LAYERS.md`）。
- 底座不缓存 `object_id → KnowledgeValue`。对象缓存属于上层 retriever lane；Snapshot Adapter 只能保留不解释 `object_id`/Aspect 的连接、原始 tree/blob 或 transport cache。
- 凭证只通过运行环境注入，不进入 layout、Schema 或知识正文。

## 选定方案 / 被否决方案

- 选定：[ADR-018](KNOWLEDGE_CATALOG_DESIGN.md#adr-018)：Git/Gitea/Dolt 作 Snapshot authority；OpenSearch 作 Retrieval provider；同一 Conformance。
- 否决（本文边界）：在 Snapshot 口加索引方法；Writer/Catalog 核心 import `index/`。

## 接口契约 / 状态机

介质角色以本文为准：Snapshot authority、检索投影、缓存、分析投影分开，同一 Conformance。装配根选择 adapter；参考实现文件名（如 `cli/authority_drivers.go`）不是协议。合同测试在各 adapter README 与 `internal/testkit/`。


## 1. 为什么介质要正交

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

访问 / retrieval / refine / feedback 证据不属于上表任一角色：丢失的是审计覆盖率，不是知识不可恢复，也不是可丢的检索投影。它不实现 `snapshot.Store`，不进入 Catalog pin，也不走 AccessSpec。写入是 fail-closed 追加，查询是时间窗上的等值过滤；介质由 `observability/` 的 adapter 承担，见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

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

## 4. 推导

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

介质维度的可证伪 ID 与证据见 [`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md) 的 A-01、C-01、P-01、R-01、CA-01。

---

## 6. 具体协议位置

- Snapshot capability：`snapshot/`；Knowledge 声明解释与写入：`knowledge/reader`、`knowledge/writer`；消费侧 State exact hydrate：`knowledge/serving` + 墙外 provider。
- Snapshot Adapter Conformance：`internal/testkit/`。
- 本机与远程 Snapshot：`snapshot/dolt/`、`snapshot/gitea/`；唯一装配入口为 `cli/authority_drivers.go`。
- 规模化 Dolt 的②原生 unit/object 解释位于 `knowledge/dolt/`；Relation 候选只由③ provider 产生；`snapshot/dolt/` 仍只拥有 ref/commit/AS OF 与字面 raw tree capability。
- Snapshot Projection：`index/`；物理 provider：`retrieval/`。
- Dynamic Materialization：`LIVE_MATERIALIZATION.md` 所描述的上层产品边界。
- 访问证据：`observability/` 的 Recorder / AccessLog；本机 JSONL 是参考 adapter，装配在 `cli/`。
