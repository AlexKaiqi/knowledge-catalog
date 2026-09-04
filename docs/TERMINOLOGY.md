# Knowledge Catalog 术语表

日期：2026-08-27

状态：规范。公开文档、CLI 帮助、JSON 合同和 Go 导出注释使用这里的名称。

## Goal

固定 Knowledge Catalog 的公开名词，使文档、CLI 帮助、JSON 合同和 Go 导出注释使用同一套名称，避免 Catalog / Repository / Workspace / Server 被当成同义词。

## Non-Goals

- 不定义协议字段、错误码或命令表（见各包 README 与公开 API）。
- 不按部署进程名反向改写 ⓪–③ 分层（见 `LAYERS.md`）。
- 不把 `repo`、SDK 模块名提升为第二种领域对象。

## 硬性约束 / Invariants

- 公开文字使用全称；短名只用于 flag、变量和路径（本文表格）。
- Workspace 公开坐标是 `WorkspaceDefinition` / `ResolvedWorkspace` / `SearchView`，不引入 WorkspaceSession（`WS-01`，`ARCHITECTURE_INVARIANTS.md`）。
- 禁止的别名见本文 §4；发现同义复述时改用这里的规范名称，不另造词。

## 选定方案 / 被否决方案

- 选定：一张规范名称表，Go 类型名作为实现锚点写在定义里。
- 否决：用 Catalog 指整套知识系统；用 CatalogClient/KnowledgeClient 当两个产品；按 Server 部署名改协议包名；用 Loom 当公开产品名。

## 接口契约 / 状态机

权威就是本文表格。类型锚点：`catalog.WorkspaceDefinition`、`catalog.WorkspaceRecipe`、`snapshot` Ref/commit。CLI/HTTP 用词必须与本表一致。

## 1. 产品与服务

| 规范名称 | 含义 | 不要混用 |
|---|---|---|
| Knowledge Catalog | 整套产品与协议 | 不用 Catalog 单独指整套知识系统 |
| Catalog | 一间共享组合空间及其登记状态 | 不是 Repository、文件仓或知识索引 |
| Catalog Plane | Repository 发现、Workspace 组合、selector 解析和文件投影所在逻辑平面 | 不理解 Aspect、Schema、Binding |
| Knowledge Plane | Schema、Entity、Aspect、Relation、READ、SEARCH 和 Writer 所在逻辑平面 | 不拥有 Catalog 生命周期；不存在无界公开 Knowledge LIST |
| Catalog Server | Catalog Plane 的共享控制服务 | `catalog/` 是协议包，不等于部署进程 |
| Knowledge Server | 固定 Workspace basis 上的结构化知识消费服务 | OpenSearch 只是其内部 provider |
| Workspace File Gateway | 按固定 ResolvedWorkspace 提供 path/tree/blob 的远程数据端口 | 不叫 Workspace Files API；不解释知识 |
| Writer API | Knowledge Repository 的 PUT/REMOVE、COMMIT/PROPOSAL 写入口 | Connector 不直接写 Git 或 OpenSearch |
| Governance API | Proposal、Preview、Validation、Gate 与 Merge 的治理入口 | 不是 Writer，也不是 Hook 执行器 |
| Admin API | 服务端授权策略的管理入口 | 不包含本机 init、Store 配置或 authority attach |
| Operations API | Projection、Hook、Gate 配置和审计/观测入口 | 不属于普通知识消费面 |
| KC Client | 对外客户端产品 | CatalogClient、KnowledgeClient 是其 SDK 模块，不是两个产品 |

`Server` 表示逻辑服务边界；这些边界可以先同进程部署。Go 包名仍使用
`catalog`、`knowledge/reader`、`retrieval` 等协议名称，不能按部署名反向改写分层。
Knowledge Reader Service 是 Knowledge Server 内部的②装配组件：它把 Catalog 交付的
Snapshot 成员包装为知识读能力，并提供 exact-basis ReadMany；不是第三个对外 Server，也不持有 Knowledge object cache。

## 2. Repository 与 Workspace

| 规范名称 | 精确定义 |
|---|---|
| Repository | Snapshot authority 和治理边界。正式文字使用全称；`repo` 只用于 CLI flag、短变量和路径名。 |
| Knowledge Repository | 内容通过 Writer 发布并遵守 Address/Schema/Aspect 合同；消费时由 Knowledge Reader 在固定 commit 上解释的 Repository。它不是 Adapter 实现的接口标记。 |
| System Repository | 部署内置且对已认证用户可读的保留 Knowledge Repository（参考 ID `kr://kc/system`）；发布 Meta Schema 和核心协议 Schema，不是业务 Workspace 的隐式成员。 |
| Source Profile / 源说明 | 每个 Knowledge Repository 最多一个自描述对象，由 `catalog show` 的 `repositories` 读出。不是 git README、不是对象 LIST、不是 Catalog 登记字段。 |
| Meta Schema | 约束 Domain Schema 文档自身的协议合同；由二进制内置信任根校验，并在 System Repository 中发布同一内容。 |
| Domain Schema | 接入方定义、随目标 Knowledge Repository 版本化的 `schema/*` 对象；通过 `schema_ref` 约束实例 Address/value。 |
| Plain Repository | 未按知识发布合同维护内容的普通 Repository；仍可被 Catalog 组合和 VFS 挂载。 |
| WorkspaceDefinition | 可变配方：成员 Repository、selector、路径布局和 revision。Go 协议类型是 `catalog.WorkspaceDefinition`。 |
| WorkspaceRecipe | `.kc-workspace.yaml` 的便携序列化形态。它是文件 DTO，不是第二种 Workspace 领域对象。Go 类型是 `catalog.WorkspaceRecipe`。 |
| ResolvedWorkspace | 一次 Resolve 的不可变结果：`{Repository → commit}`、WorkspaceID、revision、PinID。Go 协议类型是 `catalog.ResolvedWorkspace`。 |
| Workspace pin | `ResolvedWorkspace` 的用户侧简称。文件名使用 `pin.json`，标识使用 `PinID`；不要再造 `WorkspaceView` 或 `ResolvedView`。 |
| SearchView | 一次 SEARCH 实际观察到的 Snapshot/Binding basis。它属于检索结果，不等于 Workspace；Workspace 范围由请求时的 ResolvedWorkspace 编译，不写进索引文档。 |
| 固定元信息 | 知识对象的协议坐标，不是业务正文：`repository`、`object_id`、`basis`、`schema_ref`。检索索引携带它们供 typed filter。Workspace、Pin、allow 规则、当前 principal 以及未选定的仓级可见性分类都不是固定元信息。 |
| 交付链 | hydrate Canonical 之后、编码返回之前按固定顺序挂接的平台规则。输入是知识 ID，输出是调用方可见内容。公开类型 `delivery.Chain`。当前选定仅仓读权屏蔽；不是 Hook，不是检索代数，也不是新的协议层。细节由 `PERMISSIONS.md` 拥有。 |
| Preview | ControlPlane 中 Proposal + Workspace overlay 的治理 basis。它只用于 validate/gate/merge。 |
| TaskContext | 客户端宿主私有的任务上下文：身份、WorkspaceDefinition、ResolvedWorkspace 与 mount 生命周期。它不是服务端 Session，也不写入用户工作目录。 |
| Knowledge Set（知识集） | 面向消费者的产品名称：管理员命名或客户端临时形成的一组知识源。后端可用 WorkspaceDefinition 表达，但用户不必先理解或新建 Workspace。 |
| Semantic File View | 在固定 Repository commit 上由 Canonical Address 组装的只读 YAML/Markdown 消费投影；保留 `_kc` 坐标，可丢弃重建，不是 Canonical 或写入口。 |

`knowledge/reader.WorkspacePin` 是 Knowledge Plane 内部从
`catalog.ResolvedWorkspace` 投影出的读取 basis，不是另一个可持久化协议对象，也不
替代 `ResolvedWorkspace` 这个公开名称。

## 3. 动作

| 动作 | 规范含义 |
|---|---|
| attach/open Repository | 让本机 Store Directory 能打开一个 Snapshot authority。当前 CLI 是 `kc local repository attach` 的 ⓪ 部分。 |
| register Repository | 让一间 Catalog 承认已经可打开的 Repository。当前 CLI 是 `kc catalog repository register`；`kc local repository attach` 还会注册到默认 Catalog。 |
| resolve Workspace | 把 WorkspaceDefinition 的 selector 各解析一次，产生 ResolvedWorkspace。 |
| replay pin | 用 `--pin <ResolvedWorkspace.json>` 重放同一组坐标，同时按当前权限重新求值。 |
| checkout Workspace | 显式物化为普通目录/工作树。当前无公开 CLI；未来必须经 typed streaming API，不得直开 Server Home。 |
| mount Workspace | 把固定 pin 投影为宿主只读文件系统。只使用 `kcfs mount`；`mount` 不再表示接入 Repository。 |
| browse knowledge | 有界发现：可见 Catalog、知识集、源说明，以及单仓 Schema/类型分页。不是对象 LIST；空查询或 `*` 也不是 BROWSE。 |
| search knowledge | 按 Schema AccessHints 检索并在同一 basis 回读 Canonical。调用方信封是否含全文走权限交付链首段（`PERMISSIONS.md`），检索本身不裁剪。它不是文件 contains；普通文件使用 Workspace File Gateway / `kcfs` + `rg`。 |
| scan Snapshot | Provider/维护方在固定 commit 上为重建、迁移、导出或验收顺序读取全部知识。公开消费面不提供该动作。 |

## 4. 禁止的别名

以下名称不进入新的公开合同：

- `WorkspaceView`、`ResolvedView`、`viewRef`、`ViewLease`：统一为
  `ResolvedWorkspace`/pin；
- `WorkspaceSession`、`sessionId`：不进入 Workspace 或认证公开合同。传输连接、SDK
  任务对象、FUSE 进程可以在实现内称 session，但不能成为身份、Pin 或续租资源；
- 裸 `View`：必须写明 `SearchView`、Preview 或 Workspace pin 中的哪一种；
- `Workspace Files API`：统一为 `Workspace File Gateway`；
- `kc mount` 表示 Repository 接入：Repository 使用 `kc local repository attach`，宿主挂载使用
  `kcfs mount`；
- `Repo` 作为正式领域名称：公开说明使用 `Repository`，仅保留既有 `--repo`、
  `repo-add` 和实现内短变量。
- `Loom` 作为公开产品名：产品是 Knowledge Catalog。不要在协议、CLI 帮助、设计标题或新的公开 HTTP/API 路径使用 Loom。
- 无界 `LIST` 作为知识发现或 SEARCH 降级：自然语言发现使用 SEARCH；面向首次使用的
  DISCOVER/BROWSE 必须有界、分页、声明 basis/coverage，内容是 Catalog/知识集/源说明
  与 Schema namespace，不是对象实例目录；维护扫描只使用 `ScanSnapshotPage`，文件遍历使用按目录分页的 Gateway。

## 5. 一条完整链路

```text
WorkspaceDefinition
  -- ResolveWorkspace --> ResolvedWorkspace (pin.json / PinID)
  -- Knowledge SEARCH --> SearchResult.SearchView
  -- host projection --> workspacefs.Plan --> kcfs mount
```

Provider 侧是另一条链路：

```text
Source → Connector → ChangeSet → Writer API → Knowledge Repository commit
       → ProjectionMaintainer → OpenSearch projection
```
