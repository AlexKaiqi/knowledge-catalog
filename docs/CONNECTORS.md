# 外部资源访问与知识采集

日期：2026-09-18

本文回答外部系统仍持有运行态或领域权威时，Agent 怎样访问它，以及外部变化怎样显式进入 Knowledge Repository。Descriptor 与 Writer 输入的已选定形状见 `connector` / Writer 公开合同。公开名称 Collector / Observer / Resource Access 以 [`TERMINOLOGY.md`](reviewed/terminology.md) §6 为准。

---

## Goal

分开三件事：Agent 按已保存的访问声明读取源侧当前值；外部观察要成为知识时必须显式走 Collector → Writer 形成 Snapshot；Bound State 变化由 Observer 发 change notice，平台再按固定 Binding 拉取。

## Non-Goals

- 一次查询不得自动沉淀知识；一次采集不得自动授权所有 Agent 访问源（下文「为什么」）。
- 不把 Collector、Observer、Resource Access 合成一个公开名（否决 watcher）。
- `connector/` 不连源、不持 Writer、不放凭证或运行宿主。
- 不新增采集 Write Surface；采集输出仍是 ChangeSet，只经 Writer COMMIT/PROPOSAL。
- 不强制墙外 runtime 校验或消费 KC 转发的调用方认证；用不用是接入方的事，KC 仍必须带上。

## 硬性约束 / Invariants

- [ADR-021](KNOWLEDGE_CATALOG_DESIGN.md#adr-021)：访问声明是知识；凭证和运行留墙外。
- `W-01` 沉淀必须走唯一 Snapshot target 的 PUT/REMOVE。
- 声明不得携带 token 或运行拓扑（下文「句柄不是内容」）。resource-access 原点由接入方写在 Domain Schema，不由部署方按源改 Server 配置。

## 选定方案 / 被否决方案

- 选定：[ADR-021](KNOWLEDGE_CATALOG_DESIGN.md#adr-021)：句柄与内容分离；Collector 用 Preview 做 STATE Address 对账，写回只调 Writer API。
- 选定：源侧三个角色分合同——Collector 对账后发 Writer；Observer 只发 change notice；Resource Access 提供 origin 访问地址。同一进程可以兼任，协议不能混。公开名称见 `TERMINOLOGY.md`。
- 选定：`resource-access/v1` 原点写在 Domain Schema Canonical frontmatter 的 `origin`（ResourceDescriptor 操作写在描述的 `origin`）；`kc access` 用 origin + 实体 ID，不另存空 Aspect。接入方随知识发布，不由部署方按源改 Server。
- 选定：出站 `{origin}/v1/access` 必须携带已经通过 Server 认证边界的调用方证明。Taihu/Gitea 转发 `Authorization`，Taihu 网关另转发已验证的 `X-Tai-Identity`，并始终带已验证 principal（`X-Resource-Principal`）。local 配对没有 token，只带 principal。凭证不进 Schema、flags、访问账或 JSON body。
- 否决（本文边界）：把 Connector 做成 hook；整台 Server 一个访问 URL；为瞬时值再 PUT 一份 `null` 知识对象；只传身份名字、把 token 写入 origin/Schema、因接入方可能不用而省略认证头。系统级拒绝见 [R-03](KNOWLEDGE_CATALOG_DESIGN.md#r-03)。

## 接口契约 / 状态机

访问声明是知识；采集输出在内部仍是 ChangeSet，只经 Writer。操作员 CLI 只看 Preview summary 或 `connector-preview --command-id`，不持有 ChangeSet 文件。底座必须提供 Preview 对账与 Writer API；Connector registry/runtime 可以在墙外，但不能因此把采集协议从产品里删掉。参考实现：`connector/` helper。


## 1. 为什么访问、采集和通知必须分开

外部权威有三种不同需求：

```text
即时访问：Agent 已知资源 → Resource Access 按 origin 取值
知识采集：Collector 对账后发 Writer → Snapshot
变化通知：Observer 发 change notice → 平台按固定 Binding 向 Resource Access 拉取
```

如果把三者混为一谈，会产生危险推论：一次查询自动沉淀知识、一次采集自动授权所有 Agent 访问源，或一次通知带着正文写入索引/仓。三者都不成立。

---

## 2. 推导

### 2.1 句柄不是内容

Repository 可以保存稳定、可版本化的访问声明，但外部运行值仍由外部系统权威持有。声明必须足以让 Agent 理解资源语义。Domain Schema Canonical frontmatter 的 `origin` 是接入方自报的 `resource-access/v1` 服务原点；访问坐标是实体 `object_id`。`origin` 不是业务字段，也不携带 token 或内部拓扑。

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

## 3. 三个领域角色

### 3.1 访问声明与 Resource Access

已知 Descriptor 的通用操作先从本次固定 pin 回读声明，只调用其中允许的操作，并把输入与
固定声明坐标交给独立 Resource Access（`origin` 上的访问地址）。调用方不能覆盖声明的运行目标或协议，
平台也不内置 MySQL 等源语义。带输入的资源操作与 Aspect State Binding 的无输入 hydrate
是两个入口；命令及传输字段见 [`cli/SURFACE.md`](../cli/SURFACE.md) 与 `client/` 公开类型。

“自包含”表示运行方拿到固定声明后，不必再猜能力或参数语义；不表示每个动态 Aspect 必须独立成 Descriptor 文件。Aspect 可以内嵌或引用 State/Stream Binding，见 `LIVE_MATERIALIZATION.md`。

无论包装怎样变化，访问记录都应保留实际调用主体、可选代理用户、调用关联上下文、固定声明版本、实际运行代际、外部观察 basis、结果摘要与错误。Agent 代理用户时不能把用户冒充成 principal；payload 是否留存由证据策略决定，不新增 Workspace session。

### 3.2 Collector

Collector 对账外部当前态或事件窗口，把要进仓的差量发给 Writer：

```text
外部观察 → Collector 对账 → ChangeSet → Writer COMMIT
```

Collector 不新增 Write Surface，也不直写 git。STATE 对账必须受 Scope 约束：patch 不凭空删除，reconcile 只删除已观察且在 Scope 内的 Address，Desired 越界应整批拒绝。Collector 不是 Observer，也不是 Resource Access：表结构进入 Snapshot 走本节；行数等 Bound State 由 Resource Access 取值，变化通知走 §3.3。

即使 Connector 与 Server 同机，它也不能打开 KC Home。对账前通过 Writer API 取得目标的
固定基点，产生 ChangeSet 后再提交；本地文件预处理不自行解析服务端状态，也不构成另一条
写入通路。公开操作顺序见 [`connector/README.md`](../connector/README.md) 和
[`cli/SURFACE.md`](../cli/SURFACE.md)。

### 3.3 Observer

Observer 盯源变化，只向平台发 change notice（仓/ref/可选 Address/可选 source revision hint）。notice 不带 observation 正文，不写 OpenSearch，不推进 Writer HEAD，也不代替 Collector 对账。平台按固定 Binding 向 Resource Access 拉取。入站字段由 [`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) §3.2 与 `index.ChangeNotice` 拥有。

新实体仍须先经 Collector COMMIT 建立知识身份；否则只能作为外部 ResourceRef 返回。

---

## 4. 运行边界

业务方可以在墙外 integration repo 中维护具体协议、适配器、测试、Collector、Observer 与 Resource Access。平台运行环境负责构建、激活、身份接入、凭证和可观测性；integration repo 不是 Knowledge Repository，也不是 Workspace 成员。

Catalog 只组合 Repository 坐标，不解释 Descriptor，不调用外部资源。Writer 只接收显式知识变更，不托管采集循环。Hook 只能通知外部系统，不能冒充 Collector 或 Observer。

---

## 5. 与动态检索的关系

已知资源访问只解决 hydrate，不能解决 discovery。State/Stream Binding 若要被统一检索，需要稳定 Schema 和 AccessHints，由上层 Retrieval 下推查询或建立可丢投影。

接入方声明访问能力。Observer 只发 change notice；平台按固定 Binding 向 Resource Access 拉取。要进仓的结构变化由 Collector 对账后发 Writer。Observer 不写索引，也不 COMMIT。完整决策见 `LIVE_MATERIALIZATION.md`。

---

## 6. 具体协议位置

- `connector/`、`connector/README.md`：STATE Address 对账。
- `knowledge/writer/`、`knowledge/writer/README.md`：Snapshot COMMIT 输入和写约束。
- `knowledge/`：Address、ChangeSet 与 provenance。
- `docs/LIVE_MATERIALIZATION.md`：动态物化、invalidate-and-pull 与统一检索。
- `docs/PROJECTION_CONTROLLER.md`：Observer change notice 与投影拉取。
- `docs/reviewed/terminology.md`：Collector / Observer / Resource Access。
- `docs/PERMISSIONS.md`：可信身份与动作授权。
- `docs/OBSERVABILITY.md`：访问账、Agent trace/反馈与 hitmap。
- 接入方 integration 工程：具体源、运行宿主和领域验收；本仓示例在走查叶 `_materials/`（清河茶铺 + Resource Access），协议旅程在 `.data/scenes/`。
