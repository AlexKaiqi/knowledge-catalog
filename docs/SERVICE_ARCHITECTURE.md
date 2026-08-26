# Catalog / Knowledge 服务与客户端架构

日期：2026-08-26

状态：目标设计；现有实现映射与增量落地路径见第 13 节

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

Catalog Plane 对内容格式宽容；Knowledge Plane 对结构化读取和发布严格。约束发生在使用 Knowledge capability 时，不发生在 Repository 注册、Workspace 组合或 VFS 挂载时。

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
| Workspace File Gateway | 如何按固定 WorkspaceSession 提供不解释知识的 path/tree/blob？ |
| kcfs | 如何把应用层准备好的固定文件计划投影成本机只读目录？ |
| Writer API | 接入方如何以 PUT/REMOVE ChangeSet 更新唯一目标 Repository？ |

Catalog Server、Knowledge Server、Workspace File Gateway 和 Writer API 初期可以在同一进程部署，但 API、包依赖和状态所有权必须保持分离。各边界共享同一身份与 PolicyEvaluator 合同，并分别执行授权，不能各自发明权限规则。

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

Catalog 核心只认识 Repository identity、selector、commit 和 Workspace。Snapshot endpoint、驱动配置和机器凭证属于服务装配的 Store Directory，不进入 `catalog/` 协议类型。Aspect、Schema、检索字段和 Knowledge capability 由 Knowledge Plane 在实际使用时判断，不出现在 Catalog DTO 中。

### 2.2 知识发现

由 Knowledge Server 提供：

- `DESCRIBE_SCHEMA`：发现可用 Entity/Aspect/Relation 结构；
- `SEARCH`：按 Schema AccessHints 检索对象；
- `READ` / `READ_ADDRESS`：读取并拼装 Canonical；
- `RELATIONS`：按固定 basis 查询关系；
- `GET_PROVENANCE` / `LOG`：读取来源和对象历史；
- `RESOLVE_BINDING`：返回稳定访问声明，不调用墙外 runtime。

OpenSearch 不是知识发现 API。它是 Knowledge Server 内部的 Retrieval provider，只返回无正文的 `CandidateRef`。

### 2.3 Catalog、Workspace 与本地 Session

三个概念的生命周期不同：

| 概念 | 位置 | 生命周期 | 内容 |
|---|---|---|---|
| Catalog | 服务端共享 | 组织级长期 | 被承认的 Repository、WorkspaceDefinition、治理历史 |
| WorkspaceDefinition | Catalog Server 或客户端本地文件 | 可版本化、可复用 | Repository 子集、selector、可选 mount path/subPath |
| ResolvedWorkspace | 请求或客户端 | 一次任务 | 固定 `{repository → commit}` 和 PinID；PinID 绑定配方路径布局 |
| WorkspaceSession | 服务端与客户端 | 短期、可续租 | 当前身份访问某个 ResolvedWorkspace 的在线会话 |
| VFS mount | 客户端本机 | 一个进程 | ResolvedWorkspace/PinID 的只读宿主投影 |

因此 Workspace 不是本地文件系统，也不是另一个 Repository。它是“这类任务需要同时组合哪些 Snapshot Repository”的命名配方。本地只持有配方文件、这次 Resolve 后的 Session 和可选 mount。

Workspace 可以有三种来源，但进入消费面后使用同一语义：

1. 组织在 Catalog Server 发布的共享 Workspace；
2. 客户端从 `.kc-workspace.yaml` 或显式参数形成的本地/临时配方。

便携 `.kc-workspace.yaml` 是配方载体，不是 Repository 或本地状态权威；它可以被发布为服务端 Workspace，也可以只用于生成一次临时 ResolvedWorkspace。本机 overlay、目标目录和 FUSE 生命周期不写回共享配方。远程个人 Workspace 只有在 owner、visibility、命名空间和生命周期协议完整后再增加，不作为 V1 前提。

### 2.4 Workspace 不要求 Knowledge capability

Workspace 可以混合普通 Repository 与 Knowledge Repository：

```text
ResolvedWorkspace
├── plain code/docs repo       mount / checkout / rg
├── knowledge repo A           mount + READ/SEARCH
└── knowledge repo B           mount + READ/SEARCH
```

Repository 注册、Workspace Resolve 和 Workspace File Gateway 都不要求 `object_id`、Aspect 或 Schema。Knowledge Server 在固定 commit 上逐成员取得 `knowledge.Repository` capability；不具备 capability 的成员不进入结构化 READ/SEARCH，并在有 completeness 信封的结果中如实报告 coverage。用户要搜索普通文件时使用 VFS/checkout 上的 `rg`，不能让 Knowledge SEARCH 退化为整包 JSON 或文件 contains。

知识规范是发布与结构化访问合同：接入方声称某 Repository 是知识提供方时，必须遵守 Address、Schema、Aspect、Relation、provenance、PUT/REMOVE 和 Writer CAS；用户在自己的宿主 Workspace 中开发普通文件不受这些格式约束。用户决定把成果发布为知识时，再通过 Adapter/Connector 翻译为 ChangeSet 并进入 Writer。

### 2.5 Catalog 范围的知识搜索

Catalog Core 不理解知识，但产品仍应支持“在我可发现的整个 Catalog 中搜索”。该能力分两步完成：

```text
Catalog
  → 找到该 Catalog 配置的 discoveryWorkspaceId
  → 按普通 WorkspaceDefinition 执行 ResolveWorkspace
  → 取得当前身份可参与 SEARCH 的 ResolvedWorkspace
  → Knowledge Server SEARCH(WorkspaceSession)
```

Catalog 范围搜索不新增第二种组合代数。`discoveryWorkspaceId` 指向一条普通、管理员维护的 WorkspaceDefinition；`kc search --catalog` 只是“解析这条指定 Workspace，再调用 Knowledge SEARCH”的客户端语法糖。Catalog Server 仍然只做 Repository 选择和 Snapshot 坐标解析；真正的 capability、Schema 和 Aspect 查询在 Knowledge Server。

不能简单把“所有已注册 Repository 的默认分支”自动纳入搜索：注册表示 Catalog 承认该仓，不等于仓已发布或允许组织发现。管理员通过 discovery Workspace 显式选择 Repository 和 published selector。无权成员按 SEARCH 现有规则省略并返回 `partial` claim；普通、无 Knowledge capability 的成员同样进入 coverage claim，而不会阻止挂载。

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
- 要求 Workspace 的所有成员都实现 Knowledge capability；
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

### 3.4 ResolvedWorkspace 与 WorkspaceSession

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

远程服务另外打开短期 `WorkspaceSession`：

```text
WorkspaceSession
  sessionId: opaque id
  pinId: ResolvedWorkspace.PinID
  subject: authenticated principal/onBehalfOf fingerprint
  expiresAt: timestamp
```

Knowledge 和 Workspace File 请求使用短小、opaque 的 `sessionId`。Session 必须满足：

- 服务端关联完整 WorkspaceDefinition、ResolvedWorkspace、Catalog 和创建身份；
- 有有限 TTL，不能被客户端篡改或换绑 pin；
- 只是访问会话，不是永久 bearer capability；
- 每次使用或续租仍按当前身份重新求值权限；
- 续租只延长同一 PinID，绝不重新解析 selector；
- 不能替代公开的 ResolvedWorkspace 和可复核 PinID；
- pin 重放会创建新的 WorkspaceSession，而不是复活旧身份会话。

每次数据面请求必须同时携带正常身份凭证和 `sessionId`；只有 `sessionId` 不能访问
数据。Catalog Server 作为 session 状态所有者提供内部 `WorkspaceSessionVerifier` 合同：

```go
VerifyWorkspaceSession(ctx, sessionID, authenticatedSubject) (WorkspaceDefinition, ResolvedWorkspace, error)
```

它只验证 session 存活、身份绑定和 PinID 完整性，不替 Knowledge Server 或
Workspace File Gateway 决定具体 READ/SEARCH/VFS action。目标服务取得固定
ResolvedWorkspace 后，再用共享 PolicyEvaluator 对实际 Repository 和 action 求值。
这样拆服务时既不复制 session 状态，也不把数据面权限塞回 Catalog Core。

### 3.5 Catalog API 资源

目标 API 采用资源化接口；现有 `POST /v1/<verb>` 继续作为兼容 facade，而不是长期服务合同。

```text
GET    /catalog/v1/catalogs
GET    /catalog/v1/catalogs/{catalog}/repositories
POST   /catalog/v1/catalogs/{catalog}/repositories
GET    /catalog/v1/catalogs/{catalog}/workspaces
POST   /catalog/v1/catalogs/{catalog}/workspaces
GET    /catalog/v1/catalogs/{catalog}/workspaces/{workspace}
POST   /catalog/v1/catalogs/{catalog}/workspaces/{workspace}:resolve
POST   /catalog/v1/catalogs/{catalog}/workspaces/{workspace}:check
POST   /catalog/v1/catalogs/{catalog}/workspaces:resolve
POST   /catalog/v1/catalogs/{catalog}/workspace-sessions
POST   /catalog/v1/workspace-sessions/{sessionId}:renew
DELETE /catalog/v1/workspace-sessions/{sessionId}
```

管理写请求继续使用 revision/CAS 和 `requestId`，不能退化成最后写者覆盖。

两个 resolve 接口都返回不含授权能力的 ResolvedWorkspace。集合级
`workspaces:resolve` 接受客户端提交的临时 WorkspaceDefinition，只验证、授权和
解析，不调用 `DefineWorkspace`，不写 Catalog Registry。创建 WorkspaceSession 时
提交命名 Workspace 引用或临时 WorkspaceDefinition，并携带要重放的
ResolvedWorkspace；服务必须重新校验配方成员、commit、PinID 与当前权限。

### 3.6 Workspace File Gateway

远程 `kcfs` 需要读取固定 Snapshot 的 path/tree/blob。这属于 Catalog Plane 的宿主数据接缝，但不属于 `catalog/` 核心或 Knowledge Server。逻辑上用独立 Workspace File Gateway 表达；模块化单体阶段可以与 Catalog Server 同进程部署：

```text
GET /workspace-files/v1/workspace-sessions/{sessionId}/mounts
GET /workspace-files/v1/workspace-sessions/{sessionId}/tree?path=<workspace-path>
GET /workspace-files/v1/workspace-sessions/{sessionId}/file?path=<workspace-path>
```

这些接口复用 Workspace 路由与 pin 语义，只返回路径、Repository、commit、digest、encoding 和 bytes。它们不是新的 Store，也不改变 `snapshot.TreeStore` 接口。

---

## 4. Knowledge Server

### 4.1 责任

Knowledge Server 是结构感知的消费数据面。它接受同一身份取得的 `sessionId`，
验证 WorkspaceSession 后在固定 ResolvedWorkspace basis 上装配：

```text
Knowledge Reader Service
  └── Canonical Repository wrapper / ReadMany / bounded object cache
retrieval planner / executor
Retriever providers
Snapshot Repository capabilities
observability recorder
```

它拥有业务含义上的知识读取，不拥有 Repository 或 Workspace 的生命周期。
Catalog Server 交付的仍是 `snapshot.Store`；应用装配根显式调用
`knowledge/reader.Reader.Lookup` 跨入②。这个接缝统一解释 `object_id`、Aspect 和磁盘单元，
不要求 Catalog 或 Snapshot Registry 暴露知识方法。

### 4.2 消费 API

推荐的目标接口：

```text
POST /knowledge/v1/workspace-sessions/{sessionId}/search
GET  /knowledge/v1/workspace-sessions/{sessionId}/objects/{objectId}
POST /knowledge/v1/workspace-sessions/{sessionId}/addresses:read
POST /knowledge/v1/workspace-sessions/{sessionId}/relations:query
GET  /knowledge/v1/workspace-sessions/{sessionId}/objects/{objectId}/provenance
GET  /knowledge/v1/workspace-sessions/{sessionId}/objects/{objectId}/log
GET  /knowledge/v1/workspace-sessions/{sessionId}/schemas/{schemaId}
POST /knowledge/v1/workspace-sessions/{sessionId}/bindings:resolve
```

`bindings:resolve` 的请求目标是完整 Address，不是裸 ObjectID；Binding 属于一个
确定 Aspect/member 单元。消费者 API 只接受固定 WorkspaceSession，不让普通调用方
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
  → OpenSearch/SQLite 返回 CandidateRef
  → 校验 repository 与 basis
  → Knowledge Reader Service.ReadMany 在同一 commit 批量 hydrate
  → residual filter
  → SearchResult
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
  → OpenSearch/SQLite candidate search
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

基础 Knowledge Server 的 `RESOLVE_BINDING` 只返回固定声明。需要访问实时 State/Stream 时，由墙外 Materialization Runtime 取得 observation basis；不得把 cursor、watermark 或运行 generation 塞入 Catalog pin。

### 4.8 Canonical hydrate 缓存

知识访问的时间局部性发生在固定版本对象，而不是 Workspace 或 OpenSearch `_source`。缓存由
Knowledge Reader Service 统一拥有：

```text
CandidateRef[]
  → request/page ReadMany
  → bounded process LRU
  → optional distributed Cache port
  → Snapshot TreeStore / native Knowledge source
```

Canonical key 固定为 `(repository, commit, object_id)`。Workspace 不进入 key；同一 Repository
commit 可跨 Workspace 复用。commit 不可变，因此无需按 ref 前进主动失效；LRU/TTL 只负责容量
回收。AspectSelector 在完整对象命中后裁剪，不产生另一份 Canonical cache key。

Snapshot Adapter 可以缓存 HTTP connection、原始 commit tree、blob SHA/bytes 或数据库执行计划，
但不得拥有 `object_id → KnowledgeValue` 缓存语义。尤其不能由各 Adapter 分别缓存已解析
Aspect tree，否则不同介质会产生不同的内存、淘汰和观测行为。Redis 若引入，只是这一 Knowledge
Cache port 的可选 L2，不进入 Catalog、Schema、Workspace 或 Snapshot 接口。

Binding 声明可以按 declaration commit 缓存；墙外 runtime 返回的动态 observation 不得复用上述
key，必须由 Materialization Runtime 按资源版本、身份和独立 TTL 管理。OpenSearch 的 query/request
cache 只优化候选定位，不能替代 Canonical hydrate cache。

---

## 5. KC Client

### 5.1 对外只有一个客户端产品

调用者不应安装三个互不相关的 CLI。统一 KC Client 内部包含：

```text
KC Client
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

### 5.2 一次任务的 Session

客户端应提供显式 Session，避免每个调用重新 Resolve。打开命名 Workspace
或本地临时配方时，客户端先取得 `ResolvedWorkspace`，再以当前身份打开
`WorkspaceSession`：

```go
session, err := client.OpenWorkspace(ctx, catalogID, workspaceID)
defer session.Close()

hits, err := session.Knowledge.Search(ctx, request)
value, err := session.Knowledge.Read(ctx, objectID, selector)
mount, err := session.Mount(ctx, target)
```

也可以从本地文件或已经保存的 pin 打开：

```go
session, err := client.OpenDefinition(ctx, catalogID, workspaceDefinition)
session, err := client.OpenResolved(ctx, workspaceDefinition, resolvedWorkspace)
```

Session 持有固定 `ResolvedWorkspace` 和可续租 `WorkspaceSession`，不持有永久授权。
续租只能继续同一个 PinID，不能重新解析 selector；需要跟随分支时必须显式重新
Open Workspace，形成新的 ResolvedWorkspace。

### 5.3 CLI 体验

```bash
kc read --catalog kr://dw/catalog
kc resolve --workspace warehouse-agent > pin.json
kc search --workspace warehouse-agent --pin pin.json --query "GMV 指标"
kc read --workspace warehouse-agent --pin pin.json --object metric-gmv
kcfs mount --workspace warehouse-agent --pin pin.json --root ./project
```

`search --catalog` 由客户端读取 Catalog 配置的 `discoveryWorkspaceId`，按普通
Workspace 解析后再调用同一 Knowledge Search；不是 Catalog Server 搜索 Aspect。
`search --workspace` / `read --workspace` 是命名 Workspace 的便捷形式，客户端在
命令开始时隐式 Open 一次。目标远程客户端的 `--workspace-file` 只提交临时配方
用于解析，不在服务端创建 Workspace。跨命令复现保存不含授权能力的 `pin.json`，
并保留命名 Workspace revision 或同一配方；再次使用时以当前身份为同一 PinID
打开新 WorkspaceSession。

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
  → Open WorkspaceSession
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

MountController 持有 WorkspaceSession、续租器和远程 FileReader；`workspacefs/` 只看到固定
坐标和 FileReader 接口，不 import Catalog、Knowledge、Reader 或 Retrieval。
协议装配留在 Client 应用层。挂载普通仓不要求 `knowledge.Lookup` 成功。

### 6.2 远程读取模式

默认共享服务模式由 Workspace File Gateway 代理固定文件读取，这样可以：

- 不把服务机器凭证下发给客户端；
- 每次服务端 fetch 校验 WorkspaceSession 和当前 Repository 授权；
- 记录 `vfs-read` 访问证据；
- 保持所有 bytes 绑定同一 commit。

受控环境可以支持 direct-authority 模式，但必须使用短期、最小范围凭证，并得到与代理模式相同的审计和 pin 保证。

### 6.3 WorkspaceSession 与撤权

MountController 在 WorkspaceSession 到期前续租。续租重新认证、重新求值当前权限，但只能续
同一 PinID；任何实现都不得借续租跟随分支。续租或授权失败后：

- 停止新的远程 fetch，并把 mount 标记为 degraded/unauthorized；
- 未缓存路径的后续读取返回明确 I/O/授权错误；
- 不自动切换到新 commit，也不静默返回空文件；
- 可由部署策略选择显式卸载，但不能声称已收回进程、内核页缓存或用户已经复制的 bytes。

授权的可执行边界是打开 WorkspaceSession、续租和服务端 fetch。FUSE 内核缓存、进程内缓存和
客户端已收到的内容可能在撤权后继续可见；这不是服务能够倒转的事实。高敏 profile
可以缩短 session TTL、关闭持久缓存或加密缓存，但仍不能撤回已经交付的数据。

### 6.4 缓存

文件 bytes 由 `(repository, commit, path, digest)` 标识，可以安全做本地内容缓存。缓存不得：

- 按 selector 或 Workspace 名作为内容键；
- 在权限撤销后继续建立新 mount；
- 把缓存目录当 Canonical；
- 允许向缓存写入后自动回传 Repository。

缓存命中不代表当前仍有远程读取权。共享客户端若在 mount/session 之外复用缓存，必须
先重新授权；即便如此，平台也只能阻止受控接口继续交付，不能保证删除用户可直接
访问的旧缓存副本。

### 6.5 平台

`kcfs` 首版只支持 Linux FUSE。macOS/Windows 或无 FUSE 环境使用：

- `kc checkout --workspace` 的显式物化；或
- KnowledgeClient / Workspace File Gateway。

两者仍使用相同 ResolvedWorkspace/PinID，不允许出现第二套 latest 语义。

---

## 7. 接入写面

普通 Repository 仍由它自己的 authority 和工作流维护；Catalog Plane 不要求其中
的文件符合知识规范。只有声明提供 Knowledge capability 的 Repository，才通过下面
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
GET  /writer/v1/receipts/{commandId}
```

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

---

## 8. 身份、权限与秘密

### 8.1 身份

边界服务从 OIDC、Gitea 或部署 IdP 验证 token，注入稳定 principal：

```text
principal   = 实际执行主体
onBehalfOf  = 可选的被代理用户
```

Agent 代理用户时不能把用户冒充成 principal。

### 8.2 授权

默认安全边界是 Repository：

```text
principal × action × repository → allow | deny
```

- Workspace 配方不发权；
- Catalog 与 Knowledge 共用一份 `PolicyEvaluator` 合同和 action 词表，但在各自
  边界独立执行，不互相代判；
- Resolve、open/renew WorkspaceSession、READ、SEARCH、VFS fetch 分别按当前权限求值；
- ResolvedWorkspace/PinID 不冻结授权，WorkspaceSession 也不是永久 bearer capability；
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
- 在线授权会话：一个可续租但不能换 PinID 的 WorkspaceSession；
- Search page：query + SearchView + projection；
- VFS session：一个不可变 Plan/PinID；
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
| object/digest/ResolvedWorkspace/WorkspaceSession 前置条件不符 | `PRECONDITION_FAILED` |
| selector/commit/knowledge ref 无法解析 | 对应 `*_UNRESOLVED` |
| 写 ref 已前进 | `NON_FAST_FORWARD` |
| 相同 commandId 不同内容 | `IDEMPOTENCY_CONFLICT` |
| 瞬时网络、Store、provider 故障 | `TEMPORARY_UNAVAILABLE` |

索引延迟不是“知识不存在”。Knowledge Server 必须通过 completeness/claims 或明确错误暴露。

---

## 10. 可观测性

每个服务接受并透传：

```text
requestId, traceId, spanId, parentSpanId, sessionId,
principal, onBehalfOf
```

消费访问证据至少绑定：

- Catalog/Workspace（可空）/PinID/WorkspaceSession；
- Repository/commit；
- object/Address 或 VFS path；
- action、结果、耗时；
- Search completeness 和 provider claims。

服务日志、访问账和 projection hitmap 都是过程证据，不写回 Canonical。直接绕过服务读取 Git clone 或索引时，平台不能声称拥有逐条访问审计。

---

## 11. 部署拓扑

服务化是多方共享部署，不是协议成立的前提。离线或单机模式可以由 KC Client 在进程内装配 Catalog、Reader、Retrieval 和本地 Store；它必须产生与远程模式相同的 ResolvedWorkspace、SearchResult 和 mount Plan，不能另造一套语义。

### 11.1 第一阶段：模块化单体

```text
kc-server
├── /catalog/v1
├── /workspace-files/v1
├── /knowledge/v1
├── /writer/v1
├── Catalog Registry
├── WorkspaceSession Store
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
- Catalog Server 拥有 WorkspaceSession Store，只通过内部 WorkspaceSessionVerifier 交付已校验的 ResolvedWorkspace；
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
7. **WorkspaceSession 作为永久 bearer capability。** Pin 冻结数据，不冻结权限；session 有限期且持续按当前权限求值。
8. **Connector 写 Catalog 或 OpenSearch。** Connector 只生成 ChangeSet，Writer 推 Snapshot，投影随后派生。
9. **FUSE 写回自动 COMMIT。** 首版 VFS 只读；知识写入必须显式走 Writer。

---

## 13. 当前实现与目标差距

| 目标组件 | 当前基础 | 主要缺口 |
|---|---|---|
| Catalog Server | `catalog/`、`catalog.Registry`、`kc serve` | 资源化远程 API、独立 Store Directory 服务装配、临时配方 Resolve、WorkspaceSession |
| Workspace File Gateway | `workspacefs.Plan` 所需的 Store 读取能力 | 固定 WorkspaceSession 的 list/read 合同、session 校验、访问证据、远程 FileReader |
| Knowledge Server | `knowledge/reader` 的统一 Repository 包装/ReadMany/LRU、`retrieval/`、`index/`、HTTP verb facade | 独立 `/knowledge/v1` 合同、WorkspaceSession 复用、服务级 provider 路由、可选分布式 Cache port |
| Writer API | Writer + `POST /v1/commit|proposal` | 独立合同、跨进程幂等存储、生产认证/限流 |
| KC Client | `kc` CLI、DSH 插件 | 远程 SDK、统一 Session、服务发现与凭证管理 |
| MountController | `cli/workspacefs.go` | 远程 Workspace File Gateway Client、可重放 mount manifest、WorkspaceSession 续租/降级 |
| kcfs | `workspacefs/`、`cmd/kcfs/` | 远程 lazy tree、内容缓存、授权失败状态管理 |
| Projection | Catalog.Hook + OpenSearch/SQLite provider | durable outbox、worker lease、历史 basis 生命周期 |

现有 `kc serve` 是钉住一个本机 Home 的 CLI HTTP facade，适合开发和协议验证，不应直接宣称为完成态 Catalog/Knowledge Server。现有 `kcfs` 从本机 Home 装配计划；目标形态改为由 KC Client 从远程服务取得相同语义的固定计划。

---

## 14. 增量落地顺序

### P0：冻结服务合同

- 定义 Catalog、Workspace File Gateway、Knowledge、Writer 四个 API namespace；
- DTO 由各协议 namespace 所有；共享的只是 `kernel`/Snapshot 坐标等已有基础类型，
  不建立会反向混合 Catalog 与 Knowledge 的通用 DTO 桶；
- 固定 WorkspaceDefinition、ResolvedWorkspace、WorkspaceSession 的边界和序列化；
- 固定 Catalog/Knowledge 共用的 PolicyEvaluator action 合同；
- 增加同进程 contract tests，断言 API 与 Go surface 语义一致；
- 保留 `/v1/<verb>` 兼容入口。

### P1：远程 Catalog 与挂载

- 实现远程 CatalogClient；
- 实现命名 Workspace 与本地临时配方的 resolve；
- 实现 ResolvedWorkspace 的保存/重放和 WorkspaceSession 的 open/renew/close；
- 实现 Workspace File Gateway list/read 与逐 fetch 授权；
- MountController 从远程生成固定 Plan；
- Docker/Linux 验证 `kcfs mount` 在上游 ref 推进后仍保持原 bytes；
- 验证 WorkspaceSession 续租不换 PinID，撤权后阻止新 fetch 且不虚假承诺回收旧 bytes。

### P2：Knowledge Server

- 实现固定 WorkspaceSession 上的 Schema/READ/RELATIONS/PROVENANCE；
- 实现混合 Workspace 的 Knowledge capability 选择和 coverage claims；
- 接入 OpenSearch SEARCH；
- 验证 CandidateRef 同 basis 批量 hydrate，且 Canonical cache 不进入 Snapshot Adapter；
- continuation 绑定 query/SearchView/projection；
- 增加完整/部分、授权裁剪和索引滞后验收。

### P3：接入与投影可靠性

- Writer 跨进程幂等；
- durable projection outbox；
- worker retry/lease/dead-letter；
- projection generation 生命周期与重建；
- Provider Connector 通过正式 SDK/HTTP 做端到端验收。

### P4：生产化

- OIDC/企业 IdP；
- HA、限流、配额、审计导出；
- 多语言 Knowledge Client；
- 大树 lazy mount 与内容寻址缓存；
- SLO 和灾难恢复演练。

---

## 15. 验收不变量

1. Catalog API 的 DTO 不出现 `object_id`、Aspect、Binding 或 AccessSpec。
2. Workspace 可以混合普通 Repository 与 Knowledge Repository；挂载不要求 Knowledge capability。
3. Schema/Aspect 规范只约束 Knowledge 发布和结构化访问，不约束用户任意本地开发目录。
4. Knowledge 消费请求固定一个 ResolvedWorkspace/PinID；命令中途不重新 Resolve。
5. 临时 `.kc-workspace.yaml` 可被 resolve，但不会隐式写入 Catalog Registry。
6. WorkspaceSession 续租只重新认证和授权，不改变 PinID；Pin 重放仍执行当前授权。
7. SEARCH 只处理支持 Knowledge capability 的成员，并诚实报告其它成员的 coverage claim。
8. SEARCH 的公开命中全部从相同 basis Canonical hydrate。
9. OpenSearch 故障或延迟不会被报告成“知识不存在”。
10. Workspace 配方不扩大 Repository 权限。
11. VFS 生命周期内 selector 前进不改变已挂载 bytes；撤权阻止新 fetch，但不宣称撤回已交付 bytes。
12. `workspacefs/` 只消费 Plan，不 import Catalog/Knowledge/Retrieval。
13. Binding resolve 以完整 Address 为目标，不以裸 ObjectID 猜测单元。
14. Connector 不写 Catalog、Git 文件或 OpenSearch，只提交 ChangeSet。
15. 一次 Writer 请求只有一个目标 Repository，并保留 CAS/幂等语义。

这些不变量应同时进入 API contract tests、`internal/arch` 和 Linux/FUSE E2E；设计完成不以“接口能返回 200”为标准。
