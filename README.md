# Knowledge Catalog — 单一语义协议 + Git Store（参考骨架）

面向 AI 知识底座的一套统一 Catalog 协议（Go 参考实现）。

**Catalog 语义只有一套**：身份、版本、来源、写边界、Workspace 组合、维护闭环、联邦读取。不同的是 store adapter。协议分层 ⓪–③（[`docs/LAYERS.md`](docs/LAYERS.md)；不要和介质梯子混名）：

```text
③ 检索派生     AccessSpec / RetrievalPlan / CandidateRef / 完整回读
M 访问物化      外部 State / Stream runtime（上层产品）
② 知识内容     object_id、Aspect、来源信封、schema/*、Binding handle
① 组合平面     Catalog：承认仓 + Workspace 配方；解 {仓 → commit}
⓪ 操作语义     Snapshot = git
```

挂用户 git 停在 ⓪+①（链接 + 读授权，不拿走正文）。Aspect 从 ② 才感知。

## 核心理念

> 别把 git 已经会的东西重新发明成协议。⓪ 就是 Snapshot/git。① 是组合平面，不是文件仓、不是知识协议。协议在 ② 补 git 不提供的身份、来源、写边界与外部访问声明；③ 是可丢派生。

- **身份**（RESOLVE，②）：`ObjectIdentity ≠ path`，身份在文件内容（frontmatter），Address = `object_id` + aspect + member。
- **来源**（GET_PROVENANCE，②）：精确 commit 坐标 + 各单元信封；不是 git log。
- **写**：`COMMIT`/`PROPOSAL` → Snapshot；State/Stream 是 Aspect Binding 的观察面，不是 Writer Surface。
- **目标 store**：`local/` FileGit + SQLite；`gitea/` 远程 Snapshot；`scale/` Dolt + ES/SR 派生。动态运行由上层产品实现，见 [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)。

### 概念与动词

| 概念 | 动词 | 输入 → 输出 |
|---|---|---|
| Object / Address | RESOLVE / READ / PUT / REMOVE | 见 Reader / Writer |
| 来源信封 | GET_PROVENANCE | 对象+commit → `ProvenanceTrace.chain` |
| 动态状态/事件流 | Aspect State/Stream Binding | 上层产品负责 search/window/cursor；Retrieval 返回 observation basis |
| WorkspaceDefinition | DEFINE_WORKSPACE / OPEN_WORKSPACE | 配方 → 一次命令内 `{仓 → commit}` |
| Object 历史 | LOG / DIFF | 对象+commit → `ObjectRevision[]`；两 commit → `ObjectDiff` |
| Schema 内省 | DESCRIBE_SCHEMA | 只暴露字段 `text/filter/sort` 逻辑访问语义 |
| 检索计划 | RetrievalPlan | ResolvedWorkspace + AccessSpec + provider capabilities → 每请求路由 |
| 工作投影 | SEARCH / describe-index | 当前增量编 SQLite/ES；命中回读完整 Canonical，目标补版本/evidence/completeness |
| 外部资源 | ResourceDescriptor | Agent 读取自包含访问句柄，再走全系统统一访问；默认不沉淀 |
| Proposal | propose / validateStructure / MERGE | 候选 Ref；结构检查后记录 PASSED/FAILED |
| 外部套件 | recordValidation | 只绑定传入的 PASSED/FAILED，不跑测试 |

## 结构

```text
kernel/             # ② Address / 来源；RepositoryID 名 Snapshot
repository/         # ⓪ Snapshot；② Knowledge + Aspect ValueSource/Binding
local/              # 本机 FileGit + SQLite
gitea/              # 远程 ⓪ Gitea Snapshot（无工作区）
scale/              # DoltRepository；ES/SR
catalog/            # ① 组合（见 catalog/README.md）
writer/             # COMMIT/PROPOSAL → Snapshot
reader/             # ② 读；③ SEARCH 入口
index/              # ③ 工作投影
controlplane/       # PROPOSAL → Preview → validate → Merge
gate/               # merge 证据清单
hook/               # CLI 出站 pre/post
connector/          # Collector 的 STATE Address 对账 helper
observability/      # principal/onBehalfOf、版本化访问账、Agent trace/反馈、派生 hitmap
cli/  cmd/kc/       # facade（命令表 command.go + 每组一个 verbs_*.go）
internal/
├── gitdir/         # git 目录 plumbing + commit 签名；⓪ 适配器与 ① 登记表共用
├── repofile/       # ② 磁盘单元格式（frontmatter + JSON body）；不是 store
├── journal/        # 本机过程账
├── jsonfile/       # 原子 JSON 落盘
├── testkit/        # T12 / Writer 契约与测试装置
└── arch/           # 分层守卫：把 docs/LAYERS.md 的 import 规则跑成测试
docs/
├── README.md
├── LAYERS.md
├── KNOWLEDGE_CATALOG_DESIGN.md
├── ASPECT_ACCESS.md
├── LIVE_MATERIALIZATION.md
├── PERMISSIONS.md
├── HOOKS.md
├── GATES.md
├── CONNECTORS.md
├── OBSERVABILITY.md
├── STORE_ADAPTERS.md
└── WALKTHROUGH_v5.1.md
```

## catalog/

`catalog/` 只做组合，不拥有对象内容。

- **WorkspaceDefinition** — 配方：哪些 repo、哪个 selector（通常是已发布分支）
- **ResolvedWorkspace** — 只钉 `{仓 → commit}`；动态 observation cut 由上层 Retrieval/Materialization 持有
- 消费读 / `object_id` 在 `reader.Serving`，不在 Catalog。`kc checkout --workspace` 是这次坐标的只读 grep 树（`layout.checkouts`），不是成员工作区

Writer 幂等日志是 `.kc/writer.json`。Catalog 当前态 `kc read --catalog`；历史看登记表 git（`kc audit`）。`.kc/system.jsonl` / `audit.jsonl` 是本机过程账；`.kc/access.jsonl` / `feedback.jsonl` 保存非 Canonical 的访问与反馈证据，hitmap 由其派生。见 [`docs/OBSERVABILITY.md`](docs/OBSERVABILITY.md)。`.kc` 只是本机 `kc` 找文件用的。文件怎么拆见 [`catalog/README.md`](catalog/README.md)。

## 运行

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go < 1.23
go test ./...
go run ./cmd/kc -- help   # 协议动词 CLI；默认工作区 ./.kc
go run ./cmd/kc -- serve --home /tmp/kc-demo   # HTTP facade + 本机操作台，http://127.0.0.1:7380/
```

```bash
kc init && kc repo-add --repo kr://acme/public/core
kc put --command-id sync-1 --repo kr://acme/public/core --object ETLTask:job-1 --aspect io --value '{"inputs":[]}'
kc define-workspace --workspace agent --revision 1 --source kr://acme/public/core=refs/heads/main
kc read --catalog
kc read --workspace agent --object ETLTask:job-1
kc audit
kc log --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
kc serve --home .kc   # 同一套动词的 HTTP facade；GET / 是操作台
# 共享服务可验证 Gitea 登录；调用方带 Authorization，主体变为稳定的 gitea:<user-id>
kc serve --home .kc --auth gitea --auth-url https://git.acme.example --auth-admin gitea:1
```

## Conformance

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 | 已退役：底座没有 APPEND/Stream surface；state/stream 通过 Aspect Binding 声明 |
| T6 FileGit Store | object_id、移动、CAS、GET_PROVENANCE、pinned tree read、DERIVATION 约束、Aspect 独立单元 |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation |
| T8 Embedded Reader | 可重建投影定位 + Canonical 回读；非权威、basis/lag；AspectSelector 可裁 permissions |
| T9 Maintenance Loop | 完整多 Repo Preview、validateStructure、Validation basis、Merge 后下次 `read --workspace` 可见 |
| T10 Refine | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup |
| T11 Catalog | Workspace Registry（含 git）、故障传播、来源不覆盖、跟已发布分支 |
| T12 Repository Contract | Snapshot 身份、CAS、LOG/DIFF、REMOVE、Merge、Archive、Writer 幂等 / schema_ref / PROPOSAL |
| Hook / Gate | pre 非 0 无 commit；REPLAYED 不打 hook；post 只含指针；缺 suite 不能 merge；Preview 变了旧 PASSED 作废 |
| Collector helper | `patch` 不误删；`reconcile` 只在 Observed∩Scope 上 REMOVE；超 Scope 拒绝；预览可 COMMIT |
| End-to-end journey | `cli/user_journey_test.go`：从空 Home 建 Catalog / Repo / Workspace，经 HTTP 读写、proposal、权限和生命周期走通通用用户路径 |
| Layering | `internal/arch`：`docs/LAYERS.md` 的 import 规则跑成断言。① 不得（含传递）依赖 `reader`/`index`/`local`；② 不得依赖 ③；`hook`/`gate`/`connector` 不得依赖协议包 |
| CLI surface | `cli/command_test.go`：Help 与命令表双向对齐；退役动词仍报替代品；stage 归属（governed 需要工作区、home 级动词不需要）；`--limit` 全动词一致拒绝非法值 |

## 文档

- [`docs/README.md`](docs/README.md)：文档职责地图；设计、操作和验证信息分别由哪里维护
- [`docs/KNOWLEDGE_CATALOG_DESIGN.md`](docs/KNOWLEDGE_CATALOG_DESIGN.md)：问题、第一性原理、调研与核心 ADR/K 决策；具体协议看代码和包 README
- [`docs/ASPECT_ACCESS.md`](docs/ASPECT_ACCESS.md)：Aspect 写单元 vs 读/检索形态（业界对照与决策）
- [`docs/LIVE_MATERIALIZATION.md`](docs/LIVE_MATERIALIZATION.md)：Aspect State/Stream Binding、外部 Materialization Runtime、统一检索与学术对照
- [`docs/PERMISSIONS.md`](docs/PERMISSIONS.md)：权限模型——按仓隔离、`kc allow` 发权；GRANT 快照是知识，强制在源系统
- [`docs/HOOKS.md`](docs/HOOKS.md)：出站接用户系统（`kc` 动词 × pre/post）
- [`docs/GATES.md`](docs/GATES.md)：`merge` 的证据清单（不是 hook）
- [`docs/CONNECTORS.md`](docs/CONNECTORS.md)：外部访问声明、Collector 与 integration runtime 边界
- [`docs/OBSERVABILITY.md`](docs/OBSERVABILITY.md)：`principal` / `onBehalfOf`、版本化访问账、Agent trace/反馈与派生 hitmap
- [`hook/README.md`](hook/README.md)：`hook/` 目录——出站 dispatch / exec / HTTP / outbox
- [`gate/README.md`](gate/README.md)：`gate/` 目录——`Check` 与 `.kc/gates.json`
- [`connector/README.md`](connector/README.md)：`connector/` 目录——Collector 的 Address 级对账 helper
- [`catalog/README.md`](catalog/README.md)：`catalog/` 目录——Workspace 配方、ResolveWorkspace、Registry、CLI
- [`writer/README.md`](writer/README.md)：`writer/` 目录——Snapshot Surface、幂等、ChangeSet 与 Aspect Binding 声明
- [`reader/README.md`](reader/README.md)：`reader/` 目录——精确读、历史三问、Projection、Refine、GroundingCitation
- [`docs/WALKTHROUGH_v5.1.md`](docs/WALKTHROUGH_v5.1.md)：用 `kc` 命令走通全流程（每步：操作 → 进入的状态）
- [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)：Snapshot Store 与检索派生介质；State/Stream 引擎属于上层产品
- 数仓立项、公司工作台与真实源验证只在 `scene/data-warehouse` 分支的 `validation/` 中维护
- 具体类型、CLI 参数、实现状态与历史不在设计文档里重复维护

## Store 扩展

权威仓实现 Snapshot `Repository` 并通过 T12（FileGit、Dolt、Gitea）。检索引擎实现 `Retriever` / `ProjectionMaintainer`（本地 SQLite / 规模化 Elasticsearch，列投影可用 StarRocks）。不要把 ES、SR、Iceberg 或外部动态 runtime 当 Repository 权威，也不要给 `Repository` 加动态运行或索引方法。见 [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)。
