# catalog/

**Catalog 是 ① 组合平面**：承认哪些 Snapshot、Workspace 跟哪根已发布分支。它不是文件仓库（那是⓪ `snapshot.Store`），不是知识库，不解析 Aspect / `object_id` / event payload。知识协议在 writer / reader / index 上层包装。`define-workspace --source` 不授予读权；仓级 ACL 见 [`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md)。分层与入侵检查见 [`docs/LAYERS.md`](../docs/LAYERS.md)。

一间 Catalog 里有很多 Workspace。公司级默认就这一间（`kc local init --catalog acme/catalog` → `kr://acme/catalog`）；再开一间是因为组合治理要隔离，不是因为多了几个仓。

两个变化源：

| 源 | 谁 | 动什么 | 效果 |
|---|---|---|---|
| 组合 | 消费方 | Workspace（哪几个仓、跟哪根已发布分支） | 拼盘 |
| 内容 | 发布者 | 知识仓分支（COMMIT / merge 进 main） | 拼盘里实际是什么 |

发布就是推仓分支；Catalog 不再维护第二个发布对象。

仓的登记（`REGISTER_REPOSITORY` / `repository-*.yaml`）和 `WorkspaceDefinition.sources` 不是同一份名单：前者是「这间 Catalog 承认哪些 Repository 可以入配方」，登记表里是仓 id 列表；后者是某条配方此刻组合哪些仓。消费面 `kc catalog show` 的 `repositories` 由应用层附上源说明，不是登记表字段。`ResolveWorkspace` 时才把 selector 解成 `{仓 → commit}`，**不落盘**。`define-workspace --source` 不授予读权。

```text
Catalog  kr://acme/catalog
│
├── repositories                    ← 按 id 引用；正文仍在各自库里
│     kr://acme/public/core
│     kr://acme/groups/payments
│
└── WorkspaceDefinition   配方：组合哪些 repo + 已发布 selector
```

可以有多间 Catalog（另一间例如 `kr://acme/restricted/catalog` 仅当登记名单本身不可见），各有自己的 Registry。知识仓按 id 引用，不各拷一份。写/读走 Writer / Reader 的 `--repo`。没有 Host / 进程这种协议对象。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| **Catalog** | `kc local init` 第一间；`kc local catalog attach --catalog <id>` 再开一间 | 组合治理要分开时再开（谁可定义 Workspace、承认哪些仓）；不按 repo / 微服务 |
| **Repository** | `kc local repository attach --repo kr://…`（本机 Dolt 用 `--dir`；远程 Gitea 用 `--dsn`） | 挂的是 ⓪ Snapshot；各 Catalog 共享。知识读写是 ②（`kc writer put` / `kc knowledge read`） |
| **WorkspaceDefinition** | `kc workspace define [--catalog]` | 改 revision；下次 `ResolveWorkspace` / `reader.Open` 用新配方 |

一次 `kc knowledge read --workspace` 开始时 `ResolveWorkspace`：对各 source `GetRef(selector)`，固定 `{repo → commit}`，**命令内冻结、不落盘**。Catalog 不解 `object_id`，也不认识 Aspect Binding 或动态 observation cut。

Catalog 是可创建的组合空间。`kc local init --catalog acme/catalog` 创建第一间（`kr://acme/catalog`），登记表 git 留下 `init …` 提交。当前组合空间是 `kc catalog show`（`DumpState` 的 id，加上应用层拼的源说明）。改配方就是这份 git 的历史（`kc catalog audit`）；`--as` / `--request-id` 写进 commit。协议面过程账在 `.kc/system.jsonl`，`kc` 命令时间线在 `.kc/audit.jsonl`。再开一间用 `kc local catalog attach`，Catalog 命令加 `--catalog` 选。**不要**为每个库、每个服务再开一间——那是 Repository / Workspace 的事。`.kc` 只是本机 `kc` 找文件用的，不是协议对象。

不要把任何 Catalog id 交给 `kc local repository attach`。登记表不是 Workspace 的 source。

## 文件（按变化拆）

根包 `catalog` 是 ① 协议：承认仓、Workspace 配方、一次命令内 pin、路径路由、登记表。宿主 git worktree 在 `catalog/worktree/`：它消费配方和 pin，不是登记表状态，也不是新协议层。

| 文件 | 负责 |
|---|---|
| `catalog.go` | `Catalog`：构造、工作集、操作分组 |
| `definition.go` | 配方：`DefineWorkspace` / `Workspace` |
| `recipe.go` | 仓根 `.kc-workspace.yaml`：mount 配方的便携形态（跟着 git 走）；Catalog 仍是本机操作库 |
| `overlay.go` | 本机叠加层：`MergeOverlay` / `OverlayFile`（对标 `local_manifests`，不进登记表） |
| `resolve.go` | `ResolveWorkspace` / overlay Preview / `CheckResolved`；`PinID` 哈希路径布局；`BaseRev` CAS |
| `mount.go` | 路径布局校验、`RouteMount` / `RouteMounts`、`NormalizeMountPath` |
| `virtual.go` | 固定 pin 的单文件路由与 mount metadata；目录遍历由 Workspace File Gateway 的 `snapshot.DirectoryReader` 分页完成 |
| `hook.go` | 进程内 `Hook`：只有 `AfterSnapshot`（仓 from→to）。Store 发 Snapshot；index 自己算 object_id |
| `lifecycle.go` | 仓登记、Workspace 退役、Catalog/仓归档 |
| `files.go` | 登记表文件命名（`workspace-<token>.yaml` 等）；一条记录一个文件，`kc catalog audit --workspace` 才能问单个 Workspace 的 git 历史 |
| `state.go` | `CatalogState` 与归一化；不含 I/O |
| `registry.go` | 一间 Catalog 的 Registry 打开、加载、保存和 git CAS |
| `registry_codec.go` | `CatalogState` ↔ 登记表 YAML 文件集合 |
| `registry_discovery.go` | Catalog 根目录、默认 id 与只读 stamp 探测 |
| `log.go` | 登记表历史：`Catalog.Log`，对着那些 yaml 做 git log，不是 Repository `LOG` |
| `worktree/` | 宿主 git 检出 / 同步 / status / 收集本地写；`CheckoutMounts` 不是 `Catalog` 方法 |

登记表的 git 落在 `internal/gitdir`（纯 plumbing），**不是** ⓪ 的 Snapshot 适配器。本包不许 import `reader` / `index` 或任何具体 Snapshot adapter：登记表是 ① 自己的配置文件，不是知识。这条由 `internal/arch` 断言。

消费读在 `knowledge/reader/`：Client 先经 Server `ResolveWorkspace`，再由应用服务组合 `reader.Open`；之后 `Read` / `List` / `ResolveBinding` 才带 `object_id`。逻辑查询合同在 `retrieval/`。上层 Materialization runtime 在这个声明 pin 之上自行固定 observation basis。`worktree.CheckoutMounts` / `reader.WriteCheckout` 仅是内部物化机制，当前不是公开 CLI；文件产品入口是 `kcfs` → Workspace File Gateway。

ControlPlane Preview 绑 Workspace + overlay `{仓 → candidate}`，内容哈希当 `previewId`，只写 `.kc` 的 ControlState，不写登记表。`merge` 快进仓 Ref 后，下次 `read --workspace` 自然解到新 HEAD。

## 生命周期（在一间 Catalog 里）

```text
kc local init / catalog-add  →  一间 Catalog 出现（空登记表）
DEFINE_WORKSPACE            →  空间里多一条配方（可反复改 revision）
OPEN_WORKSPACE / READ       →  解 selector，命令内冻 {仓 → commit}
RETIRE_DEFINITION      →  kc workspace retire：这条配方不能再 OpenWorkspace
ARCHIVE_CATALOG        →  kc catalog archive：整间只读历史，没有 DELETE
REGISTER_REPOSITORY    →  kc catalog repo register（attach 只打开 Snapshot，不登记）
ARCHIVE_REPOSITORY     →  kc catalog repo archive：仓禁写；新 OpenWorkspace 不选入
```


## CLI

以下业务命令假设已配置 `KC_SERVER_URL` 和 `KC_AS`；只有前两条 `kc local` 属于宿主 bootstrap。

```bash
go run ./cmd/kc -- local init --catalog acme/catalog
go run ./cmd/kc -- local catalog attach --catalog kr://acme/docs/catalog
go run ./cmd/kc -- workspace define --workspace agent --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- workspace define --catalog kr://acme/docs/catalog --workspace docs --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- catalog show
go run ./cmd/kc -- knowledge read --workspace agent --object ETLTask:job-1
go run ./cmd/kc -- operations access-spec describe --workspace agent
go run ./cmd/kc -- catalog audit --workspace agent
go run ./cmd/kc -- catalog audit --catalog kr://acme/docs/catalog
```
