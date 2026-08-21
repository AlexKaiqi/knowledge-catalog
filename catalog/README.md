# catalog/

**Catalog 是 ① 组合空间**：哪些 Snapshot 可以一起读。它不是知识库，不解析 Aspect，也不按 Unity 那样 `catalog.schema.table` 当容器名。`define-view --source` 不授予读权；仓级 ACL 见 [`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md)。分层见 [`docs/LAYERS.md`](../docs/LAYERS.md)。

一间 Catalog 里有很多 View。公司级默认就这一间（`kc init --catalog acme/catalog` → `kr://acme/catalog`）；再开一间是因为组合治理要隔离，不是因为多了几个仓。

两个变化源：

| 源 | 谁 | 动什么 | 效果 |
|---|---|---|---|
| 组合 | 消费方 | View（哪几个仓、跟哪根已发布分支） | 拼盘 |
| 内容 | 发布者 | 知识仓分支（COMMIT / merge 进 main） | 拼盘里实际是什么 |

仓的登记（`REGISTER_REPOSITORY` / `repository-*.yaml`）和 `ViewDefinition.sources` 不是同一份名单：前者是「这间 Catalog 承认哪些 Repository 可以入配方」，`kc read --catalog` 里是 `repositories`（仓 id 列表）；后者是某条配方此刻组合哪些仓。`OpenView` 时才把 selector 解成 `{仓 → commit}`，**不落盘**。`define-view --source` 不授予读权。

```text
Catalog  kr://acme/catalog
│
├── repositories                    ← 按 id 引用；正文仍在各自库里
│     kr://acme/public/core
│     kr://acme/groups/payments
│
└── ViewDefinition   配方：组合哪些 repo + 已发布 selector
```

可以有多间 Catalog（另一间例如 `kr://acme/restricted/catalog` 仅当登记名单本身不可见），各有自己的 Registry。知识仓按 id 引用，不各拷一份。写/读走 Writer / Reader 的 `--repo`。没有 Host / 进程这种协议对象。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| **Catalog** | `kc init` 第一间；`catalog-add --catalog <id>` 再开一间 | 组合治理要分开时再开（谁可 define-view、承认哪些仓）；不按 repo / 微服务 |
| **Repository** | `repo-add --repo kr://…`（本机新建 FileGit；给链接挂仓尚未做） | 挂的是 ⓪ Snapshot；各 Catalog 共享。知识读写是 ②（`put` / `read`） |
| **ViewDefinition** | `define-view [--catalog]` | 改 revision；下次 `OpenView` 用新配方 |

没有登记表里的 Generation / Release。一次 `read --view` 开始时 `ResolveView`：对各 source `GetRef(selector)`，**命令内冻结、不落盘**。commit 仍出现在 `FederatedValue` / citation。

Catalog 是可创建的组合空间。`kc init --catalog acme/catalog` 创建第一间（`kr://acme/catalog`），登记表 git 留下 `init …` 提交。当前组合空间是 `kc read --catalog`（`DumpState`：`catalogId` / `repositories` / `views`）。改配方就是这份 git 的历史（`kc audit`）；`--as` / `--request-id` 写进 commit。协议面过程账在 `.kc/system.jsonl`，`kc` 命令时间线在 `.kc/audit.jsonl`。再开一间用 `catalog-add`，动词加 `--catalog` 选。**不要**为每个库、每个服务再开一间——那是 Repository / View 的事。`.kc` 只是本机 `kc` 找文件用的，不是协议对象。

不要 `repo-add` 任何 Catalog id。登记表不是 View 的 source。

## 文件（按变化拆）

| 文件 | 负责 |
|---|---|
| `catalog.go` | `Catalog`：构造、工作集、操作分组 |
| `definition.go` | 配方：`DefineView` / `View` |
| `resolve.go` | `ResolveView` / overlay / `FederatedRead` / `CheckResolved` |
| `serving.go` | 消费读：`OpenView` → 这次解开的 `ViewReadVersion`；不传仓/commit |
| `indexplan.go` | View 当前解析上的 `IndexPlan`：各仓 AccessHints → lanes / schemaDigest；不落登记表。物理索引在 `index/` |
| `hook.go` | 进程内 `Hook`：只有 `AfterSnapshot`。Store 发 Snapshot，不是 facade，也不是出站 `kc hook-add` |
| `registry.go` | 这一间 Catalog 的落盘：`catalogs/<encoded-id>/*.yaml`（`catalog.yaml` / `view-*.yaml` / `repository-*.yaml`），独立 git。旧 `generation-*` / `release-*` 加载时忽略，下次 Save 删掉 |
| `log.go` | 登记表历史：`Catalog.Log`，对着那些 yaml 做 git log，不是 Repository `LOG` |

消费读走 `Serving`：`OpenView("analyst-board")` 把 selector 解成 `{仓 → commit}`，之后 `Read` / `List` / `Resolve` / `GetProvenance` / `DescribeSchema` / `Log` 都钉在这组坐标上。调用方不传 `--repo` / `--ref` / `--commit`。CLI 是 `kc read --view analyst-board`。SEARCH 按这次的 `IndexPlan` 分仓定位，命中后回读同一 commit；发布者刚推上已发布分支，下次 search 能命中。`kc log --view --object` 是对象历史；`kc read --catalog` 是组合空间当前态；`kc audit` 是登记表 git。

ControlPlane Preview 绑 View + overlay `{仓 → candidate}`，内容哈希当 `previewId`，只写 `.kc` 的 ControlState，不写登记表。`merge` 快进仓 Ref 后，下次 `read --view` 自然解到新 HEAD。

## 生命周期（在一间 Catalog 里）

```text
kc init / catalog-add  →  一间 Catalog 出现（空登记表）
DEFINE_VIEW            →  空间里多一条配方（可反复改 revision）
OPEN_VIEW / READ       →  解 selector，命令内冻 {仓 → commit}
RETIRE_DEFINITION      →  kc retire-view：这条配方不能再 OpenView
ARCHIVE_CATALOG        →  kc archive-catalog：整间只读历史，没有 DELETE
REGISTER_REPOSITORY    →  kc register（repo-add 会登记到默认 Catalog）
ARCHIVE_REPOSITORY     →  kc archive-repo：仓禁写；新 OpenView 不选入
```

无别名：`pin-view` / `promote` / `rollback` / `retire-release` / `--release` 打了只给改用提示。

## CLI

```bash
go run ./cmd/kc -- init --catalog acme/catalog
go run ./cmd/kc -- catalog-add --catalog kr://acme/docs/catalog
go run ./cmd/kc -- define-view --view agent --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- define-view --catalog kr://acme/docs/catalog --view docs --revision 1 --source kr://acme/public/core=refs/heads/main
go run ./cmd/kc -- read --catalog
go run ./cmd/kc -- read --view agent --object ETLTask:job-1
go run ./cmd/kc -- index-plan --view agent
go run ./cmd/kc -- audit --view agent
go run ./cmd/kc -- audit --catalog kr://acme/docs/catalog
```
