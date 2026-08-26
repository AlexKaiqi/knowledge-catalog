# Knowledge Catalog 术语表

日期：2026-08-26

状态：规范。公开文档、CLI 帮助、JSON 合同和 Go 导出注释使用这里的名称。

## 1. 产品与服务

| 规范名称 | 含义 | 不要混用 |
|---|---|---|
| Knowledge Catalog | 整套产品与协议 | 不用 Catalog 单独指整套知识系统 |
| Catalog | 一间共享组合空间及其登记状态 | 不是 Repository、文件仓或知识索引 |
| Catalog Plane | Repository 发现、Workspace 组合、selector 解析和文件投影所在逻辑平面 | 不理解 Aspect、Schema、Binding |
| Knowledge Plane | Schema、Entity、Aspect、Relation、READ、SEARCH 和 Writer 所在逻辑平面 | 不拥有 Catalog 生命周期 |
| Catalog Server | Catalog Plane 的共享控制服务 | `catalog/` 是协议包，不等于部署进程 |
| Knowledge Server | 固定 Workspace basis 上的结构化知识消费服务 | OpenSearch 只是其内部 provider |
| Workspace File Gateway | 按固定 ResolvedWorkspace 提供 path/tree/blob 的远程数据端口 | 不叫 Workspace Files API；不解释知识 |
| Writer API | Knowledge Repository 的 PUT/REMOVE、COMMIT/PROPOSAL 写入口 | Connector 不直接写 Git 或 OpenSearch |
| KC Client | 对外客户端产品 | CatalogClient、KnowledgeClient 是其 SDK 模块，不是两个产品 |

`Server` 表示逻辑服务边界；这些边界可以先同进程部署。Go 包名仍使用
`catalog`、`knowledge/reader`、`retrieval` 等协议名称，不能按部署名反向改写分层。
Knowledge Reader Service 是 Knowledge Server 内部的②装配组件：它把 Catalog 交付的
Snapshot 成员包装为知识读能力，并统一拥有 ReadMany 与 Canonical cache；不是第三个对外 Server。

## 2. Repository 与 Workspace

| 规范名称 | 精确定义 |
|---|---|
| Repository | Snapshot authority 和治理边界。正式文字使用全称；`repo` 只用于 CLI flag、短变量和路径名。 |
| Knowledge Repository | 内容通过 Writer 发布并遵守 Address/Schema/Aspect 合同；消费时由 Knowledge Reader 在固定 commit 上解释的 Repository。它不是 Adapter 实现的接口标记。 |
| Plain Repository | 未按知识发布合同维护内容的普通 Repository；仍可被 Catalog 组合和 VFS 挂载。 |
| WorkspaceDefinition | 可变配方：成员 Repository、selector、路径布局和 revision。Go 协议类型是 `catalog.WorkspaceDefinition`。 |
| WorkspaceRecipe | `.kc-workspace.yaml` 的便携序列化形态。它是文件 DTO，不是第二种 Workspace 领域对象。Go 类型是 `catalog.WorkspaceRecipe`。 |
| ResolvedWorkspace | 一次 Resolve 的不可变结果：`{Repository → commit}`、WorkspaceID、revision、PinID。Go 协议类型是 `catalog.ResolvedWorkspace`。 |
| Workspace pin | `ResolvedWorkspace` 的用户侧简称。文件名使用 `pin.json`，标识使用 `PinID`；不要再造 `WorkspaceView` 或 `ResolvedView`。 |
| SearchView | 一次 SEARCH 实际观察到的 Snapshot/Binding basis。它属于检索结果，不等于 Workspace；Workspace 范围由请求时的 ResolvedWorkspace 编译，不写进索引文档。 |
| Preview | ControlPlane 中 Proposal + Workspace overlay 的治理 basis。它只用于 validate/gate/merge。 |

`knowledge/reader.WorkspacePin` 是 Knowledge Plane 内部从
`catalog.ResolvedWorkspace` 投影出的读取 basis，不是另一个可持久化协议对象，也不
替代 `ResolvedWorkspace` 这个公开名称。

## 3. 动作

| 动作 | 规范含义 |
|---|---|
| attach/open Repository | 让本机 Store Directory 能打开一个 Snapshot authority。当前 CLI 是 `kc repo-add` 的 ⓪ 部分。 |
| register Repository | 让一间 Catalog 承认已经可打开的 Repository。当前 CLI 是 `kc register`；`kc repo-add` 还会注册到默认 Catalog。 |
| resolve Workspace | 把 WorkspaceDefinition 的 selector 各解析一次，产生 ResolvedWorkspace。 |
| replay pin | 用 `--pin <ResolvedWorkspace.json>` 重放同一组坐标，同时按当前权限重新求值。 |
| checkout Workspace | 显式物化为普通目录/工作树。当前 CLI 是 `kc checkout`。 |
| mount Workspace | 把固定 pin 投影为宿主只读文件系统。只使用 `kcfs mount`；`mount` 不再表示接入 Repository。 |
| search knowledge | 按 Schema AccessHints 检索并从同一 basis 回读 Canonical。它不是文件 contains；普通文件使用 checkout/VFS + `rg`。 |

## 4. 禁止的别名

以下名称不进入新的公开合同：

- `WorkspaceView`、`ResolvedView`、`viewRef`、`ViewLease`：统一为
  `ResolvedWorkspace`/pin；
- `WorkspaceSession`、`sessionId`：不进入 Workspace 或认证公开合同。传输连接、SDK
  任务对象、FUSE 进程可以在实现内称 session，但不能成为身份、Pin 或续租资源；
- 裸 `View`：必须写明 `SearchView`、Preview 或 Workspace pin 中的哪一种；
- `Workspace Files API`：统一为 `Workspace File Gateway`；
- `kc mount` 表示 Repository 接入：Repository 使用 `kc repo-add`，宿主挂载使用
  `kcfs mount`；
- `Repo` 作为正式领域名称：公开说明使用 `Repository`，仅保留既有 `--repo`、
  `repo-add` 和实现内短变量。

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
       → ProjectionMaintainer → OpenSearch/SQLite/StarRocks projection
```
