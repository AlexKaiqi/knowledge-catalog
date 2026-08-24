# 协议分层（⓪–③）

日期：2026-08-21
范围：哪一层感知 git、哪一层感知 Catalog、哪一层感知 Aspect / 索引。  
对照：`KNOWLEDGE_CATALOG_DESIGN.md` 第 0.15 节；物理引擎见 `STORE_ADAPTERS.md`（权威 / 索引 / 缓存 / 投影是 **介质梯子**，不要和本文件的 ⓪–③ 混成一套「四层」）。

---

## 主张

Catalog **不是**文件仓库，也 **不是**知识协议。文件仓库是 ⓪ SnapshotStore。知识协议是加在坐标上的上层包装。APPEND 是 ⓪ 流；Catalog 只冻结 cursor，不读 payload。

```text
③ 检索派生     IndexPlan / AccessHints / 命中后回读 Canonical     ← 上层包装
② 知识内容     object_id、Aspect、来源信封、schema/*              ← 上层包装
────────────────────────────────
① 组合平面     Catalog：承认仓、Workspace 配方、跟已发布分支
               一次命令内解 {仓 → commit} + 附属 AppendCuts
────────────────────────────────
⓪ 文件仓+流    Snapshot = git（tree / commit / ref / CAS）
               Stream  = 有序段（cursor / eventId）  ← 不是 git、不是仓
```

依赖只许向上：① 解出 ⓪ 坐标；② 解释该坐标上的文件；③ 读 ② 在这次 commit 上的 `schema/*`。禁止 ⓪ 挂载口要求 Aspect。跨命令跟已发布分支；命令内不得跟随 `latest`。

---

## 入侵检查（缺了就停手）

| 要动的东西 | 落点 | 禁止 |
|---|---|---|
| 挂仓、commit、ref、CAS | ⓪ SnapshotStore | Catalog 解析 frontmatter |
| APPEND、cursor、eventId、payload | ⓪ Stream（可绑仓 id） | 流当仓、`repo-add --driver stream`、Catalog 读 payload |
| 承认仓、Workspace、跟分支、解坐标、钉 stream cursor | ① `catalog/` | `object_id`、Aspect、IndexPlan、额外的发布状态对象 |
| PUT/READ 拼装、来源信封 | ② Writer / Reader | 直写 git 绕过 Writer；`catalog/` import `reader`/`index` |
| Workspace 只读检出（grep） | `reader.WriteCheckout`；`layout.checkouts` | 挂 `.kc/repos` / `kc serve` tree 当 Workspace；pathHint 当身份；直写检出 |
| 检索定位 | ③ index | 索引当权威；Catalog Hook 带 object 列表 |

上层包装只许 **import ① 的坐标，反向不许**。要再封装，加包，不要往 `catalog/` 里长。

「反向不许」是**编译期强制**的，不只是约定：`internal/arch` 用 import 图断言本表。`go test ./internal/arch/` 失败时会打印违规的传递路径。改这张表要连规则一起改，不要只改代码。

注意「间接违规」也算：曾经 `catalog/registry.go` 直接持 `*local.FileGitRepository`，于是 ① 通过 ⓪ 的实现把 `reader`/`index` 拖进依赖图。现在登记表落在 `internal/gitdir`（纯 git plumbing），`catalog` 的 kc 依赖只有 `kernel`、`repository` 与三个 internal 库。

---

## `internal/` 是给两层共用的下沉物

| 包 | 是什么 | 谁用 | 不是什么 |
|---|---|---|---|
| `internal/gitdir` | git 目录 plumbing：init、config stamp、ref、tree 读、worktree commit、log；commit 签名与 `Request-Id`/`Rule-Id` trailer 的唯一实现 | ⓪ `local`、`gitea`；① `catalog` 登记表 | 不是 Snapshot 口（那是 `repository.SnapshotStore`），不认识 `object_id` |
| `internal/repofile` | ② 的磁盘单元格式：frontmatter + JSON body、`Tree`、PUT/REMOVE 落文件、`SafeRelativePath` | ⓪ 适配器、`writer` 预览 | 不是 store |
| `internal/journal` | 本机过程账（`system.jsonl`） | 各层 | 不是协议对象 |

下沉到 `internal/` 的判据只有一个：**两个不该互相依赖的包需要同一段机制**。`gitdir` 就是这么来的 —— 让 ① 的登记表和 ⓪ 的 Snapshot 适配器共用 git 机制，而不必让 ① 认识 ⓪ 的实现类型。

---

## 各层看见什么

| 层 | 看见 | 不看见 |
|---|---|---|
| **⓪ Snapshot** | git URL、commit、ref、tree/blob、CAS | `object_id`、Aspect、Workspace、IndexPlan |
| **⓪ Append** | stream 名、cursor、`eventId`、不可变 Entry | git commit、Aspect、合订本的一页 |
| **① Catalog** | 仓 id、Workspace 配方（已发布 selector）；一次命令内 `{repo → commit}`；成员上已有流的 cursor（`ResolvedWorkspace.AppendCuts`） | `object_id`、对象正文、Aspect、event payload、检索引擎；不落盘第二套发布状态 |
| **② 知识** | frontmatter 身份、Aspect 分区、PUT/REMOVE、来源信封、`schema/*`；`reader.Serving` 联邦读 | 把 git 当知识协议；把索引当权威 |
| **③ 检索** | AccessHints → IndexPlan → 定位 `object_id` | Canonical 正文；按 Workspace 复制整列 |

**发布**不是 Catalog 里的第二种对象。发布者推知识仓自己的分支（COMMIT / merge）。Catalog 只声明消费跟哪根 selector。变化源就两个：改 Workspace；推已发布分支。

挂用户仓停在 ⓪+①：给 git 链接、给平台读授权，Catalog 只记 id 与 Workspace 配方，**不把正文收进登记表**。要 `READ` 拼装 / `SEARCH`，该仓在这次解开的 commit 上还须符合 ②。普通代码仓可以挂，不是知识仓。写权威在外部的仓外部 push 是预期而非事故，代价与降级见 `COMPOSITION.md`。

消费方点 `--workspace`：一次命令开始时 `ResolveWorkspace`。不要让消费请求带 `--repo` / `--ref` / `--commit`。facade 把坐标交给 `reader.Open`；commit / cut 仍出现在结果里。`object_id` 从 ② 和 ③ 才出现。

观测 / 只追加走 ⓪ 的流。`kc stream --workspace` 用这次钉的 AppendCuts，不是 live head。不要 `repo-add --driver stream`。

---

## 和「Store 派生四层」怎么并存

`STORE_ADAPTERS.md` 的权威 / 索引 / 缓存 / 投影回答的是：**同一层语义用什么引擎**。

- Snapshot 权威、APPEND 权威 → 落在 **⓪**
- 全文 / 列投影 / 热尾 → 落在 **③**（可丢；命中回读 ⓪ 上的 Canonical，知识形态由 ② 解释）
- Workspace checkout（grep Provider）→ 可丢文件系统投影，落在 `layout.checkouts`；钉这次 `ResolveWorkspace`；不是 ③，不是权威，不是成员 git
- Catalog 登记表 git 是 **①** 的落盘，不是知识仓本身

本机 FileGit 把 JSONL 放在 `.git` 旁，只是 ⓪ 两种权威的 **落盘同居**，不是「APPEND 是 git 的一种能力」，也不是 Catalog 的子模块。

---

## 参考实现怎么切（当前）

| 层 | 包 | 口 |
|---|---|---|
| ⓪ Snapshot | `repository.SnapshotStore`；`local.FileGitRepository`；`scale.DoltRepository`；`gitea.Repository` | commit / ref / CAS；默认 ref 是 `repository.DefaultRef` |
| ⓪ Append | `repository.Stream`；`local.JSONLStream`；`scale.OpenStream`（stub Append） | `APPEND` / `StreamRefs` / cursor |
| ① | `catalog/`（Registry、Workspace、ResolvedWorkspace）；登记表落在 `internal/gitdir` | `ResolveWorkspace`：`HasCommit` + 钉 `AppendCuts`；不解 Aspect / object_id / payload |
| ② | `kernel.Address`；Writer `PUT`/`COMMIT`；Reader 单仓拼装；`reader.Serving` 消费联邦 | Serving 骑在这次 `WorkspacePin` 上调 Knowledge；`WriteCheckout` 是拼装结果的只读落盘 |
| ③ | `index/`；`schema/*` AccessHints；`reader.IndexPlan` | Catalog.Hook（`AfterSnapshot` from→to）；index 自己算 object_id |

`repository.Repository` = SnapshotStore + Knowledge，但那是**能力**不是入场券：`Store.Add` 只要 SnapshotStore，普通 git 仓照样挂得进来、组合、检出、按路径收写。需要 ② 的地方（reader 拼装、index、带 `schema_ref` 的 PUT）问 `Store.Knowledge`，缺了报 `CAPABILITY_UNSATISFIED`，不在挂载时预先拦。APPEND 走 `Store` 上按仓 id 绑定的 Stream。Catalog 登记的是 SnapshotStore；流 ≠ 仓。

T12：`RepositoryContract` 跑 FileGit、Dolt 与 Gitea；`StreamContract` 跑 JSONLStream。

新增 ⓪ 适配器要付的成本，只是 `repository.Repository` 的实现 + 过 T12；不必重写 ② 的语义。`repository.FastChanges` 是可选加速（FileGit 用 `diff-tree`），不实现就退回比对 `List` digest，两条路必须给同一答案。

用户给链接挂仓是 ⓪ 的产品能力。`kc repo-add --driver gitea --dsn http(s)://{host}/{owner}/{name}` 挂远程 Snapshot。`--driver filegit|dolt` 仍在 `--home/repos/` 新建本地仓。

---

## 写面落在哪

| Surface | 层 | target |
|---|---|---|
| `COMMIT` / `PROPOSAL` | ⓪ Snapshot 上产生 commit；ChangeSet 的 PUT/REMOVE 是 **②** | 唯一 Snapshot |
| `APPEND` | ⓪ 流 | 唯一 Stream（可绑定某仓做 ACL，但流 ≠ 仓） |

K-01：一次写入一个 target。COMMIT/PROPOSAL 的 target 是 Snapshot；APPEND 的 target 是 Stream。

Workspace 本身不是写 target，但它的 mount 路径**决定**落点：改哪个文件 → 归属仓唯一 → 一次 COMMIT 一个仓，K-01 自动满足。路径布局、写回路由、挂已有 git 仓见 `COMPOSITION.md`。
