# Catalog / Knowledge 服务与客户端架构

日期：2026-08-27

定位：逻辑服务、typed API、KC Client、Workspace File Gateway 与部署拓扑的设计合同。
实现状态和外部环境验收只在 `MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 维护。

本文定义多方接入与消费 Knowledge Catalog 时的服务边界。它回答四个问题：

1. 接入方如何让自己的 Repository 被发现并持续发布知识；
2. 消费方如何发现 Repository、组合 Workspace、检索结构化知识；
3. 普通 Repository 与 Knowledge Repository 如何在同一 Workspace 中共存；
4. Agent 如何把同一份固定版本内容挂载为本地只读 VFS。

本文不改变 `docs/LAYERS.md` 的 ⓪–③ 分层。服务是应用装配和部署边界，不是新的协议层。
公开名称以 `docs/TERMINOLOGY.md` 为准。

---

## Goal

定义多方接入与消费时的逻辑服务、typed API、KC Client、Workspace File Gateway 与部署拓扑：两个逻辑平面，接入写面和宿主文件接缝作为端口暴露。
部署实例可以替换；Catalog、已接受的操作和治理状态必须从独立的持久权威恢复。

## Non-Goals

- 服务不是新的协议层（文首；`LAYERS.md`）。
- 不改变 ⓪–③；公开名称以 `TERMINOLOGY.md` 为准。
- 不把 CLI 命令表注册成 HTTP；不恢复 `/v1/<verb>` 或任意 flags DTO。
- 实现状态不在本文（`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`）。

## 硬性约束 / Invariants

- `API-01` CLI 与 HTTP 调同一应用 executor，transport 注册相互独立。
- 应用 executor 接受按用例定义的 typed request，返回 typed result；argv flag map、HTTP
  method/path/header 和 provider 私有请求都不得进入应用核心。标识在 transport 边界解析为
  其 owner 包的命名类型，不能在应用编排中退回可互换的裸字符串。
- `KS-01` 远程消费仍逐请求认证授权，不增加 WorkspaceSession。
- `CA-01` / `CA-02` / `CA-03` 同版本正文缓存由上层组件装配；低层只持 hydrate 端口，缓存不得改变版本、交付授权或有界读取。
- 部署必须显式声明认证模式；只注册公开 HTTP registry 中的 typed namespaces，不以本文复制路由表。
- 运行指标不得塞进 CLI flags、HTTP DTO 或公开协议。
- Catalog Snapshot 权威的耐久边界独立于进程工作盘；成功响应必须对应已持久化的登记变更。启动只恢复既有部署，不能隐式创建 Catalog 或 Snapshot。
- 部署配置、授权与 Gate、Writer 幂等账、ControlState、待投递通知和原始访问证据都必须有明确的恢复来源；不能因缺失而静默装配成空状态。

## 选定方案 / 被否决方案

- 选定：Catalog Plane 与 Knowledge Plane；Writer / Governance / Admin / Operations 为独立端口。
- 选定：CLI 与 HTTP 分别完成语法解析和 DTO 转换，再调用同一组 typed application use case；
  应用核心拥有授权、固定 basis、Reader/Writer、交付链与证据的调用顺序，不拥有 transport
  注册和具体 provider 装配。
- 选定：声明式部署配置、独立的 Catalog Snapshot 权威与耐久控制状态、可重建工作缓存；Repository 接入用一次应用操作验证既有 authority 并提交 Catalog 登记。
- 选定：第一阶段模块化单体的运行陪伴为 `kc-server`、lakeFS、只服务 lakeFS 的 PostgreSQL、对象存储数据面（生产 COS，本地 MinIO）、OpenSearch，以及一条可观测管道（OTel Collector + 指标 + trace + 日志 + 面板，1 个容器或接到现成平台）。本地替代见 `scripts/deploy/`。
- 选定：服务端内部以可注入的同版本 hydrate 端口复用完整 Snapshot 读取；上层缓存命中复用已验证副本，未命中部分批量回源。
- 否决：SDK 在 SEARCH 之外补读正文；把正文缓存放入 Reader/Snapshot 的具体实现；用知识 ID 或 latest 作为跨版本缓存依据。
- 否决：把进程工作目录当 Catalog 权威；启动时创建业务仓；要求用户先本机挂仓再单独登记；把配置、秘密、幂等账和可丢索引混成一份 Git 目录。
- 否决：PostgreSQL 或对象桶冒充 Snapshot 权威；把可观测进程打进 `kc-server`；用 Jaeger/Loki 的进程数当合同。
- 否决：用通用 verb + flags map 作为应用服务接口；HTTP handler 调 CLI parser/dispatcher；
  CLI 与 HTTP 各自复制授权、固定 basis 或错误映射。
- 否决：本文 §12 的架构方案（Catalog 理解 Aspect、返回 `_source`、WorkspaceSession、FUSE 当 Writer 等）。跨进程幂等、MCP、多实例拆分是规模化方向，未落地只记 `MVP_ACCEPTANCE.md`，不是否决。

## 接口契约 / 状态机

两个逻辑平面 + Writer / Governance / Admin / Operations 端口；CLI 与 HTTP 共用 typed
application executor、分别注册。executor 的用例边界由应用包公开类型和 Conformance 拥有；
CLI argv、HTTP method/path/header 与 provider 信封不得成为其输入。文件投影是 Workspace File
Gateway。路由与 DTO 以 HTTP registry、包 README 和 `TEST_CATALOG.md` 路由分母为准；参考实现：
`cli/`、`cmd/kcfs`。不得把当前单体装配写成「只能单进程」。


## 1. 结论

Knowledge Catalog 产品对外呈现两个逻辑平面；接入写面和宿主文件接缝作为各自平面的独立端口暴露：

```text
Knowledge Catalog
├── Catalog Plane       Repository / Workspace / selector / pin / mount
└── Knowledge Plane     Schema / Entity / Aspect / Relation / Search / Writer
```

Catalog Plane 对内容格式宽容；Knowledge Plane 对结构化读取和发布严格。约束发生在 Knowledge Reader/Writer 解释固定 tree 时，不发生在 Repository 注册、Workspace 组合或 VFS 挂载时。

```text
                                        ┌─────────────────────┐
Provider Connector ── ChangeSet ───────→│ Writer API          │
                                        └──────────┬──────────┘
                                                   │ COMMIT
                                                   ▼
                                           Snapshot Repository
                                                   │ AfterSnapshot
                                                   ▼
                                           Retrieval Projection

Consumer / Agent
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│ KC Client                                                      │
│  CatalogClient     KnowledgeClient      MountController         │
└────────┬─────────────────┬────────────────────┬─────────────────┘
         │                 │                    │ immutable plan
         ▼                 ▼                    ▼
┌────────────────┐  ┌────────────────┐   ┌──────────────┐  ┌──────────┐
│ Catalog Server │  │Knowledge Server│   │Workspace File│→ │  kcfs    │
│ repo/workspace │  │Aspect/search   │   │Gateway       │  │local VFS │
└────────────────┘  └────────────────┘   └──────────────┘  └──────────┘
```

职责一句话：

| 组件 | 回答的问题 |
|---|---|
| Catalog Server | 有哪些 Repository/Workspace？这次任务固定在哪些 commit？ |
| Knowledge Server | 固定视图里有哪些 Entity/Aspect/Relation？如何检索并回读？ |
| KC Client | 如何以统一身份调用控制面、知识面并启动本地挂载？ |
| Workspace File Gateway | 如何按固定 ResolvedKnowledgeSet 提供不解释知识的 path/tree/blob？ |
| kcfs | 如何把应用层准备好的固定文件计划投影成本机只读目录？ |
| Writer API | 接入方如何以 PUT/REMOVE ChangeSet 更新唯一目标 Repository？ |

Catalog Server、Knowledge Server、Workspace File Gateway 和 Writer API 初期可以在同一进程部署，但 API、包依赖和状态所有权必须保持分离。各边界共享同一身份与 PolicyEvaluator 合同，并分别执行授权，不能各自发明权限规则。

### 1.1 Surface 与 transport

产品能力、CLI、HTTP 和 Agent 工具不是同一张表：

```text
Agent ── shell ──→ grouped kc CLI ───────────→ Typed HTTP Clients

External caller ── formal HTTP API ─────────→ Application Services
```

- Agent 不注册 `kc`、`knowledge_*`、`resource` 或 VFS 模型工具；它调用分组 CLI。
- 文件通过只读 `kcfs` mount 成为普通宿主路径，使用 `ls/find/rg/cat`；用户工作目录的其它路径仍可写。
- HTTP handler 不接收任意 verb/flags，不调用 CLI dispatcher；每个服务 namespace 显式注册 typed route。
- 本地是部署拓扑，不是旁路 transport：本机 CLI、Connector 和 `kcfs` 也必须调用本机 KC Server。
- 显式部署管理依据持久配置初始化或检查部署；Server 启动只恢复。业务请求始终经 typed Client/Server。
- Knowledge 消费面没有无界 LIST。内部全量遍历命名为 Snapshot scan，只供重建、迁移、
  导出、Semantic File View 投影构建和验收；首次使用所需 DISCOVER/BROWSE 是 Catalog/
  知识集与 Schema 分页，不是对象 LIST。README 走 READ/SEARCH。

### 1.2 Typed Application Core

Application Core 是服务层的用例编排边界，不是新的协议层。每个用例以 typed request/result
表达调用意图；它组合当前身份、当前授权、固定 Repository/Workspace basis、协议服务、
交付链和证据端口。应用核心只能依赖这些端口与协议 owner 的公开类型，不得 import CLI
dispatcher、HTTP registry、具体 Snapshot/Retrieval adapter 或部署配置解析器。

CLI 和 HTTP 可以有不同的公开 surface，但相同用例必须进入同一个 executor。transport 负责：

- 解析 argv、method/path/header 与 wire DTO；
- 在边界校验并构造命名标识和 typed request；
- 把 typed result 或协议错误编码为各自公开输出。

transport 不负责授权、选择 latest、解析 Workspace、回读 Canonical、执行 Writer 或改写交付
结果。应用核心也不接收 `map[string]FlagValue`、任意 verb、HTTP request 或 provider 私有信封。
因此新增 transport 不会长出一套业务规则，替换 transport 也不会改变协议观察。

---

## 2. 两级发现

“发现”有两个不同对象，不能由一个模糊的 Search API 承担。

### 2.1 Repository / Workspace 发现

由 Catalog Server 提供：

- 当前身份可见的 Repository；
- Repository 的治理边界和 Snapshot/TreeStore 能力摘要；
- 可使用的 Workspace；
- Workspace 的成员配方、revision 和 mount 布局；
- 一次 `ResolveKnowledgeSet` 得到的固定 `{repository → commit}`。

Catalog 核心只认识 Repository identity、selector、commit 和 Workspace。Snapshot endpoint、驱动配置和机器凭证属于服务装配的 Store Directory，不进入 `catalog/` 协议类型。Aspect、Schema 和检索字段由 Knowledge Plane 在实际使用时解释，不出现在 Catalog DTO 中。

每个非归档 Catalog 都登记内置 `kr://kc/system`。它发布 Meta Schema 与核心协议 Schema，
对已认证用户可读，但不自动进入业务 Workspace，也不扩大其它 Repository 权限。登记由
显式部署初始化完成；Server 恢复只验证既有权威，不隐式补登记或重置治理数据。完整生命周期见
`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`。

### 2.2 知识发现

由 Knowledge Server 提供：

- `DESCRIBE_SCHEMA`：发现可用 Entity/Aspect/Relation 结构；
- `SEARCH`：按 Schema AccessHints 检索对象；
- `READ` / `READ_ADDRESS`：读取并拼装 Canonical；
- `RELATIONS`：按固定 basis 查询关系；
- `GET_PROVENANCE` / `LOG`：读取来源和对象历史；
- `RESOLVE_BINDING`：返回稳定访问声明，不调用墙外 runtime。

OpenSearch 不是知识发现 API。它是 Knowledge Server 内部的 Retrieval provider，只返回无正文的 `CandidateRef`。

### 2.3 Catalog、Workspace 与本地任务

三个概念的生命周期不同：

| 概念 | 位置 | 生命周期 | 内容 |
|---|---|---|---|
| Catalog | 服务端共享 | 组织级长期 | 被承认的 Repository、KnowledgeSet、治理历史 |
| KnowledgeSet | Catalog Server 或客户端本地文件 | 可版本化、可复用 | Repository 子集、selector、可选 mount path/subPath |
| ResolvedKnowledgeSet | 请求或客户端 | 一次任务 | 固定 `{repository → commit}` 和 PinID；PinID 绑定配方路径布局 |
| VFS mount | 客户端本机 | 一个进程 | ResolvedKnowledgeSet/PinID 的只读宿主投影 |

因此 Workspace 不是本地文件系统，也不是另一个 Repository。它是“这类任务需要同时组合哪些 Snapshot Repository”的命名配方。本地只持有配方文件、这次 Resolve 后的 pin 和可选 mount。

Workspace 可以有两种来源，但进入消费面后使用同一语义：

1. 组织在 Catalog Server 发布的共享 Workspace；
2. 客户端从 `.kc-dataset.yaml` 或显式参数形成的本地/临时配方。

便携 `.kc-dataset.yaml` 是配方载体，不是 Repository 或本地状态权威；它可以被发布为服务端 Workspace，也可以只用于生成一次临时 ResolvedKnowledgeSet。本机 overlay、目标目录和 FUSE 生命周期不写回共享配方。远程个人 Workspace 只有在 owner、visibility、命名空间和生命周期协议完整后再增加，不作为 V1 前提。

### 2.4 Workspace 不要求知识格式

Workspace 可以混合普通 Repository 与 Knowledge Repository：

```text
ResolvedKnowledgeSet
├── plain code/docs repo       mount / checkout / rg
├── knowledge repo A           mount + READ/SEARCH
└── knowledge repo B           mount + READ/SEARCH
```

Repository 注册、Workspace Resolve 和 Workspace File Gateway 都不要求 `object_id`、Aspect 或 Schema。Knowledge Server 只对显式提供 layer ② 精确读取能力的成员运行 Knowledge Reader；`TreeStore` 本身不能被推断为知识能力。缺少 Schema 精确读取能力或准备好的 Retrieval projection 时分别返回 capability/coverage，不在消费请求中遍历 tree。Relation 没有 layer ② locator：候选发现必须来自 exact-basis Retriever。用户要搜索普通文件时使用 mount 上的 `rg`，不能让 Knowledge SEARCH 退化为整包 JSON 或文件 contains。

知识规范是发布与结构化访问合同：接入方声称某 Repository 是知识提供方时，必须遵守 Address、Schema、Aspect、Relation、provenance、PUT/REMOVE 和 Writer CAS；用户在自己的宿主 Workspace 中开发普通文件不受这些格式约束。用户决定把成果发布为知识时，再通过 Connector 翻译为 ChangeSet 并进入 Writer。

### 2.5 知识搜索范围

知识 SEARCH 只有两种显式 basis：一个 Repository，或调用方已经固定的 Workspace pin。
Catalog 只提供有界库存发现，不自动把已登记 Repository 变成搜索候选，也不提供
`search --catalog` 语法糖。多源搜索仍是 ResolveKnowledgeSet 后调用 Knowledge
SEARCH；Catalog Server 只做 Repository 选择和 Snapshot 坐标解析，真正的 capability、
Schema、Aspect 查询和交付链在 Knowledge Server。

---

## 3. Catalog Server

### 3.1 责任

Catalog Server 是组合控制面：

1. 管理 Catalog 生命周期；
2. 注册、归档和枚举 Repository identity；
3. 管理 KnowledgeSet；
4. 在请求开始时解析已保存或客户端临时提交的 KnowledgeSet，生成 ResolvedKnowledgeSet；
5. 对 Catalog/Workspace/Repository 动作执行授权；
6. 保存 Catalog 变更历史和服务审计；
7. 暴露 Catalog 配置的 discovery Workspace identity，不执行知识搜索。

### 3.2 不负责

Catalog Server 不应：

- 读取知识 frontmatter；
- 理解 `object_id`、Aspect、Schema、Binding 或 AccessSpec；
- 拥有一个跨仓知识索引；
- 返回 OpenSearch 文档；
- 把 Workspace 当成新 Repository；
- 因 Workspace 包含某仓而授予该仓读权；
- 要求 Workspace 的所有成员都符合知识格式或具备 tree 读取能力；
- 在一次已解析任务中继续跟随 `latest`。

### 3.3 Catalog 与 Store Directory

服务进程需要打开远程 Repository，但这个运行事实不能污染 Catalog 协议。应用装配使用两个登记面：

```text
Catalog Registry              Store Directory
repository id                 repository id → Snapshot adapter config
workspace recipe              endpoint / driver / capability
selector / mount layout       server-side credential reference
```

Catalog API 可以把两者组合成一个面向用户的 Repository 摘要，但 `catalog/` 包仍只保存 Repository ID。密码、token 和用户传入凭证绝不进入任一公开响应。

### 3.4 ResolvedKnowledgeSet 与请求认证

消费任务必须先 Resolve：

同一配方中的各 Repository selector 被解析为固定 commit；成员坐标与配方路径布局共同决定
PinID。这是从可继续演进的配方到不可变任务 basis 的一次转换。公开类型由
[`catalog/README.md`](../catalog/README.md) 与 `catalog.KnowledgeSet` /
`catalog.ResolvedKnowledgeSet` 拥有，传输与重放载体见 [`client/README.md`](../client/README.md)。

`ResolvedKnowledgeSet` 是可导出、可复核的不可变坐标；PinID 绑定成员 commits 和
KnowledgeSet 中的路径布局。它不复制配方字段、不带 TTL，也不冻结权限。
跨命令重放时保存 `pin.json`，并继续提供产生它的同一 KnowledgeSet：命名
Workspace 使用 setId + revision，本地配方继续提交同一 `.kc-dataset.yaml`。
Catalog 核心保持两个类型分离，不再为这组坐标增加第三种公开别名。

远程消费不再创建 Workspace 领域的 session。每个 Knowledge 或 Workspace File 请求
携带正常身份凭证，并提交完整 `ResolvedKnowledgeSet` basis 及产生它的
KnowledgeSet 引用或临时配方。服务端执行三项独立校验：

1. 认证器验证凭证并注入 `principal`；请求体或普通 header 不能自报身份；
2. Catalog Server 复算 PinID，校验配方、成员 commit 和固定坐标的一致性；
3. 目标服务对本次 action 和每个 Repository 按当前权限求值。

`ResolvedKnowledgeSet` 不是 bearer capability：知道 PinID 或 commit 不能替代凭证。它也
不绑定身份、不带 TTL；token 过期或刷新不会改变 pin，selector 前进也不会改变本次
basis。客户端可以让 SDK 隐藏 basis 的重复传输。服务内部也可以按 PinID 做不可变
解析缓存，或让网关签发完整性保护的 basis envelope，但这些都是无身份、可重建的
传输优化，不成为公开 `sessionId`、续租协议或服务端 Session Store。

认证成本同样不需要 WorkspaceSession 解决：JWT/OIDC token 可由边界服务本地验签；
opaque token introspection 可以在认证器内部按凭证摘要做短 TTL 缓存。缓存失效窗口
属于认证器策略，不能改变 ResolvedKnowledgeSet 或授权语义。

### 3.5 Catalog API 资源

目标 API 采用资源化接口。`POST /v1/<verb>` 不再存在，也不提供兼容开关。Catalog namespace 的具体 method/path 以公开 HTTP registry 与 `TEST_CATALOG.md` 路由分母为准，本文不复制路由表。

管理写请求继续使用 revision/CAS 和 `requestId`，不能退化成最后写者覆盖。

两个 resolve 都返回不含授权能力的 ResolvedKnowledgeSet。集合级 resolve 接受客户端提交的临时 KnowledgeSet，只验证、授权和解析，不调用 `DefineKnowledgeSet`，不写 Catalog Registry。重放时提交命名 Workspace 引用或临时配方；服务重新校验成员、commit、PinID 与当前权限。

### 3.6 Workspace File Gateway

远程 `kcfs` 需要读取固定 Snapshot 的 path/tree/blob。这属于 Catalog Plane 的宿主数据接缝，但不属于 `catalog/` 核心或 Knowledge Server。逻辑动作只有：列 mount、列一个目录的直接子项、按 offset/length 读 blob。没有递归列全树或写接口。它们不是新的 Store，也不改变 `snapshot.TreeStore` 的权威语义。路由在 `dataset-files/v1`。

---

## 4. Knowledge Server

### 4.1 责任

Knowledge Server 是结构感知的消费数据面。它认证每个请求，并在请求携带且校验通过的
固定 ResolvedKnowledgeSet basis 上装配：

```text
Knowledge Reader Service
  └── Canonical Repository wrapper / exact-basis ReadMany
retrieval planner / executor
Retriever providers
Snapshot Repository capabilities
observability recorder
```

它拥有业务含义上的知识读取，不拥有 Repository 或 Workspace 的生命周期。
Catalog Server 交付的仍是 `snapshot.Store`；应用装配根显式取得该 authority 的
`knowledge.Repository` 能力后跨入②。精确 READ、SchemaLocator、BindingLocator 与维护
Scanner 是相互独立的可选能力；不能因为 authority 有 TreeStore 就在消费请求中扫描文件
来补齐缺失能力。Catalog 和 Snapshot Registry 都不暴露知识方法。

### 4.2 消费 API

Knowledge namespace 的 method/path 与 DTO 以 HTTP registry、`retrieval/README.md` 和 `knowledge/reader/README.md` 为准。本文只冻结边界：

- 没有 Knowledge LIST。未知对象用 SEARCH；SEARCH 不可用时返回 capability/completeness，不得全仓扫描。
- Schema 目录只分页枚举固定 basis 上的 Schema，不是对象 LIST。
- Catalog 库存只列源身份（及 `schemaCount`）；README 是仓内知识对象，缺少时不得由平台补写，也不得展成 title/summary。
- 对象 RESOLVE 只判断固定 basis 上的对象状态，不重新解析组合。LOG 必须有界分页。
- Binding 属于一个确定 Address 单元，不能只用 ObjectID 猜测目标。
- 消费者 API 使用固定 ResolvedKnowledgeSet；单仓维护读、索引维护和 DIFF 用各自授权与入口，不得混淆维护坐标和消费任务坐标。
- 查询表达式形状属于 `retrieval/`。服务端不接受字符串布尔查询 DSL。
- RERANK 是显式候选上的 Refine，不能生成新 Ref。Provider 只能重排、并列或声明输入未评判。公开结果保留 SearchView、未入选与未评判的区别，以及 provider/model/spec/candidate digest 证据。非确定性 rerank 不得塞进 SEARCH continuation。
- `search:rerank` 是同一次 pin 上的薄物理组合，不是 Logical Retrieval Program：SEARCH 产生有界 CandidateWindow 及真实 lane/local rank 证据，RERANK 复用同一 SearchView 做一次 listwise 判断。含 continuation 或超过候选/字节预算的请求在模型调用前拒绝，不通过自动分批改变全局排序。物理 rank/score 进入审计证据但不进入模型请求。
### 4.3 Workspace capability 选择

Knowledge Server 不假定 Workspace 的所有成员都是 Knowledge Repository。它对固定 pin 逐成员执行：

```text
snapshot.Store
  → Knowledge Reader Service.Lookup
      supported   → 进入 Reader / AccessSpec / RetrievalPlan
      unsupported → 不解释文件；记录 coverage claim
```

已知对象的 READ/RELATIONS 等无 completeness 信封时继续 fail closed，不能把不支持或无权成员伪装成空结果。SEARCH 有 completeness/claims，可以检索知识能力成员，并把无能力、投影缺失和预算耗尽分别报告。仓级读权与正文交付见 [`PERMISSIONS.md`](PERMISSIONS.md) §7.2。挂载完全不经过这个选择过程。

### 4.4 Search 执行

```text
SearchRequest
  → 从固定 ResolvedKnowledgeSet 读取 schema/*
  → 为每个成员生成 AccessSpec
  → Provider.Probe 每条 clause
  → 编译 RetrievalPlan
  → OpenSearch 返回 CandidateRef
  → 校验 repository 与 basis
  → 同版本 hydrate 端口（缓存命中；未命中部分批量回读 Canonical）
  → residual filter
  → SearchResult（检索合同，含 hydrate 后的 Canonical）
  → 交付链（`PERMISSIONS.md` §7.2；公开类型 `delivery.Chain`）
  → 调用方
```

显式 Refine 的独立执行链为：

```text
KnowledgeRef[] + SemanticOperatorSpec
  → ResolveKnowledgeSet 一次
  → per-ref authorization
  → same-basis Canonical READ / State hydrate
  → EvaluationProjection
  → injected batch Reranker
  → validate ref-preserving partition
  → RankGroups + notSelected + unjudged + evidence
```

SEARCH 与 Refine 的有界组合链为：

```text
SearchRequest + SemanticOperatorSpec
  → ResolveKnowledgeSet once
  → SEARCH / bounded CandidateWindow
  → Canonical hydrate + per-hit authorization
  → preserve provider/lane/originalRank evidence
  → EvaluationProjection + byte budget
  → one bounded listwise Reranker call
  → Retrieval evidence + SemanticRerankResult
```

结果必须保留固定来源、知识版本、完整性解释和检索证据；分页不能脱离生成该页的查询与
投影 basis。具体信封与 continuation 合同由 [`retrieval/README.md`](../retrieval/README.md)
及公开类型拥有。模型参数属于 Provider 配置，不成为消费协议。

Provider 的 `_source`、stored fields、summary、score payload 都不能冒充知识正文。

#### Workspace 是请求范围，不是索引字段

Workspace 范围由 `ResolvedKnowledgeSet` 在请求开始时给出，不写进知识正文、`CompiledDoc`
或 OpenSearch 文档。Knowledge Server 先在授权后得到本次可见的
`{Repository → commit}`，再为每个成员编译固定 basis 的 Retrieval fragment，最后合并候选并
回读 Canonical：

```text
KnowledgeSet
  → ResolvedKnowledgeSet / PinID
  → authorized (Repository, commit) fragments
  → OpenSearch candidate search
  → union / residual / hydrate
  → SearchResult.SearchView
```

同一 Repository commit 可以同时被任意多个 Workspace 引用，物理投影仍只建立一次。不得给
文档增加 `workspace_id/workspace_ids`：Workspace 配方或 revision 改变不应重写知识投影，也
不能把 Workspace membership 误当成 Repository 授权。

OpenSearch 可在一次请求中搜索多个 generation index，也可用 `_msearch` 降低扇出的网络往返。
若部署为了热点 pin 建 alias，它只能是绑定不可变 PinID 的可丢优化，并且要有 TTL/回收；alias
不是 Workspace 或权限权威，不能改变 SearchView、basis 校验和 Canonical hydrate。默认实现不
为每个 Workspace 建 index 或永久 alias。

### 4.5 Projection basis

一把物理投影对应：

```text
(repository, basisCommit, provider, physicalDigest)
```

它不对应 Workspace。Workspace SEARCH 按本次 pin 扇出到各成员投影。请求旧 commit 时：

- 优先选择同 basis 的投影；
- 能证明不漏候选时才允许 superset + residual；
- 只能 approximate 或缺少 basis 时返回 `partial`/明确能力错误；
- 禁止查询 live 索引后把旧 basis 中未命中的对象静默当成不存在。

### 4.6 精确读优先

已知 ObjectID/Address 时直接走 Reader，不先搜索。一个 `object_id` 在多个成员仓中可以有多个独立值；Knowledge Server 返回来源保留的 union，不做 public/group/personal 覆盖。

### 4.7 动态 Binding

基础 Reader 的 `RESOLVE_BINDING` 只返回固定声明。面向消费者的 Knowledge Serving 对精确
`READ` 检查每个 Address 的 `ValueSource`：Snapshot 返回 commit 中的值；State Binding 经应用
注入的 `StateLookup` 取得值与 observation basis；Stream Binding 在普通 READ 上明确返回
`CAPABILITY_UNSATISFIED`，等待独立 window/query surface。不得把 cursor、watermark 或运行
generation 塞入 Catalog pin。

返回值必须同时保留 declaration commit/digest 与 observation basis。部署没有 State runtime 时，
Bound State READ 失败关闭，不得把 Repository 中的 `null` 占位返回成业务值。Repository 维护读、
VFS 与 checkout 仍是固定 Snapshot/声明视图，不调用 runtime。

服务经显式注入的 Resource Access 端口调用独立 runtime；配置与 HTTP 合同见
[`cli/README.md`](../cli/README.md) 及 `resource-access/v1` registry。这里的“墙外”是服务所有权和协议边界，不是
“只能本机进程外”：Knowledge Server 与 runtime 可以分别位于 Docker 容器中，通过服务 DNS
通信。首版只要求每个逻辑服务单实例，不因此引入副本一致性、选主或分片协议。

动态字段参与发现时仍须有可证明 basis 的 Retrieval 路由，不能扫描运行值补齐候选。
命中随后经过 Knowledge Serving hydrate，State 单元保留运行值与 observation basis，
与精确 READ 遵守同一声明和来源约束。动态发现、projection coverage 与双 basis 由
[`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) 拥有，实现覆盖只在验收文档维护。

目标形态由现有 `index` 控制链同时接收 Snapshot advance 与 source observation notice，维护独立的
Snapshot projection 和动态 State projection。具体绑定后拼装、coverage、失效、basis、Docker
旅程与验收矩阵见 `PROJECTION_CONTROLLER.md`。该控制链是索引唯一写入者；Observer
只通知，Resource Access 按固定 Binding 返回 observation，二者都不直写 OpenSearch。

新观察按固定 Binding 向 runtime 取值，并返回声明依据与观察依据；查询命中、分页和复核则
使用已固定依据下的完整观察，不能重新取 latest 替代原值。变更信号只承担发现，不承担取值；
信号丢失必须能由重读或对账恢复。重读能力、观察记录和生命周期由
[`LIVE_MATERIALIZATION.md`](LIVE_MATERIALIZATION.md) §6 拥有。Resource Access 端口缺能力时
必须失败关闭：不得把信号载荷当知识值，不得降级为扫描，也不得返回占位空值冒充业务值。

统一访问层负责组合本次消费的声明范围、观察依据、时效要求与当前授权，再交付可解释的结果；
它不新增动态 Store，不把控制器调度塞入消费请求，也不替墙外 runtime 实现通用流处理。
应用装配需要给后台维护提供仍在服务的固定声明需求，并为来源访问配置独立身份；已发布
Dataset 的 Snapshot 投影就绪不能证明其动态观察就绪。源授权与共享观察的边界由
`PERMISSIONS.md` §2 拥有，来源拒绝后不能改从平台记录绕过。

### 4.8 Canonical hydrate 边界

READ、SEARCH 和 RELATIONS 复用服务端内部的同版本 hydrate 端口。检索仍先校验候选坐标，
只取得当前页所需的完整知识；缓存不改变公开响应，也不把正文补读转交给 SDK：

```text
CandidateRef page
  → verify repository + exact basis
  → hydrate port
      → complete Snapshot cache hits
      → ReadMany(missing IDs, exact commit) → decode and verify Canonical
  → independent result copies → residual / delivery authorization
```

Knowledge Reader、Serving 与检索执行器只依赖 Knowledge 拥有的 hydrate 端口；默认未注入时
保持原有固定版本读取。应用装配根注入上层 retriever lane 的缓存实现，不能让 Reader、Writer、
Catalog 或 Snapshot import 具体缓存。端口与参考实现分别见 [Knowledge 公开合同](../knowledge/README.md)
和 [正文缓存合同](../retrieval/cache/README.md)。这不是新服务、协议层或 Writer target。

缓存身份包含 Repository、不可变 commit 与完整读取身份，完整对象与单个 Address 读取不能混用；
同一知识 ID 在另一仓、另一版本或另一 Address 上不能命中。缓存保存由固定 authority 解释的完整
Snapshot 值、声明和来源，输入与返回值均须隔离可变副本。调用方修改一次结果不能污染缓存或另一次读取。
首版选择容量有界的进程内 LRU，批量读取只向 authority 发送未命中的子集；错误与不完整读取不得
形成成功条目。缓存驱逐或进程重启后，在同一固定 commit 回源即可恢复读取。

不可变条目不需要在新 commit 发布时删除才正确；后续请求使用新版本键，旧 pin 仍按旧版本读取。
热点预热是独立的后台派生消费者，受容量与批次约束；显式启用冷启动预热后才可取有限维护页，不做全仓物化。
消费请求的 cache miss 仍直接批量回源，不同步启动维护扫描。预热不承诺全仓就绪，也不决定 SEARCH
投影是否 READY；控制器恢复与预热生命周期由 `PROJECTION_CONTROLLER.md` 拥有。

缓存只复用内容，不复用允许交付的决定。每次读取沿用当前请求授权与交付链，命中旧 pin 的条目也不能
绕过撤权。Snapshot 中的 Binding 声明可以缓存，墙外 runtime 的动态 observation 及其拼装结果不能
作为该 commit 的 Snapshot 值缓存；动态同依据回读继续由 Serving State 合同承担。

Snapshot Adapter 仍可缓存 HTTP connection、原始 commit tree、blob SHA/bytes 或数据库执行计划，
但不得解释 `object_id`/Aspect 以持有完整知识对象。同一次 authority `ReadMany` 的解析结果仍在调用
结束后释放。OpenSearch 的 query/request cache 只优化候选定位，不能成为 Canonical 正文。

---

## 5. KC Client

### 5.1 对外只有一个客户端产品

交付客户端封装唯一服务入口与配对信息。接入方、消费方登录后，在既定授权范围内自行完成
知识源接入、发布维护、选源消费与显式更新；日常步骤不要求部署方修改配置、代建知识集或代为发布。
普通用户登录不得要求携带部署应用秘密。部署方负责一次性初始化平台能力、身份接入与默认策略，
连接配置和凭证由服务管理并持久保存；具体用户旅程由 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` 定义。

调用者不应安装三个互不相关的 CLI。统一 KC Client 内部包含：

```text
KC Client
├── Identity / Authentication
│   ├── login / logout
│   ├── client-local credential store
│   └── per-audience request authentication
├── CatalogClient
│   ├── repositories
│   ├── workspaces
│   └── resolveWorkspace
├── KnowledgeClient
│   ├── describeSchema
│   ├── search
│   ├── resolve / read / readAddress
│   ├── relations
│   └── provenance / log / binding
├── WriterClient
│   ├── commit
│   ├── proposal
│   └── receipt
└── MountController
    ├── buildPlan
    ├── start
    └── unmount
```

CLI、Go SDK 和其它语言 SDK 使用同一协议模型。`CatalogClient` 是 SDK 模块，不是 VFS 实现。

客户端身份与凭证分开：`Identity` 是 `principal/onBehalfOf`，`Authentication` 是不进入
Catalog、Repository、Workspace pin 或 telemetry baggage 的秘密。`Login/Logout` 只改变
客户端凭证库，不在 KC Server 创建会话资源。每个远程请求都重新从当前登录态
取身份和目标 audience 的凭证，因此 token refresh 不会改变 `ResolvedKnowledgeSet`/PinID。

Client 必须与 Server `--auth` 配对，见 §8.1。`client.PassThroughAuthenticator` 只校验
形状；产品 CLI 在 token 配对下只发 `Authorization`，在 local 配对下只发 `X-Kc-As`，
不得混装。SDK 调用方替换 `client.Authenticator` 时也必须遵守同一配对。

### 5.2 一次任务的固定 Workspace

客户端应提供任务级对象，避免每个调用重新 Resolve，但它只是本地 SDK 对固定 basis
的封装，不是远程 Session 资源。打开命名 Workspace 或本地临时配方时，客户端取得
一次 `ResolvedKnowledgeSet`：

这个本地对象应支持：解析命名或临时配方；恢复已经保存的固定 basis；在同一 basis 上读取、
搜索和挂载。这里规定任务生命周期，不定义另一组 SDK 方法。已选定的 typed Client 接口和
请求形状由 [`client/README.md`](../client/README.md) 与该包公开代码维护。

任务对象持有固定 KnowledgeSet 与 `ResolvedKnowledgeSet`，每次远程调用仍携带
当前凭证并重新授权。SDK 刷新 access token 不改变 PinID；需要跟随分支时必须显式
重新 Resolve，形成新的 ResolvedKnowledgeSet。用户不创建、保存、续租或关闭 sessionId。

### 5.3 CLI 体验

产品 argv、help 与操作数由 [`CLI.md`](CLI.md) 拥有；公开命令、flag 与 HTTP 对应以 CLI / HTTP registry 为准。一次远程任务的顺序是：配对登录 → 解析命名 Workspace 或临时配方得到 pin → 同一 pin 上 SEARCH/READ → 需要宿主文件时经 Workspace File Gateway 挂载。

本机 `--auth local` 只发送 `X-Kc-As`。产品 token 配对下身份来自 token，不能再带 `--as`。

知识面 CLI 与 `kc catalog` 等消费命令没有公开 `--home` 旁路；部署管理与 Server 读取显式持久配置。`kcfs` 必须连接 Workspace File Gateway，不能直接打开 Repository。组件测试可以进程内调用 Application Services，但该接缝不是产品 transport。

命名或临时 Workspace 的便捷消费入口在一次任务开始时解析配方；临时配方不在服务端创建 Workspace。跨命令复现保存不含授权能力的 pin，并保留命名 Workspace revision 或同一配方；再次使用时以当前身份为同一 PinID 重新校验。单仓维护入口不能改变这条组合规则。

Catalog 范围发现遵守 §2.5；公开命令是否覆盖该能力以 CLI 合同和验收文档为准。交付链后续隐私化未选定，禁止实现。

`kcfs mount` 只接受 KnowledgeSet + ResolvedKnowledgeSet，不接受 Catalog scope：Catalog 范围搜索是发现入口，挂载前必须显式选择 Workspace。普通成员仓可以用文件工具；只有结构化 search/read/relations 进入 Knowledge Plane。

---

## 6. MountController 与 kcfs

### 6.1 边界

MountController 是 KC Client 的应用层编排；`kcfs` 是宿主投影进程：

```text
CatalogClient.ResolveKnowledgeSet / ResolveDefinition
  → ResolvedKnowledgeSet
  → Workspace File Gateway
  → immutable workspacefs.Plan
  → kcfs mount
```

挂载计划必须把本机路径映射与 Repository 固定版本绑定；其公开形状由
[`datasetfs/plan.go`](../datasetfs/plan.go) 拥有。

MountController 持有当前凭证提供器、固定 basis 和远程 FileReader；`datasetfs/` 只看到固定
坐标和 FileReader 接口，不 import Catalog、Knowledge、Reader 或 Retrieval。
协议装配留在 Client 应用层。挂载普通仓不要求 `knowledge.Lookup` 成功。

### 6.2 远程读取模式

所有产品拓扑均由 Workspace File Gateway 代理固定文件读取，这样可以：

- 不把服务机器凭证下发给客户端；
- 每次服务端 fetch 认证请求并校验当前 Repository 授权；
- 记录 semantic action `file.read` 的访问证据；
- 保持所有 bytes 绑定同一 commit。

本机与共享部署遵守同一入口；客户端不直接向 Snapshot authority 取文件。用户另行取得的
Git clone 不属于这条服务读取链，不能声称拥有同样的逐次授权与访问证据。

### 6.3 凭证刷新与撤权

MountController 可以通过凭证提供器刷新 access token，但固定 basis 始终是同一个 PinID；
任何实现都不得借 token 刷新跟随分支。认证或授权失败后：

- 停止新的远程 fetch，并把 mount 标记为 degraded/unauthorized；
- 未缓存路径的后续读取返回明确 I/O/授权错误；
- 不自动切换到新 commit，也不静默返回空文件；
- 可由部署策略选择显式卸载，但不能声称已收回进程、内核页缓存或用户已经复制的 bytes。

授权的可执行边界是 Resolve、每次服务端 fetch 和显式重新挂载。FUSE 内核缓存、进程内缓存和
客户端已收到的内容可能在撤权后继续可见；这不是服务能够倒转的事实。高敏 profile
可以缩短 access token TTL、关闭持久缓存或加密缓存，但仍不能撤回已经交付的数据。

### 6.4 缓存

文件 bytes 由 `(repository, commit, path, digest)` 标识，可以安全做本地内容缓存。缓存不得：

- 按 selector 或 Workspace 名作为内容键；
- 在权限撤销后继续建立新 mount；
- 把缓存目录当 Canonical；
- 允许向缓存写入后自动回传 Repository。

缓存命中不代表当前仍有远程读取权。共享客户端若在 mount 生命周期之外复用缓存，必须
先重新授权；即便如此，平台也只能阻止受控接口继续交付，不能保证删除用户可直接
访问的旧缓存副本。

### 6.5 平台

`kcfs` 是 FUSE 上的宿主投影，不是协议层。无 FUSE 的宿主走 KnowledgeClient / Workspace File Gateway 按需读取，或基于 typed streaming API 的显式物化。具体宿主支持由包合同和验收文档记录，不把某次实现只覆盖的 OS 写成协议 Non-Goal。

不提供让 CLI 直开 Server Home 并写宿主路径的 checkout 旁路。任何物化都必须使用相同 ResolvedKnowledgeSet/PinID，不允许出现第二套 latest 语义。

---

## 7. 接入写面

普通 Repository 仍由它自己的 authority 和工作流维护；Catalog Plane 不要求其中
的文件符合知识规范。接入方决定发布 Knowledge Repository 时，必须通过下面
的标准化写面发布 Schema、Entity、Aspect 和 Relation。接入方不向 Catalog Server
上传源数据，也不直写 OpenSearch：

```text
Source
  → Provider-owned Collector
  → stable source key → Address
  → Collector 在明确 Scope 内预览对账
  → ChangeSet(PUT/REMOVE)
  → Writer API COMMIT
  → Snapshot advanced event
  → ProjectionMaintainer
```

### 7.1 Writer API

逻辑动作是单仓 COMMIT / PROPOSAL、读 HEAD、按 commandId 查 Receipt。路径以 `writer/v1` registry 为准。目录写入是 `writer commit --dir`：Client 对照当前 HEAD 求差后提交。HTTP Writer 仍收 ChangeSet。

请求必须指明幂等身份、预期目标版本、显式变更与来源；相同命令不能被用于提交另一份内容，
过期目标必须先处理冲突。一次请求只能写一个 Repository。字段、Receipt 与错误码见
[`knowledge/writer/README.md`](../knowledge/writer/README.md)、`kernel/` 与 Conformance。

### 7.2 Schema

Schema 是知识，Connector 第一次同步前由 Writer API 写入目标 Repository 的 `schema/*` 对象。Schema 草稿、源客户端、密码和调度配置属于 Provider 工程，不进入 Catalog Server。

Domain Schema 先由内置 Meta Schema 校验；同批 ChangeSet 可以一起发布 Schema 和引用实例。
Writer 按同一 target Repository 固定 basis 验证声明和实例；声明不合法、实例不符合约束、
引用无法解析都必须拒绝整批写入。产品生命周期见 [`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`](KNOWLEDGE_PRODUCT_AND_SCHEMA.md)，
校验形状与错误码见 [`knowledge/writer/README.md`](../knowledge/writer/README.md)。

Workspace File Gateway 支持两种明确视图：`repository` 原样投影已声明子树；`semantic`
在显式 attach/mount 阶段从固定 pin 构建并缓存 YAML 消费投影。后者按 Domain Schema Entity
形成 `metrics/`、`tables/` 等目录，文件保留 `_kc` 坐标，不接受写回。

### 7.3 投影事件

Snapshot 推进通知是追赶投影的快路径；常驻服务仍须在启动时及周期运行中，把发布目标与
投影 basis 对账，保证通知丢失后可以恢复。多实例间的通知必须可持久重放并保留来源与版本
跃迁，不能用进程回调承诺跨实例交付。控制循环与通知合同由
[`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) 及 `index/` 公开代码拥有。

消费者必须幂等；事件只通知“某仓从哪到哪”，ProjectionMaintainer 自己计算对象变化和物理代际。Catalog/Writer 核心不能 import `index/`。

### 7.4 Governance、Admin 与 Operations

治理与运维不复用 Writer 或 Knowledge consumer route。公开 namespace 是 `/governance/v1`、`/identity/v1`、`/admin/v1`、`/operations/v1`；具体 method/path 以 HTTP registry 为准。

Operations 中的 retrieval/refine 是非 Canonical 证据查询面，不执行检索。Knowledge Server 执行
SEARCH/RELATION/RERANK 后先写 access，再写 retrieval/refine 原始证据；训练接口只重建带标签强度的
派生样本，不能把模型输出反写为知识或监督真值。

部署初始化与检查读取声明式配置，属于运维过程；Server 启动只恢复。配置声明 Catalog Snapshot 权威、Repository binding、持久状态位置和身份装配，秘密由部署环境注入。公开命令及配置类型以 `cli/`、`home/` 的 README 和 Go 类型为准。

Repository 接入是 Catalog 应用操作：接入方通过客户端申请平台仓或连接自有仓，由服务按既定策略准备并保存连接；仓库供给、连接管理与成员登记各有明确结果，登记本身不创建 Snapshot。在可用 binding 上只读打开既有 authority，校验身份、可用性和固定 HEAD，再把成员登记原子提交到 Catalog Snapshot 权威。对于既有来源的 attach，验证或登记失败不得创建 Snapshot、修改其 ref 或留下半个成员登记。接入不隐式授予知识读权；首次维护权限与消费共享必须来自显式授权策略，不能借自助入口扩大权限。平台仓供给是显式的管理写操作：服务按部署选择的存储池分配 Snapshot，持久保存连接及创建进度，再完成 Catalog 准入和显式创建者策略。成功必须形成可直接进行后续 Writer/Reader 操作的结果。创建请求与结果具备稳定命令身份；未完成和已就绪的创建进度及连接记录属于独立服务耐久账，不能只存在于进程 Store 或 Catalog 缓存中。跨介质失败不得假装全部回滚；恢复根据原请求与已完成阶段继续，不能覆盖既有仓或重复发权。Server 启动仍只恢复，普通 attach 仍只读验证已有 authority。当前实现与完整自助旅程之间的缺口见 `MVP_ACCEPTANCE.md`。

---

## 8. 身份、权限与秘密

### 8.1 身份

边界服务从每个请求的凭证注入稳定身份。没有 Server session / `sessionId`。

```text
principal   = 实际执行主体
onBehalfOf  = 可选的被代理用户；仅认证器可注入
```

Agent 代理用户时不能把用户冒充成 principal。授权词表见
[`PERMISSIONS.md`](PERMISSIONS.md) §7.3。

配对发现是无凭证的 identity 资源，报告服务支持的认证方式。它不是会话，也不发权。
客户端先发现再登录；产品部署必须显式选择认证器，不能因配置缺失退回本地身份断言。
混装、缺失或不匹配的凭证必须失败关闭，并明确区别认证错配与授权不足。具体 header、
错误码、模式发现响应及进程内测试接缝由 [`cli/README.md`](../cli/README.md)、
[`client/README.md`](../client/README.md) 和 HTTP registry 拥有。

`onBehalfOf` 只有在 IdP 委托声明、token exchange 或可信反向代理签名已被认证器
验证后才能注入。Gitea 认证器不提供委托，因此拒绝客户端自报 `onBehalfOf`。local
也不能用 header 自报委托。

### 8.2 授权

默认安全边界是 Repository：

```text
principal × action × repository → allow | deny
```

- Workspace 配方不发权；
- Catalog 与 Knowledge 共用一份 `PolicyEvaluator` 合同和 action 词表，但在各自
  边界独立执行，不互相代判；
- Resolve、READ、SEARCH、VFS fetch 分别按当前权限求值；
- ResolvedKnowledgeSet/PinID 不冻结授权，也不是 bearer capability；
- Catalog 范围 SEARCH 与交付链首段见 [`PERMISSIONS.md`](PERMISSIONS.md) §7.2；
- 无 completeness 信封的 READ/RELATIONS 等按现有规则 fail closed；
- VFS 清楚报告实际可见 mounts，不能冒充完整知识搜索，也不能承诺撤回已交付 bytes。

### 8.3 秘密

三类凭证必须分开：

| 凭证 | 使用者 | 存放位置 |
|---|---|---|
| 用户访问 token | KC Client | OS keychain / agent credential store |
| 服务读取 Repository 的机器凭证 | Server | Secret Manager / deployment env |
| Connector 访问源系统的凭证 | Provider runtime | Provider Secret Manager |

Binding/ResourceDescriptor、Catalog Registry、Schema 和日志都不能保存这些 secret。
Taihu 网关 HMAC 与资源方 client_secret 只进部署环境
（`KC_TAIHU_HMAC_SECRET`、`KC_SERVICE_CLIENT_SECRET`），见
[`DEPLOY_AUTH.md`](DEPLOY_AUTH.md)；用户 Bearer 走 `kc login` 或 `KC_AUTH_TOKEN`，
不要把字面量写进 git 或 argv。

---

## 9. 一致性与失败语义

### 9.1 一致性单位

- Catalog registry 修改：Catalog 自身 CAS/revision；
- Repository 写入：单仓 ref CAS；
- 消费任务：一个 ResolvedKnowledgeSet/PinID；
- Search page：query + SearchView + projection；
- VFS mount 生命周期：一个不可变 Plan/PinID；
- 动态取值：一次观察（固定声明依据 + 观察依据），不同取值之间没有全局原子切；
- 跨 Repository：没有事务。

### 9.2 错误信封

服务返回稳定 `error.code`，形状与码表以 `kernel/` 和 `TestProtocolErrorJSON` 为准。本文只冻结：索引延迟不是“知识不存在”；Knowledge Server 必须通过 completeness/claims 或明确错误暴露。

---

## 10. 可观测性

本节只声明服务边界需要携带的业务关联信息；运行 metric/log/distributed trace、传播、健康、SLI/SLO 与 Conformance 统一见 [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md)。知识访问证据仍见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

每个服务必须传递调用关联上下文和可信身份；消费证据要能还原当次任务的固定来源、实际
目标、动作及结果，并解释搜索覆盖范围。原始证据字段由 [`observability/README.md`](../observability/README.md)
与公开类型维护，遥测传播合同由系统可观测性 owner 维护。

服务日志、访问账和 projection hitmap 都是过程证据，不写回 Canonical。直接绕过服务读取 Git clone 或索引时，平台不能声称拥有逐条访问审计。

---

## 11. 部署拓扑

Server 是知识系统唯一运行边界，不以多方共享为前提。单机部署仍启动 KC Server；KC Client、Connector 与 `kcfs` 经 loopback typed API 调用它。单机与共享部署只替换认证和 Store/Retrieval adapter，不能改写 ResolvedKnowledgeSet、SearchResult、Writer、授权、证据或索引语义。

```text
单机：Client/Connector/kcfs → 127.0.0.1 KC Server → 本机 Store / Retrieval provider
共享：Client/Connector/kcfs → 远程 KC Server    → 部署 Store / Retrieval provider
```

### 11.1 第一阶段：模块化单体

```text
kc-server
├── /catalog/v1
├── /dataset-files/v1
├── /knowledge/v1
├── /writer/v1
├── /governance/v1
├── /identity/v1 + /admin/v1
├── /operations/v1
├── Catalog Registry
├── Store Directory
├── Reader/Retrieval
└── projection outbox worker
```

优点是复用当前 Go 装配、减少远程跳数；逻辑边界仍由包依赖和 API namespace 保证。

进程工作盘只容纳可重建缓存。重新部署连接同一 Catalog Snapshot 权威和知识 Snapshot authority，并恢复独立的耐久状态：授权与治理配置、托管仓连接及创建进度、Writer command ledger、Proposal/Preview/validation、Hook outbox 和原始访问证据。配置本身也需要持久来源；恢复 Catalog Snapshot 不能替代这些状态的恢复。客户端 overlay、凭证、任务 pin 和挂载生命周期不属于 Server 权威。具体存储介质按状态性质选择，不要求全部进入同一仓库。 首次初始化跨 Catalog Snapshot 与服务状态介质，不承诺跨介质事务；若中断后已有 Catalog 而缺耐久状态，必须先恢复或核对未完成部署，不能再次建立空授权账。

第一阶段的容器清单不拆逻辑服务。`kc-server` 仍是唯一知识运行边界；lakeFS 只提供 Graveler 控制面，PostgreSQL 只给 lakeFS，对象存储只当字节数据面，OpenSearch 只当可重建检索投影，可观测管道不进入 Snapshot 或 Catalog。

```text
自管：kc-server + lakeFS + PostgreSQL + OpenSearch + 可观测（1 容器） + 托管 COS
PG 与可观测都托管：kc-server + lakeFS + OpenSearch + 托管件
本地代替：上表中 COS → MinIO；可观测 → grafana/otel-lgtm（OTel Collector + Prometheus + Tempo + Loki + Grafana）
```

同一份本地陪伴清单用两个隔离的 compose 项目：可清盘的测试栈与可重启的开发栈，见 `scripts/deploy/`。本地 ttyd 只给远程操作员敲 `kc`，不是这张运行清单的一员。`kc serve` 的 typed API 根路径不是页面。

### 11.2 规模化拆分

只有出现独立扩缩容或安全边界时再拆：

```text
Catalog Server
Workspace File Gateway
Knowledge Server
Writer API
Projection Workers
```

拆分后：

- Catalog 仍不依赖 Knowledge；
- 每个边界服务验证请求凭证；Catalog 校验 ResolvedKnowledgeSet，数据面再对具体 action 求值；
- Knowledge 与 Workspace File Gateway 分别对具体 action 执行共享 PolicyEvaluator；
- Writer 只推进单仓 Snapshot；
- projection event 使用 durable outbox；
- 任何服务都不能拥有跨仓写事务。

---

## 12. 不采用的方案

1. **Catalog Server 直接理解 Aspect。** 会破坏普通 Git 可组合能力和 ①/② 分层。
2. **Knowledge Server 直接返回 OpenSearch `_source`。** 会把派生投影变成伪权威。
3. **Catalog Client 自己实现另一套 VFS。** 宿主投影统一由 `kcfs/workspacefs` 承担。
4. **把 Workspace 建成一个联邦大索引。** Workspace 是请求时组合，索引按 Repository+basis 建。
5. **Workspace 只允许 Knowledge Repository。** 组合/挂载应保持内容无关；知识能力在使用时选择。
6. **用 Schema/Aspect 约束用户本地工作目录。** 规范约束的是 Knowledge 发布和结构化访问边界，不是任意文件开发。
7. **为远程读取引入 WorkspaceSession。** 固定数据已经由 ResolvedKnowledgeSet 表达；认证与撤权应逐请求处理，额外 session 只会制造状态、续租和可用性问题。
8. **Connector 写 Catalog 或 OpenSearch。** Connector 只生成 ChangeSet，Writer 推 Snapshot，投影随后派生。
9. **FUSE 写回自动 COMMIT。** VFS 不是 Writer Surface；知识写入必须显式走 Writer。只读是当前选定的宿主投影合同，直到存在显式 checkout/overlay + reconcile，也不能把普通 write 解释成 COMMIT。

---

## 13. 实现状态与验证入口

本篇只拥有逻辑服务、typed API、KC Client、Workspace File Gateway 和部署拓扑的设计结论，
不再维护 P0–P4 实施台账或逐组件“当前基础/主要缺口”表。状态分别由以下位置拥有：

- 当前可用命令和服务入口：根 `README.md`；
- 产品可用性与生产缺口：`MVP_ACCEPTANCE.md`；
- route、客户端、FUSE、动态投影和失败语义的机器证据：`TEST_CATALOG.md`；
- 可证伪的跨层约束：`ARCHITECTURE_INVARIANTS.md`。

实现改变不应在这里追加阶段记录；只有服务责任、请求边界或部署不变量改变时才修改本文。

---

## 14. 验收不变量

1. Catalog API 的 DTO 不出现 `object_id`、Aspect、Binding 或 AccessSpec。
2. Workspace 可以混合普通 Repository 与 Knowledge Repository；挂载不要求知识格式或 TreeStore。
3. Schema/Aspect 规范只约束 Knowledge 发布和结构化访问，不约束用户任意本地开发目录。
4. Knowledge 消费请求固定一个 ResolvedKnowledgeSet/PinID；命令中途不重新 Resolve。
5. 临时 `.kc-dataset.yaml` 可被 resolve，但不会隐式写入 Catalog Registry。
6. token 刷新不改变 PinID；每个请求和 Pin 重放都执行当前认证、授权。
7. SEARCH 只处理可由 Knowledge Reader 解释的成员，并诚实报告其它成员的 coverage claim。
8. SEARCH 的公开命中全部从相同 basis Canonical hydrate。
9. OpenSearch 故障或延迟不会被报告成“知识不存在”。
10. Workspace 配方不扩大 Repository 权限。
11. VFS 生命周期内 selector 前进不改变已挂载 bytes；撤权阻止新 fetch，但不宣称撤回已交付 bytes。
12. `datasetfs/` 只消费 Plan，不 import Catalog/Knowledge/Retrieval。
13. Binding resolve 以完整 Address 为目标，不以裸 ObjectID 猜测单元。
14. Connector 不写 Catalog、Git 文件或 OpenSearch，只提交 ChangeSet。
15. 一次 Writer 请求只有一个目标 Repository，并保留 CAS/幂等语义。

这些不变量应同时进入 API contract tests、`internal/arch` 和 Linux/FUSE E2E；设计完成不以“接口能返回 200”为标准。
