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
| Workspace File Gateway | 如何按固定 ResolvedWorkspace 提供不解释知识的 path/tree/blob？ |
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
- `kc local` 只初始化 Home、Store Directory 和首个管理主体；不执行知识、Catalog、Writer 或 Retrieval 操作。
- Knowledge 消费面没有 LIST。内部全量遍历命名为 Snapshot scan，只供重建、迁移、导出和验收。

---

## 2. 两级发现

“发现”有两个不同对象，不能由一个模糊的 Search API 承担。

### 2.1 Repository / Workspace 发现

由 Catalog Server 提供：

- 当前身份可见的 Repository；
- Repository 的治理边界和 Snapshot/TreeStore 能力摘要；
- 可使用的 Workspace；
- Workspace 的成员配方、revision 和 mount 布局；
- 一次 `ResolveWorkspace` 得到的固定 `{repository → commit}`。

Catalog 核心只认识 Repository identity、selector、commit 和 Workspace。Snapshot endpoint、驱动配置和机器凭证属于服务装配的 Store Directory，不进入 `catalog/` 协议类型。Aspect、Schema 和检索字段由 Knowledge Plane 在实际使用时解释，不出现在 Catalog DTO 中。

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
| Catalog | 服务端共享 | 组织级长期 | 被承认的 Repository、WorkspaceDefinition、治理历史 |
| WorkspaceDefinition | Catalog Server 或客户端本地文件 | 可版本化、可复用 | Repository 子集、selector、可选 mount path/subPath |
| ResolvedWorkspace | 请求或客户端 | 一次任务 | 固定 `{repository → commit}` 和 PinID；PinID 绑定配方路径布局 |
| VFS mount | 客户端本机 | 一个进程 | ResolvedWorkspace/PinID 的只读宿主投影 |

因此 Workspace 不是本地文件系统，也不是另一个 Repository。它是“这类任务需要同时组合哪些 Snapshot Repository”的命名配方。本地只持有配方文件、这次 Resolve 后的 pin 和可选 mount。

Workspace 可以有两种来源，但进入消费面后使用同一语义：

1. 组织在 Catalog Server 发布的共享 Workspace；
2. 客户端从 `.kc-workspace.yaml` 或显式参数形成的本地/临时配方。

便携 `.kc-workspace.yaml` 是配方载体，不是 Repository 或本地状态权威；它可以被发布为服务端 Workspace，也可以只用于生成一次临时 ResolvedWorkspace。本机 overlay、目标目录和 FUSE 生命周期不写回共享配方。远程个人 Workspace 只有在 owner、visibility、命名空间和生命周期协议完整后再增加，不作为 V1 前提。

### 2.4 Workspace 不要求知识格式

Workspace 可以混合普通 Repository 与 Knowledge Repository：

```text
ResolvedWorkspace
├── plain code/docs repo       mount / checkout / rg
├── knowledge repo A           mount + READ/SEARCH
└── knowledge repo B           mount + READ/SEARCH
```

Repository 注册、Workspace Resolve 和 Workspace File Gateway 都不要求 `object_id`、Aspect 或 Schema。Knowledge Server 只对显式提供 layer ② 精确读取能力的成员运行 Knowledge Reader；`TreeStore` 本身不能被推断为知识能力。缺少 Schema 精确读取能力或准备好的 Retrieval projection 时分别返回 capability/coverage，不在消费请求中遍历 tree。Relation 没有 layer ② locator：候选发现必须来自 exact-basis Retriever。用户要搜索普通文件时使用 mount 上的 `rg`，不能让 Knowledge SEARCH 退化为整包 JSON 或文件 contains。

知识规范是发布与结构化访问合同：接入方声称某 Repository 是知识提供方时，必须遵守 Address、Schema、Aspect、Relation、provenance、PUT/REMOVE 和 Writer CAS；用户在自己的宿主 Workspace 中开发普通文件不受这些格式约束。用户决定把成果发布为知识时，再通过 Connector 翻译为 ChangeSet 并进入 Writer。

### 2.5 Catalog 范围的知识搜索

Catalog Core 不理解知识，但产品仍应支持“在我可发现的整个 Catalog 中搜索”。该能力分两步完成：

```text
Catalog
  → 找到该 Catalog 配置的 discoveryWorkspaceId
  → 按普通 WorkspaceDefinition 执行 ResolveWorkspace
  → 取得当前身份可参与 SEARCH 的 ResolvedWorkspace
  → Knowledge Server SEARCH(ResolvedWorkspace)
```

Catalog 范围搜索不新增第二种组合代数。`discoveryWorkspaceId` 指向一条普通、管理员维护的 WorkspaceDefinition；`kc knowledge search --catalog` 只是“解析这条指定 Workspace，再调用 Knowledge SEARCH”的客户端语法糖。Catalog Server 仍然只做 Repository 选择和 Snapshot 坐标解析；真正的 capability、Schema 和 Aspect 查询在 Knowledge Server。

不能简单把“所有已注册 Repository 的默认分支”自动纳入搜索：注册表示 Catalog 承认该仓，不等于仓已发布或允许组织发现。管理员通过 discovery Workspace 显式选择 Repository 和 published selector。无权成员按 SEARCH 现有规则省略并返回 `partial` claim；普通成员和没有 tree 读取能力的成员同样进入 coverage claim，而不会阻止挂载。

---

## 3. Catalog Server

### 3.1 责任

Catalog Server 是组合控制面：

1. 管理 Catalog 生命周期；
2. 注册、归档和枚举 Repository identity；
3. 管理 WorkspaceDefinition；
4. 在请求开始时解析已保存或客户端临时提交的 WorkspaceDefinition，生成 ResolvedWorkspace；
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

### 3.4 ResolvedWorkspace 与请求认证

消费任务必须先 Resolve：

```text
WorkspaceDefinition
  ├─ kr://dw/physical @ refs/heads/main → commit A
  └─ kr://dw/semantic @ refs/heads/main → commit B

ResolvedWorkspace
  workspaceId: warehouse-agent
  revision: 7
  pinId: <digest>
  repositories:
    kr://dw/physical: A
    kr://dw/semantic: B
```

`ResolvedWorkspace` 是可导出、可复核的不可变坐标；PinID 绑定成员 commits 和
WorkspaceDefinition 中的路径布局。它不复制配方字段、不带 TTL，也不冻结权限。
跨命令重放时保存 `pin.json`，并继续提供产生它的同一 WorkspaceDefinition：命名
Workspace 使用 workspaceId + revision，本地配方继续提交同一 `.kc-workspace.yaml`。
Catalog 核心保持两个类型分离，不再为这组坐标增加第三种公开别名。

远程消费不再创建 Workspace 领域的 session。每个 Knowledge 或 Workspace File 请求
携带正常身份凭证，并提交完整 `ResolvedWorkspace` basis 及产生它的
WorkspaceDefinition 引用或临时配方。服务端执行三项独立校验：

1. 认证器验证凭证并注入 `principal`；请求体或普通 header 不能自报身份；
2. Catalog Server 复算 PinID，校验配方、成员 commit 和固定坐标的一致性；
3. 目标服务对本次 action 和每个 Repository 按当前权限求值。

`ResolvedWorkspace` 不是 bearer capability：知道 PinID 或 commit 不能替代凭证。它也
不绑定身份、不带 TTL；token 过期或刷新不会改变 pin，selector 前进也不会改变本次
basis。客户端可以让 SDK 隐藏 basis 的重复传输。服务内部也可以按 PinID 做不可变
解析缓存，或让网关签发完整性保护的 basis envelope，但这些都是无身份、可重建的
传输优化，不成为公开 `sessionId`、续租协议或服务端 Session Store。

认证成本同样不需要 WorkspaceSession 解决：JWT/OIDC token 可由边界服务本地验签；
opaque token introspection 可以在认证器内部按凭证摘要做短 TTL 缓存。缓存失效窗口
属于认证器策略，不能改变 ResolvedWorkspace 或授权语义。

### 3.5 Catalog API 资源

目标 API 采用资源化接口。`POST /v1/<verb>` 不再存在，也不提供兼容开关。

```text
GET    /catalog/v1/catalogs
GET    /catalog/v1/catalogs/{catalog}
GET    /catalog/v1/catalogs/{catalog}/audit
POST   /catalog/v1/catalogs/{catalog}/archive
GET    /catalog/v1/catalogs/{catalog}/repositories
POST   /catalog/v1/catalogs/{catalog}/repositories
POST   /catalog/v1/catalogs/{catalog}/repositories/{repository}/archive
GET    /catalog/v1/catalogs/{catalog}/workspaces
POST   /catalog/v1/catalogs/{catalog}/workspaces
GET    /catalog/v1/catalogs/{catalog}/workspaces/{workspace}
POST   /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/retire
POST   /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/resolve
POST   /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/check
POST   /catalog/v1/catalogs/{catalog}/workspaces/resolve
```

管理写请求继续使用 revision/CAS 和 `requestId`，不能退化成最后写者覆盖。

两个 resolve 接口都返回不含授权能力的 ResolvedWorkspace。集合级
`workspaces/resolve` 接受客户端提交的临时 WorkspaceDefinition，只验证、授权和
解析，不调用 `DefineWorkspace`，不写 Catalog Registry。重放 ResolvedWorkspace 时
提交命名 Workspace 引用或临时 WorkspaceDefinition；服务重新校验配方成员、commit、
PinID 与当前权限。

### 3.6 Workspace File Gateway

远程 `kcfs` 需要读取固定 Snapshot 的 path/tree/blob。这属于 Catalog Plane 的宿主数据接缝，但不属于 `catalog/` 核心或 Knowledge Server。逻辑上用独立 Workspace File Gateway 表达；模块化单体阶段可以与 Catalog Server 同进程部署：

```text
POST /workspace-files/v1/mounts:list
POST /workspace-files/v1/tree:list
POST /workspace-files/v1/file:read
```

请求体都携带 WorkspaceDefinition/引用、ResolvedWorkspace 和具体 path；响应只返回路径、
Repository、commit、digest、encoding 和 bytes。`tree:list` 只列一个目录的直接子项，
使用与 pin、mount、path 绑定的 continuation；`file:read` 接受 offset/length。没有递归
列全树或写接口。它们不是新的 Store，也不改变 `snapshot.TreeStore` 的权威语义。

---

## 4. Knowledge Server

### 4.1 责任

Knowledge Server 是结构感知的消费数据面。它认证每个请求，并在请求携带且校验通过的
固定 ResolvedWorkspace basis 上装配：

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

推荐的目标接口：

```text
POST /knowledge/v1/search
POST /knowledge/v1/search:rerank
POST /knowledge/v1/rerank
POST /knowledge/v1/objects:read
POST /knowledge/v1/addresses:read
POST /knowledge/v1/relations:query
POST /knowledge/v1/provenance:get
POST /knowledge/v1/log:get
POST /knowledge/v1/schemas:get
POST /knowledge/v1/bindings:resolve
```

SEARCH 的兼容字段 `query/match/equal/...` 组成隐式 `All`。需要组合时，请求使用结构化
`expression = {clause | all | any}`；排序通过请求级 `order` 传入一个 `SORT` clause。两种谓词
形态不能混用，`SORT` 不能成为表达式叶子。服务端不接受字符串布尔查询 DSL。

RERANK 是显式候选集合上的 Refine，不重新发现知识。服务端从同一 ResolvedWorkspace pin 校验并
Canonical 回读每个 KnowledgeRef，逐对象授权后只把 `EvaluationProjection` 白名单字段交给注入的
批量 Reranker。Provider 只能重排、并列或声明输入 Ref 未评判；不能生成新 Ref。公开结果保留
SearchView、未入选与未评判的区别，以及 provider/model/spec/candidate digest 证据。第一版不把
非确定性 rerank 塞进 SEARCH continuation。

`search:rerank` 是 Agent 可直接使用的薄物理组合，不是 Logical Retrieval Program。它在服务端只
解析一次 Workspace：SEARCH 产生有界 CandidateWindow 及真实 lane/local rank 证据，RERANK 复用
同一 SearchView 和已 hydrate 的值做一次 listwise 判断。响应保留 `retrieval` 与 `rerank` 两段；
物理 rank/score 进入审计证据但不进入模型请求。含 continuation 或超过候选/字节预算的请求在模型
调用前拒绝，不通过自动分批改变全局排序。

不存在 `/knowledge/v1/list`。已知对象直接 READ；未知对象使用 SEARCH；SEARCH 不可用时
返回明确 capability/completeness，不得改用全仓扫描。

`bindings:resolve` 的请求目标是完整 Address，不是裸 ObjectID；Binding 属于一个
确定 Aspect/member 单元。消费者 API 只接受固定 ResolvedWorkspace basis，不让普通调用方
混传 `--repo`、`--ref` 和 `--commit`。单仓直读、索引维护和 diff 属于维护 API，
必须使用不同授权动作和命名空间。

### 4.3 Workspace capability 选择

Knowledge Server 不假定 Workspace 的所有成员都是 Knowledge Repository。它对固定 pin 逐成员执行：

```text
snapshot.Store
  → Knowledge Reader Service.Lookup
      supported   → 进入 Reader / AccessSpec / RetrievalPlan
      unsupported → 不解释文件；记录 coverage claim
```

已知对象的 READ/RELATIONS 等无 completeness 信封时继续 fail closed，不能把不支持或无权成员伪装成空结果。SEARCH 有 completeness/claims，可以检索知识能力成员，并把无能力、无权限、投影缺失和预算耗尽分别报告。挂载完全不经过这个选择过程。

### 4.4 Search 执行

```text
SearchRequest
  → 从固定 ResolvedWorkspace 读取 schema/*
  → 为每个成员生成 AccessSpec
  → Provider.Probe 每条 clause
  → 编译 RetrievalPlan
  → OpenSearch 返回 CandidateRef
  → 校验 repository 与 basis
  → Knowledge Reader Service.ReadMany 在同一 commit 批量 hydrate
  → residual filter
  → SearchResult
```

显式 Refine 的独立执行链为：

```text
KnowledgeRef[] + SemanticOperatorSpec
  → ResolveWorkspace 一次
  → per-ref authorization
  → same-basis Canonical READ / State hydrate
  → EvaluationProjection
  → injected batch Reranker
  → validate ref-preserving partition
  → RankGroups + notSelected + unjudged + evidence
```

Agent 的 MVP 组合链为：

```text
SearchRequest + SemanticOperatorSpec
  → ResolveWorkspace once
  → SEARCH / bounded CandidateWindow
  → Canonical hydrate + per-hit authorization
  → preserve provider/lane/originalRank evidence
  → EvaluationProjection + byte budget
  → one listwise Reranker call (reasoning=none)
  → Retrieval evidence + SemanticRerankResult
```

公开结果必须携带：

- `SearchView.snapshots`；
- `Completeness` 与解释性 claims；
- 完整 `KnowledgeValue`；
- `KnowledgeVersion`；
- provider/lane/matched fields 证据；
- 与 query、SearchView、projection 绑定的 continuation。

Provider 的 `_source`、stored fields、summary、score payload 都不能冒充知识正文。

#### Workspace 是请求范围，不是索引字段

Workspace 范围由 `ResolvedWorkspace` 在请求开始时给出，不写进知识正文、`CompiledDoc`
或 OpenSearch 文档。Knowledge Server 先在授权后得到本次可见的
`{Repository → commit}`，再为每个成员编译固定 basis 的 Retrieval fragment，最后合并候选并
回读 Canonical：

```text
WorkspaceDefinition
  → ResolvedWorkspace / PinID
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

参考服务装配使用 `--resource-access-url` / `KC_RESOURCE_ACCESS_URL`，经
`resource-access/v1` HTTP 调用独立 runtime 服务。这里的“墙外”是服务所有权和协议边界，不是
“只能本机进程外”：Knowledge Server 与 runtime 可以分别位于 Docker 容器中，通过服务 DNS
通信。首版只要求每个逻辑服务单实例，不因此引入副本一致性、选主或分片协议。

```yaml
services:
  knowledge:
    command: ["kc", "serve", "--home", "/data/kc", "--listen", "0.0.0.0:7380"]
    environment:
      KC_RESOURCE_ACCESS_URL: http://resource-runtime:8090
  resource-runtime:
    image: company/resource-runtime:version
```

这里的 URL 是容器网络中的服务地址；不能把调用方容器自己的 `localhost` 当成另一个服务。

Workspace SEARCH 仍先由 Snapshot projection 找 CandidateRef，再从同一 pinned commit 回读；公开
命中随后经过相同 Knowledge Serving hydrate，State 单元返回运行值与 observation basis。这个能力
保证命中正文与 exact READ 一致，但尚不支持依靠未物化的动态字段发现候选。

目标形态由现有 `index` 控制链同时接收 Snapshot advance 与 source observation notice，维护独立的
Snapshot projection 和动态 State projection。具体绑定后拼装、coverage、失效、basis、Docker
旅程与验收矩阵见 `PROJECTION_CONTROLLER.md`。该控制链是索引唯一写入者；source
observer 只通知，具体 runtime 按固定 Binding 返回 observation，二者都不直写 OpenSearch。

### 4.8 Canonical hydrate 边界

底座不持有 Knowledge object cache。检索候选只按当前页在同一 basis 上回读：

```text
CandidateRef page
  → ReadMany(candidate IDs, exact commit)
  → decode and verify Canonical
  → discard request-local state
```

Snapshot Adapter 可以缓存 HTTP connection、原始 commit tree、blob SHA/bytes 或数据库执行计划，
但不得拥有 `object_id → KnowledgeValue` 缓存语义。同一次 `ReadMany` 可共享本次 tree/解析结果，
调用结束即释放。产品若需要对固定 `(repository, commit, object_id)` 的完整对象做缓存，
应在 KC 上层的 retriever lane 实现；本项目不提供 ObjectRetriever、Redis port 或分布式对象缓存接口。

Binding 声明和墙外 runtime 返回的动态 observation 也不进入底座对象缓存。OpenSearch
的 query/request cache 只优化候选定位，不能成为 Canonical 正文。

---

## 5. KC Client

### 5.1 对外只有一个客户端产品

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
│   ├── read / readAddress
│   ├── relations
│   └── provenance / binding
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
取身份和目标 audience 的凭证，因此 token refresh 不会改变 `ResolvedWorkspace`/PinID。
首批 `client.PassThroughAuthenticator` 只校验形状并直接携带身份/凭证，不得被描述为
生产认证；以后的 OIDC、Gitea 或部署 IdP 只替换 `client.Authenticator`。

### 5.2 一次任务的固定 Workspace

客户端应提供任务级对象，避免每个调用重新 Resolve，但它只是本地 SDK 对固定 basis
的封装，不是远程 Session 资源。打开命名 Workspace 或本地临时配方时，客户端取得
一次 `ResolvedWorkspace`：

```go
workspace, err := client.ResolveWorkspace(ctx, catalogID, workspaceID)

hits, err := workspace.Knowledge.Search(ctx, request)
value, err := workspace.Knowledge.Read(ctx, objectID, selector)
mount, err := workspace.Mount(ctx, target)
```

也可以从本地文件或已经保存的 pin 打开：

```go
workspace, err := client.ResolveDefinition(ctx, catalogID, workspaceDefinition)
workspace, err := client.UseResolved(ctx, workspaceDefinition, resolvedWorkspace)
```

任务对象持有固定 WorkspaceDefinition 与 `ResolvedWorkspace`，每次远程调用仍携带
当前凭证并重新授权。SDK 刷新 access token 不改变 PinID；需要跟随分支时必须显式
重新 Resolve，形成新的 ResolvedWorkspace。用户不创建、保存、续租或关闭 sessionId。

### 5.3 CLI 体验

```bash
export KC_SERVER_URL=http://127.0.0.1:7380   # 本机或共享 Server
export KC_AS=agent:consumer
kc catalog show --catalog kr://dw/catalog
kc catalog workspace resolve --workspace warehouse-agent > pin.json
kc knowledge search --workspace warehouse-agent --pin pin.json --query "GMV 指标"
kc knowledge read --workspace warehouse-agent --pin pin.json --object metric-gmv
kcfs mount --workspace warehouse-agent --pin pin.json --root ./project
```

`kc knowledge ... --home`、`kc catalog ... --home` 等公开旁路不存在；`--home` 只属于
`kc local` 宿主 bootstrap 与 `kc serve` 进程装配。`kcfs` 同样必须连接 Workspace File
Gateway，不能直接打开 Repository。组件测试可以进程内调用 Application Services，但该
测试接缝不是产品 transport。

`search --catalog` 由客户端读取 Catalog 配置的 `discoveryWorkspaceId`，按普通
Workspace 解析后再调用同一 Knowledge Search；不是 Catalog Server 搜索 Aspect。
`search --workspace` / `read --workspace` 是命名 Workspace 的便捷形式，客户端在
命令开始时隐式 Open 一次。目标远程客户端的 `--workspace-file` 只提交临时配方
用于解析，不在服务端创建 Workspace。跨命令复现保存不含授权能力的 `pin.json`，
并保留命名 Workspace revision 或同一配方；再次使用时以当前身份为同一 PinID
重新校验后直接访问。

`kcfs mount` 只接受 WorkspaceDefinition + ResolvedWorkspace，不接受 Catalog scope：
Catalog 范围搜索是发现入口，挂载前必须显式选择或创建 Workspace，避免把整个
Catalog 隐式落到本机。普通成员仓可以用文件、`rg` 等工具；只有结构化
`search/read/relations` 会进入 Knowledge Plane。

---

## 6. MountController 与 kcfs

### 6.1 边界

MountController 是 KC Client 的应用层编排；`kcfs` 是宿主投影进程：

```text
CatalogClient.ResolveWorkspace / ResolveDefinition
  → ResolvedWorkspace
  → Workspace File Gateway
  → immutable workspacefs.Plan
  → kcfs mount
```

`workspacefs.Plan` 至少固定：

- WorkspaceID（可空）/ PinID；
- 本机 root；
- 每条 mount 的 Workspace path；
- Repository / commit；
- immutable file reader。

MountController 持有当前凭证提供器、固定 basis 和远程 FileReader；`workspacefs/` 只看到固定
坐标和 FileReader 接口，不 import Catalog、Knowledge、Reader 或 Retrieval。
协议装配留在 Client 应用层。挂载普通仓不要求 `knowledge.Lookup` 成功。

### 6.2 远程读取模式

默认共享服务模式由 Workspace File Gateway 代理固定文件读取，这样可以：

- 不把服务机器凭证下发给客户端；
- 每次服务端 fetch 认证请求并校验当前 Repository 授权；
- 记录 semantic action `file.read` 的访问证据；
- 保持所有 bytes 绑定同一 commit。

受控环境可以支持 direct-authority 模式，但必须使用短期、最小范围凭证，并得到与代理模式相同的审计和 pin 保证。

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

`kcfs` 首版只支持 Linux FUSE。macOS/Windows 或无 FUSE 环境使用：

- KnowledgeClient / Workspace File Gateway 按需读取；或
- 未来基于 typed streaming API 的显式物化工具。

不提供让 CLI 直开 Server Home 并写宿主路径的 checkout 旁路。任何物化都必须使用相同 ResolvedWorkspace/PinID，不允许出现第二套 latest 语义。

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
  → connector.Preview(FULL scope)
  → ChangeSet(PUT/REMOVE)
  → Writer API COMMIT
  → Snapshot advanced event
  → ProjectionMaintainer
```

### 7.1 Writer API

逻辑接口：

```text
POST /writer/v1/repositories/{repository}/commits
POST /writer/v1/repositories/{repository}/proposals
GET  /writer/v1/repositories/{repository}/head?ref=...
GET  /writer/v1/receipts/{commandId}
```

`writer ingest` 是 Client 侧的确定性文件→ChangeSet 预处理，不是另一个写面。Client 先经上述 typed HEAD 路由获取 target ref 的 base commit，然后只读调用方指定的本地输入目录生成 ChangeSet；它不打开 Server Home。最终变更仍必须发到 `commits` route。

请求必须包含：

- `commandId`；
- `expectedTargetCommit`；
- PUT/REMOVE ChangeSet；
- provenance；
- principal/onBehalfOf/request/trace 信息。

同一 `commandId` 异 digest 返回 `IDEMPOTENCY_CONFLICT`；目标 ref 已推进返回 `NON_FAST_FORWARD`。一次请求只能写一个 Repository。

### 7.2 Schema

Schema 是知识，Connector 第一次同步前由 Writer API 写入目标 Repository 的 `schema/*` 对象。Schema 草稿、源客户端、密码和调度配置属于 Provider 工程，不进入 Catalog Server。

### 7.3 投影事件

单进程可以继续使用 `Catalog.Hook` 的 AfterSnapshot 装配。服务化后需要持久化的 projection outbox/队列，至少包含：

```text
repository, fromCommit, toCommit, eventId, occurredAt
```

消费者必须幂等；事件只通知“某仓从哪到哪”，ProjectionMaintainer 自己计算对象变化和物理代际。Catalog/Writer 核心不能 import `index/`。

### 7.4 Governance、Admin 与 Operations

治理与运维不复用 Writer 或 Knowledge consumer route：

```text
/governance/v1  proposals / previews / validations / merge
/identity/v1    whoami
/admin/v1       grants
/operations/v1  projections / hooks / gates / access / retrieval / refine / training / traces / hitmap / feedback
```

Operations 中的 retrieval/refine 是非 Canonical 证据查询面，不执行检索。Knowledge Server 执行
SEARCH/RELATION/RERANK 后先写 access，再写 retrieval/refine 原始证据；训练接口只重建带标签强度的
派生样本，不能把模型输出反写为知识或监督真值。

本机 `init`、Store 配置、Catalog/Repository authority attach 只属于 `kc local`，永不暴露为 HTTP。授权规则使用稳定 semantic action，不使用 CLI 命令字符串。

---

## 8. 身份、权限与秘密

### 8.1 身份

边界服务从 OIDC、Gitea 或部署 IdP 验证 token，注入稳定 principal：

```text
principal   = 实际执行主体
onBehalfOf  = 可选的被代理用户
```

Agent 代理用户时不能把用户冒充成 principal。

认证发生在每个远程请求入口。启用认证后，`principal` 必须完全来自 Authenticator，
服务拒绝请求体或 `X-Kc-As` 自报身份。`onBehalfOf` 只有在 IdP 的委托声明、token
exchange 或可信反向代理签名已被认证器验证后才能注入；普通客户端 header 不能作为
委托证据。当前 Gitea 认证器不提供委托声明，因此认证模式下拒绝客户端自报
`onBehalfOf`。本机单用户 facade 保留显式 `--as/--on-behalf-of`，但不能被描述为生产认证。

### 8.2 授权

默认安全边界是 Repository：

```text
principal × action × repository → allow | deny
```

- Workspace 配方不发权；
- Catalog 与 Knowledge 共用一份 `PolicyEvaluator` 合同和 action 词表，但在各自
  边界独立执行，不互相代判；
- Resolve、READ、SEARCH、VFS fetch 分别按当前权限求值；
- ResolvedWorkspace/PinID 不冻结授权，也不是 bearer capability；
- SEARCH 可在诚实声明 `partial` 的前提下跳过无权成员；
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

---

## 9. 一致性与失败语义

### 9.1 一致性单位

- Catalog registry 修改：Catalog 自身 CAS/revision；
- Repository 写入：单仓 ref CAS；
- 消费任务：一个 ResolvedWorkspace/PinID；
- Search page：query + SearchView + projection；
- VFS mount 生命周期：一个不可变 Plan/PinID；
- 跨 Repository：没有事务。

### 9.2 错误信封

服务统一返回：

```json
{"error":{"code":"VERSION_UNRESOLVED","message":"..."}}
```

关键映射：

| 情况 | code |
|---|---|
| 请求形状或非法查询 | `USAGE_INVALID` |
| 未认证 | `UNAUTHENTICATED` |
| 当前身份无权 | `FORBIDDEN` |
| object/digest/ResolvedWorkspace 前置条件不符 | `PRECONDITION_FAILED` |
| selector/commit/knowledge ref 无法解析 | 对应 `*_UNRESOLVED` |
| 写 ref 已前进 | `NON_FAST_FORWARD` |
| 相同 commandId 不同内容 | `IDEMPOTENCY_CONFLICT` |
| 瞬时网络、Store、provider 故障 | `TEMPORARY_UNAVAILABLE` |

索引延迟不是“知识不存在”。Knowledge Server 必须通过 completeness/claims 或明确错误暴露。

---

## 10. 可观测性

本节只声明服务边界需要携带的业务关联信息；运行 metric/log/distributed trace、传播、健康、SLI/SLO 与 Conformance 统一见 [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md)。知识访问证据仍见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

每个服务接受并透传：

```text
requestId, traceId, spanId, parentSpanId,
principal, onBehalfOf
```

消费访问证据至少绑定：

- Catalog/Workspace（可空）/PinID；
- Repository/commit；
- object/Address 或 VFS path；
- action、结果、耗时；
- Search completeness 和 provider claims。

服务日志、访问账和 projection hitmap 都是过程证据，不写回 Canonical。直接绕过服务读取 Git clone 或索引时，平台不能声称拥有逐条访问审计。

---

## 11. 部署拓扑

Server 是知识系统唯一运行边界，不以多方共享为前提。单机部署仍启动 KC Server；KC Client、Connector 与 `kcfs` 经 loopback typed API 调用它。单机与共享部署只替换认证和 Store/Retrieval adapter，不能改写 ResolvedWorkspace、SearchResult、Writer、授权、证据或索引语义。

```text
单机：Client/Connector/kcfs → 127.0.0.1 KC Server → 本机 Store / Retrieval provider
共享：Client/Connector/kcfs → 远程 KC Server    → 部署 Store / Retrieval provider
```

### 11.1 第一阶段：模块化单体

```text
kc-server
├── /catalog/v1
├── /workspace-files/v1
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
- 每个边界服务验证请求凭证；Catalog 校验 ResolvedWorkspace，数据面再对具体 action 求值；
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
7. **为远程读取引入 WorkspaceSession。** 固定数据已经由 ResolvedWorkspace 表达；认证与撤权应逐请求处理，额外 session 只会制造状态、续租和可用性问题。
8. **Connector 写 Catalog 或 OpenSearch。** Connector 只生成 ChangeSet，Writer 推 Snapshot，投影随后派生。
9. **FUSE 写回自动 COMMIT。** 首版 VFS 只读；知识写入必须显式走 Writer。

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
4. Knowledge 消费请求固定一个 ResolvedWorkspace/PinID；命令中途不重新 Resolve。
5. 临时 `.kc-workspace.yaml` 可被 resolve，但不会隐式写入 Catalog Registry。
6. token 刷新不改变 PinID；每个请求和 Pin 重放都执行当前认证、授权。
7. SEARCH 只处理可由 Knowledge Reader 解释的成员，并诚实报告其它成员的 coverage claim。
8. SEARCH 的公开命中全部从相同 basis Canonical hydrate。
9. OpenSearch 故障或延迟不会被报告成“知识不存在”。
10. Workspace 配方不扩大 Repository 权限。
11. VFS 生命周期内 selector 前进不改变已挂载 bytes；撤权阻止新 fetch，但不宣称撤回已交付 bytes。
12. `workspacefs/` 只消费 Plan，不 import Catalog/Knowledge/Retrieval。
13. Binding resolve 以完整 Address 为目标，不以裸 ObjectID 猜测单元。
14. Connector 不写 Catalog、Git 文件或 OpenSearch，只提交 ChangeSet。
15. 一次 Writer 请求只有一个目标 Repository，并保留 CAS/幂等语义。

这些不变量应同时进入 API contract tests、`internal/arch` 和 Linux/FUSE E2E；设计完成不以“接口能返回 200”为标准。
