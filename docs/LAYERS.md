# 协议分层（⓪–③）

日期：2026-08-25

本文回答 git、Catalog、Aspect、动态物化与检索分别由谁感知。介质的权威/索引/缓存/投影职责见 `STORE_ADAPTERS.md`。

---

## 1. 主张

Knowledge Catalog 底座的权威 Store 只有 Snapshot。实时状态和事件流不再作为 ⓪ Stream 进入 Repository、Writer 或 Catalog；② 只保存稳定 Binding，墙外 Materialization Runtime 提供动态观察，③ Retrieval 消费它。

```text
③ 检索派生     AccessSpec / RetrievalPlan / CandidateRef / hydrate
       ↑
M 访问物化      state / stream / cursor / watermark / projection controller
               墙外上层产品，不是底座编号层
       ↑
② 知识内容     object_id / Aspect / schema / provenance / Binding handle
────────────────────────────────
① 组合平面     Catalog / Workspace / 一次命令内 {repo → commit}
────────────────────────────────
⓪ Snapshot     git tree / commit / ref / CAS
```

M 在语义上位于知识声明之上、检索派生之下，但不进入底座的 import DAG：② 不依赖 runtime；③ 通过上层 provider 接缝使用 runtime。

---

## 2. 入侵检查

| 要动的东西 | 落点 | 禁止 |
|---|---|---|
| 挂仓、commit、ref、CAS | ⓪ SnapshotStore | 挂载时要求 Aspect；Catalog 解析 frontmatter |
| 承认仓、Workspace、selector、pin | ① `catalog/` | `object_id`、Binding、动态 cursor、AccessPlan |
| PUT/READ、Address、来源、Schema、Binding 声明 | ② Writer/Reader | 直接调用外部 runtime；直写 git 绕过 Writer |
| state/stream lookup、window、cursor、watermark、retention | M 上层产品 | 注册成 Repository；塞进 Workspace pin；由 Writer APPEND |
| 检索定位、路由与 hydrate | ③ Retrieval/Index | 索引或外部 score 冒充 Canonical |
| 凭证、endpoint、运行 generation | 墙外运行基础设施 | 写入知识正文或 Catalog Registry |
| 访问可观测性 | 横切 `observability/`：身份上下文、版本化访问账、Agent trace/反馈、派生 hitmap | 把访问次数写回知识对象；把 hitmap 当 Canonical 或授权依据 |

上层只消费下层提供的稳定接口，反向不许。底座 import 规则由 `internal/arch` 强制；M 的具体实现不进入本仓库核心包。

---

## 3. 各层看见什么

| 层 | 看见 | 不看见 |
|---|---|---|
| ⓪ Snapshot | git URL、commit、ref、tree/blob、CAS | `object_id`、Aspect、Workspace、Binding、索引 |
| ① Catalog | Repository id、Workspace 配方、`{repo → commit}` | 正文、Aspect、动态 observation、检索引擎 |
| ② Knowledge | identity、Aspect、PUT/REMOVE、provenance、Schema、Binding handle | 凭证、运行状态、物理索引 |
| M Materialization | 固定 Binding generation、state/stream、cursor/watermark、health | 改 Repository/Catalog；发明 object_id |
| ③ Retrieval | AccessSpec、Binding/provider capabilities、projection basis、无正文 CandidateRef | 把候选或物理 stored fields 当知识结果 |

挂普通 Git 仓停在 ⓪+①。READ/SEARCH 才要求该 commit 上存在可解释的 ② 知识。动态 Aspect 的声明仍由 Workspace commit 固定；动态 observation basis 在 Retrieval 请求开始时由上层产品观察。

---

## 4. `internal/` 的边界

| 包 | 是什么 | 不是什么 |
|---|---|---|
| `internal/gitdir` | ⓪ Adapter 与 ① Registry 共用的 git plumbing | SnapshotStore；知识解释器 |
| `internal/repofile` | ② 的磁盘单元格式与安全路径机制 | Store；Materialization Runtime |
| `internal/journal` | 本机过程账 | 协议对象；外部事件流 |

`observability/` 不属于 ⓪–③ 的知识层级：它只记录对这些层的调用证据。访问目标必须使用固定 `repository + commit + object/Address`；hitmap 是可重建统计，不进入成员仓、Catalog 或索引权威。

下沉到 `internal/` 只用于让两个不应互相依赖的底座包复用机制。不要用它把动态运行时偷偷带回核心。

---

## 5. 写面与观察面

底座写面只有：

```text
COMMIT    PUT/REMOVE → Snapshot authority
PROPOSAL  PUT/REMOVE → candidate Snapshot
```

State/Stream Binding 是观察面，不是 Writer Surface。动态数据若需要成为可版本化知识，Collector 在明确 scope 和 provenance 下把某次观察翻译成 Snapshot ChangeSet，再走 COMMIT；不会保留一个底座 APPEND 快车道。

---

## 6. 外部资源与检索

Aspect 可以内嵌 Binding，也可以引用 ResourceDescriptor。声明包含资源语义、逻辑能力和结果 Schema；凭证与实际 endpoint 留在墙外。

已知句柄访问只解决 hydrate。要支持 discovery，③ 根据 Schema AccessHints 与 Binding capabilities 选择：

- Snapshot projection；
- source-side search pushdown；
- 上层产品维护的 State/Stream projection。

命中后回 Snapshot 或固定 Binding 读取完整知识，并同时返回查询 view、知识版本、commit basis 与 observation basis。结果裁剪属于更上层的上下文组装，不是索引或 SEARCH 的职责。详见 `LIVE_MATERIALIZATION.md`。

---

## 7. 具体协议位置

- ⓪ Snapshot：`repository.SnapshotStore`、`local.FileGitRepository`、`gitea.Repository`、`scale.DoltRepository`。
- ① Composition：`catalog/`。
- ② Knowledge：`kernel/`、`writer/`、`reader/` 与 Repository 中的 `schema/*`。
- ③ Snapshot Index：`index/`；跨 Snapshot/State/Stream 的 RetrievalPlan 属于待建上层产品。
- M Binding 语义：`LIVE_MATERIALIZATION.md`；具体运行时不放进本仓库核心。
