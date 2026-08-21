# 协议分层（⓪–③）

日期：2026-08-20  
范围：哪一层感知 git、哪一层感知 Catalog、哪一层感知 Aspect / 索引。  
对照：`KNOWLEDGE_CATALOG_DESIGN.md` 第 0.15 节；物理引擎见 `STORE_ADAPTERS.md`（权威 / 索引 / 缓存 / 投影是 **介质梯子**，不要和本文件的 ⓪–③ 混成一套「四层」）。

---

## 主张

先有操作语义和组合，再有「文件必须长成知识」。Aspect、声明式索引不是挂仓的前提。

```text
③ 检索派生     IndexPlan / AccessHints / 命中后回读 Canonical
② 知识内容     object_id、Aspect、来源信封、schema/*
① 组合         Catalog：仓引用 + 钉死 {仓 → commit}（流则钉 cursor）
⓪ 操作语义     Snapshot = git（tree / commit / ref / CAS）
               Append  = 有序段（cursor / eventId）  ← 不是 git
```

依赖只许向上：① 只钉 ⓪ 的坐标；② 解释 ⓪ 在该坐标上的文件；③ 读 ② 在已钉死 commit 上的 `schema/*`。禁止 ⓪ 的挂载口要求 Aspect。

---

## 各层看见什么

| 层 | 看见 | 不看见 |
|---|---|---|
| **⓪ Snapshot** | git URL、commit、ref、tree/blob、CAS | `object_id`、Aspect、View、IndexPlan |
| **⓪ Append** | stream 名、cursor、`eventId`、不可变 Entry | git commit、Aspect、合订本的一页 |
| **① Catalog** | 仓 id、View 配方、`{repo → commit}`、Release；流可钉 cursor（`ViewReadVersion.AppendCuts`） | 对象正文、Aspect 名、检索引擎 |
| **② 知识** | frontmatter 身份、Aspect 分区、PUT/REMOVE、来源信封、`schema/*` | 把 git 当知识协议；把索引当权威 |
| **③ 检索** | AccessHints → IndexPlan → 定位 `object_id` | Canonical 正文；按 View 复制整列 |

挂用户仓停在 ⓪+①：给 git 链接、给平台读授权，Catalog 只记 id 与钉死的 commit，**不把正文收进登记表**。要 `READ` 拼装 / `SEARCH`，该仓在钉死 commit 上还须符合 ②（以及 ③ 的 Hints）。普通代码仓可以挂、可以 pin，不是知识仓。

消费方走 View 访问口：点 `--release`，看见的是那一代钉死的数据。不要让消费请求带 `--repo` / `--ref` / `--commit`。维护方 pin / promote；`Catalog.Serving`（`OpenRelease`）解指针并联邦读。

观测 / 只追加走 ⓪ 的流，不经 ② 才存在，也不要 `repo-add --driver stream`。

---

## 和「Store 派生四层」怎么并存

`STORE_ADAPTERS.md` 的权威 / 索引 / 缓存 / 投影回答的是：**同一层语义用什么引擎**。

- Snapshot 权威、APPEND 权威 → 落在 **⓪**
- 全文 / 列投影 / 热尾 → 落在 **③**（可丢；命中回读 ⓪ 上的 Canonical，知识形态由 ② 解释）
- Catalog 登记表 git 是 **①** 的落盘，不是知识仓本身

本机 FileGit 把 JSONL 放在 `.git` 旁，只是 ⓪ 两种权威的 **落盘同居**，不是「APPEND 是 git 的一种能力」。

---

## 参考实现怎么切（当前）

| 层 | 包 | 口 |
|---|---|---|
| ⓪ Snapshot | `repository.SnapshotStore`；`local.FileGitRepository`；`scale.DoltRepository`；`gitea.Repository` | commit / ref / CAS |
| ⓪ Append | `repository.Stream`；`local.JSONLStream`；`scale.OpenStream`（stub Append） | `APPEND` / cursor |
| ① | `catalog/`（Registry、View、Generation、Release、Serving） | pin 检查 `HasCommit`；消费读 `OpenRelease`；不解析 Aspect |
| ② | `kernel.Address`；Writer `PUT`/`COMMIT`；Reader 单仓拼装；`Catalog.Serving` 消费联邦 | 内容约定。Serving 骑在 ① 的 pin 上调 Knowledge |
| ③ | `index/`；`schema/*` AccessHints；`IndexPlan` | Catalog.Hook 订阅；不进 Writer/Catalog 核心 |

`repository.Repository` = SnapshotStore + Knowledge。APPEND 走 `Store` 上按仓 id 绑定的 Stream（JSONL 同居是 packing）。Catalog 登记的是 SnapshotStore；不要把流 `repo-add` 成知识仓。

T12：`RepositoryContract` 跑 FileGit、Dolt 与 Gitea；`StreamContract` 跑 JSONLStream。

用户给链接挂仓（remote + 读授权、不拿走正文）是 ⓪ 的产品能力。`kc repo-add --driver gitea --dsn http(s)://{host}/{owner}/{name}` 挂远程 Snapshot（无工作区，本机只留 `remote.yaml`）。`--driver filegit|dolt` 仍在 `--home/repos/` 新建本地仓。

---

## 写面落在哪

| Surface | 层 | target |
|---|---|---|
| `COMMIT` / `PROPOSAL` | ⓪ Snapshot 上产生 commit；ChangeSet 的 PUT/REMOVE 是 **②** | 唯一 Snapshot |
| `APPEND` | ⓪ 流 | 唯一 Stream（可绑定某仓做 ACL，但流 ≠ 仓） |

K-01：一次写入一个 target。COMMIT/PROPOSAL 的 target 是 Snapshot；APPEND 的 target 是 Stream。
