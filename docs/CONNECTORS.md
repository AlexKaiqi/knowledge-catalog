# 外部资源访问与知识采集

日期：2026-08-25

本文回答外部系统仍持有运行态或领域权威时，Agent 怎样访问它，以及外部变化怎样显式进入 Knowledge Repository。Descriptor 与 Writer 输入的已选定形状见 `connector` / Writer 公开合同。

---

## Goal

分开两件事：Agent 按已保存的访问声明读取源侧当前值；外部观察要成为知识时必须显式走 Writer 形成 Snapshot。

## Non-Goals

- 一次查询不得自动沉淀知识；一次采集不得自动授权所有 Agent 访问源（下文「为什么」）。
- `connector/` 不连源、不持 Writer、不放凭证或运行宿主。
- 不新增采集 Write Surface；采集输出仍是 ChangeSet，只经 Writer COMMIT/PROPOSAL。

## 硬性约束 / Invariants

- [ADR-021](KNOWLEDGE_CATALOG_DESIGN.md#adr-021)：访问声明是知识；凭证和运行留墙外。
- `W-01` 沉淀必须走唯一 Snapshot target 的 PUT/REMOVE。
- 声明不得携带 token、任意 endpoint 或运行拓扑（下文「句柄不是内容」）。

## 选定方案 / 被否决方案

- 选定：[ADR-021](KNOWLEDGE_CATALOG_DESIGN.md#adr-021)：句柄与内容分离；Collector 用 Preview 做 STATE Address 对账，写回只调 Writer API。
- 否决（本文边界）：把 Connector 做成 hook。系统级拒绝见 [R-03](KNOWLEDGE_CATALOG_DESIGN.md#r-03)。

## 接口契约 / 状态机

访问声明是知识；采集输出是 ChangeSet，只经 Writer。底座必须提供 Preview 对账与 Writer API；Connector registry/runtime 可以在墙外，但不能因此把采集协议从产品里删掉。参考实现：`connector/` helper。


## 1. 为什么访问和采集必须分开

外部权威有两种不同需求：

```text
即时访问：Agent 已知资源 → 读取源侧当前值
知识采集：外部观察 → 显式形成可版本化的 Snapshot 知识
```

如果把两者混为一谈，会产生两个危险推论：一次查询自动沉淀知识，或一次采集自动授权所有 Agent 访问源系统。两者都不成立。

---

## 2. 推导

### 2.1 句柄不是内容

Repository 可以保存稳定、可版本化的访问声明，但外部运行值仍由外部系统权威持有。声明必须足以让 Agent 理解资源语义和逻辑操作，却不能携带 token、任意 endpoint 或运行拓扑。

### 2.2 访问和采集正交

资源访问默认不写知识；需要沉淀时，Collector 必须显式形成 Snapshot ChangeSet，并经过 Writer 的 CAS、幂等、Schema 与 provenance 约束。

### 2.3 平台能力与领域翻译分离

身份与授权由 [`PERMISSIONS.md`](PERMISSIONS.md) 拥有；凭证和运行边界由
[`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) 拥有；访问证据由
[`OBSERVABILITY.md`](OBSERVABILITY.md) 拥有。领域接入方负责源语义、源身份映射和变化翻译，
不应在 Catalog、Writer 或 CLI 内长出具体源客户端。

### 2.4 外部身份不能冒充 object_id

source key 到 Knowledge Address 的映射属于接入方 integration 工程。协议不能从 URL、路径或外部主键自行发明 `object_id`；协议旅程夹具也不成为领域映射的所有者。

---

## 3. 两个领域角色

### 3.1 访问声明

已知 Descriptor 的通用操作先从本次固定 pin 回读声明，只调用其中允许的操作，并把输入与
固定声明坐标交给独立 Resource Access runtime。调用方不能覆盖声明的运行目标或协议，
平台也不内置 MySQL 等源语义。带输入的资源操作与 Aspect State Binding 的无输入 hydrate
是两个入口；命令及传输字段见 [`cli/SURFACE.md`](../cli/SURFACE.md) 与 `client/` 公开类型。

“自包含”表示运行方拿到固定声明后，不必再猜能力或参数语义；不表示每个动态 Aspect 必须独立成 Descriptor 文件。Aspect 可以内嵌或引用 State/Stream Binding，见 `LIVE_MATERIALIZATION.md`。

无论包装怎样变化，访问记录都应保留实际调用主体、可选代理用户、调用关联上下文、固定声明版本、实际运行代际、外部观察 basis、结果摘要与错误。Agent 代理用户时不能把用户冒充成 principal；payload 是否留存由证据策略决定，不新增 Workspace session。

### 3.2 Collector

Collector 读取外部当前态或事件窗口，并把需要长期保留的观察翻译为 Snapshot ChangeSet：

```text
外部观察 → Collector → ChangeSet → COMMIT
```

Collector 不新增 Write Surface，也不直写 git。STATE 对账必须受 Scope 约束：patch 不凭空删除，reconcile 只删除已观察且在 Scope 内的 Address，Desired 越界应整批拒绝。

即使 Connector 与 Server 同机，它也不能打开 KC Home。对账前通过 Writer API 取得目标的
固定基点，产生 ChangeSet 后再提交；本地文件预处理不自行解析服务端状态，也不构成另一条
写入通路。公开操作顺序见 [`connector/README.md`](../connector/README.md) 和
[`cli/SURFACE.md`](../cli/SURFACE.md)。

---

## 4. 运行边界

业务方可以在墙外 integration repo 中维护具体协议、适配器、测试与 Collector。平台运行环境负责构建、激活、身份接入、凭证和可观测性；integration repo 不是 Knowledge Repository，也不是 Workspace 成员。

Catalog 只组合 Repository 坐标，不解释 Descriptor，不调用外部资源。Writer 只接收显式知识变更，不托管采集循环。Hook 只能通知外部系统，不能冒充 Collector。

---

## 5. 与动态检索的关系

已知资源访问只解决 hydrate，不能解决 discovery。State/Stream Binding 若要被统一检索，需要稳定 Schema 和 AccessHints，由上层 Retrieval 下推查询或建立可丢投影。

接入方的默认职责是声明访问能力并通知 invalidation；平台按固定 Binding 拉取、追赶和 reconcile。接入方不直接写某一种物理索引。完整决策见 `LIVE_MATERIALIZATION.md`。

---

## 6. 具体协议位置

- `connector/`、`connector/README.md`：STATE Address 对账。
- `knowledge/writer/`、`knowledge/writer/README.md`：Snapshot COMMIT 输入和写约束。
- `knowledge/`：Address、ChangeSet 与 provenance。
- `docs/LIVE_MATERIALIZATION.md`：动态物化、invalidate-and-pull 与统一检索。
- `docs/PERMISSIONS.md`：可信身份与动作授权。
- `docs/OBSERVABILITY.md`：访问账、Agent trace/反馈与 hitmap。
- 接入方 integration 工程：具体源、运行宿主和领域验收；本仓数仓示例在 `.data/data-warehouse/`，协议旅程在 `.data/scenes/`。
