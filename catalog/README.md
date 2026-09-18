# catalog/

**Catalog 是 ① 组合平面**：承认哪些 Snapshot、Dataset 发布哪一份冻住的文件清单。它不是文件仓库（那是⓪ `snapshot.Store`），不是知识库，不解析 Aspect / `object_id` / event payload。知识协议在 writer / reader / index 上层包装。`dataset define --source` 不授予读权；仓级 ACL 见 [`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md)。分层与入侵检查见 [`docs/LAYERS.md`](../docs/LAYERS.md)。Dataset 发布时把 selector 冻成 commit，pin 带文件清单；空清单不是整仓别名。

一间 Catalog 里有很多 Workspace。公司级默认就这一间（例如 `kr://acme/catalog`）；再开一间是因为组合治理要隔离，不是因为多了几个仓。

两个变化源：

| 源 | 谁 | 动什么 | 效果 |
|---|---|---|---|
| 组合 | 消费方 | Dataset（哪几个仓、发布时钉死的文件清单） | 拼盘 |
| 内容 | 发布者 | 知识仓分支（COMMIT / merge 进 main） | 拼盘里实际是什么 |

发布就是推仓分支；Catalog 不再维护第二个发布对象。

仓的登记（`REGISTER_REPOSITORY` / `repository-*.yaml`）和 `KnowledgeSet.sources` 不是同一份名单：前者是「这间 Catalog 承认哪些 Repository 可以入配方」，登记表里是仓 id 列表；后者是某条配方此刻组合哪些仓。消费面 `kc show` 的 `repositories` 仍是身份列表（加 `schemaCount`），不是登记表字段，也不附 README。`ResolveKnowledgeSet` 使用已发布的冻结 commit 与 `Items`，**不跟 live 分支**；`latest` 是当前最大 `revision`（显示为 `vN`），一次请求只解一次。`dataset define --source` 不授予读权。

```text
Catalog  kr://acme/catalog
│
├── repositories                    ← 按 id 引用；正文仍在各自库里
│     kr://acme/public/core
│     kr://acme/groups/payments
│
└── KnowledgeSet   配方：组合哪些 repo + 发布时冻结的 commit / 文件清单
```

可以有多间 Catalog（另一间例如 `kr://acme/restricted/catalog` 仅当登记名单本身不可见），各有自己的 Registry。知识仓按 id 引用，不各拷一份。写/读走 Writer / Reader 的 `--repo`。没有 Host / 进程这种协议对象。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| **Catalog** | 部署时显式 `CreateSnapshotRegistry`；运行时 `OpenSnapshotRegistry` 恢复既有权威 | 组合治理要分开时再开（谁可定义 Workspace、承认哪些仓）；不按 repo / 微服务 |
| **Repository** | `kc create` 供给或连接新仓；`kc attach --repo` 才登记进当前 Catalog | 创建的供给、连接、命令结果和授权在服务管理面；Catalog 只登记仓身份 |
| **KnowledgeSet** | `kc dataset define` | 改 revision；下次 `ResolveKnowledgeSet` / `reader.Open` 用新配方 |

`create` 不改变本包状态。应用层按部署显式策略供给或连接 Snapshot 并形成可撤销创建者
grant；之后独立 `attach` 才调用 Catalog 准入。平台连接、创建进度与凭证不进 CatalogState
或 registry YAML。

`kc read --dataset` 读取已发布 Dataset；命令开始时内部 resolve 一次（`V-01`），
回执带 `commit`。Catalog 不解 `object_id`。再发一版才会看到之后的仓提交。

Catalog 是可创建的组合空间，每间有独立 Snapshot 权威。生产部署用 `CreateSnapshotRegistry` / `OpenSnapshotRegistry` 在已配置的 Snapshot repository 上写入或恢复登记表；该仓不是 Knowledge Repository，也不走 Writer。`NewRegistry` 与 `CreateRemoteRegistry` 保留为组件夹具的本机 git 构造器，不是生产部署恢复入口。空 ref 参数使用 `snapshot.DefaultRef`。

运行目录只是可删除缓存；清空后可从 Catalog Snapshot 权威恢复登记名单、Workspace、归档状态与治理历史。`Registry.Save` 以读取时的 expected-old 对权威 ref 做 CAS；冲突返回 `NON_FAST_FORWARD`。

`Catalog.ReadView` 固定当前已接受的登记状态，为一次只读请求独立持有状态映射、journal 与身份印记。视图不重新加载远端、不订阅 Snapshot 推进事件，也不能持久写入；共享底层访问能力的生命周期由宿主负责。并发读取不会覆盖另一请求的审计身份。

当前组合空间是 `kc show`；改配方就是这份 Git 的历史（`kc catalog audit`）。Catalog id
不代表某个机器目录。

不要把任何 Catalog id 交给 `kc attach --repo`。连接只说明如何打开源，不授予 Catalog
成员资格；attach 的一次 Catalog 提交完成登记。

## 文件（按变化拆）

根包 `catalog` 是 ① 协议：承认仓、Workspace 配方、一次命令内 pin、路径路由、登记表。宿主 git worktree 在 `catalog/worktree/`：它消费配方和 pin，不是登记表状态，也不是新协议层。

| 文件 | 负责 |
|---|---|
| `catalog.go` | `Catalog`：构造、工作集、操作分组 |
| `definition.go` | 配方：`DefineKnowledgeSet` / `Workspace` |
| `recipe.go` | 仓根 `.kc-dataset.yaml`：mount 配方的便携形态（跟着 git 走）；配方可持久化到 Catalog 权威 |
| `overlay.go` | 任务叠加层：`MergeOverlay` / `OverlayFile`；客户端合成临时配方，不进登记表 |
| `resolve.go` | `ResolveKnowledgeSet` / overlay Preview / `CheckResolved`；`PinID` 哈希路径布局；`BaseRev` CAS |
| `mount.go` | 路径布局校验、`RouteMount` / `RouteMounts`、`NormalizeMountPath` |
| `virtual.go` | 固定 pin 的单文件路由与 mount metadata；目录遍历由 Workspace File Gateway 的 `snapshot.DirectoryReader` 分页完成 |
| `hook.go` | 进程内 `Hook`：只有 `AfterSnapshot`（仓 from→to）。Store 发 Snapshot；index 自己算 object_id |
| `lifecycle.go` | 仓登记、Workspace 退役、Catalog/仓归档 |
| `files.go` | 登记表文件命名（`dataset-<token>.yaml` 等）；一条记录一个文件，`kc catalog audit --dataset` 才能问单个 Dataset 的 git 历史 |
| `state.go` | `CatalogState` 与归一化；不含 I/O |
| `registry.go` | 一间 Catalog 的 Registry 加载、候选状态保存和 Git CAS |
| `registry_remote.go` | 独立 Git 权威的显式创建、只读恢复和缓存隔离 |
| `read_view.go` | 单次只读请求的登记状态与审计上下文隔离 |
| `registry_codec.go` | `CatalogState` ↔ 登记表 YAML 文件集合 |
| `registry_discovery.go` | Catalog 根目录、默认 id 与只读 stamp 探测 |
| `registry_store.go` | 生产 Catalog：独立 Snapshot 权威上的 YAML + ref CAS |
| `log.go` | 登记表历史：`Catalog.Log`，对着那些 yaml 的权威历史，不是 Repository `LOG` |
| `worktree/` | 宿主 git 检出 / 同步 / status / 收集本地写；`CheckoutMounts` 不是 `Catalog` 方法 |

生产登记表落在独立 Snapshot 权威（树 + CAS），**不是**知识仓、也不是 SQL。本包只依赖 `snapshot.Store` 接口，不许 import `reader` / `index` 或任何具体 Snapshot adapter。组件夹具仍可用 `internal/gitdir`。这条由 `internal/arch` 断言。

消费读在 `knowledge/reader/`：Client 先经 Server `ResolveKnowledgeSet`，再由应用服务组合 `reader.Open`；之后 `Read` / `List` / `ResolveBinding` 才带 `object_id`。逻辑查询合同在 `retrieval/`。上层 Materialization runtime 在这个声明 pin 之上自行固定 observation basis。`worktree.CheckoutMounts` / `reader.WriteCheckout` 仅是内部物化机制，当前不是公开 CLI；文件产品入口是 `kcfs` → Workspace File Gateway。

ControlPlane Preview 绑调用方给出的固定 pin + overlay `{仓 → candidate}`，内容哈希当
`previewId`，写部署耐久状态目录的 ControlState，不写登记表。

## 生命周期（在一间 Catalog 里）

```text
CreateSnapshotRegistry     →  一间 Catalog 出现（空登记表）
DEFINE_WORKSPACE            →  空间里多一条配方（可反复改 revision）
OPEN_WORKSPACE / READ       →  解 selector，命令内冻 {仓 → commit}
RETIRE_DEFINITION      →  kc dataset retire：这条配方不能再 OpenKnowledgeSet
ARCHIVE_CATALOG        →  kc catalog archive：整间只读历史，没有 DELETE
REGISTER_REPOSITORY    →  kc attach --repo（一次提交完成登记）
UNREGISTER_REPOSITORY  →  kc detach --repo（只移除 Catalog 成员关系）
```


## CLI

以下业务命令假设部署已显式创建并恢复 Catalog，客户端已配置 `KC_SERVER_URL` 和身份。

```bash
go run ./cmd/kc -- dataset define --dataset agent --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- catalog use kr://acme/docs/catalog
go run ./cmd/kc -- dataset define --dataset docs --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- show
go run ./cmd/kc -- read --dataset docs --object ETLTask:job-1
go run ./cmd/kc -- operations access-spec describe --dataset docs
go run ./cmd/kc -- catalog audit --dataset agent
```
