# Knowledge Catalog — 通用知识底座（Go 参考实现）

面向 AI 知识底座的一套统一 Catalog 协议（Go 参考实现）。

## 当前 MVP

当前版本的**本地参考 MVP 合格**：接入方可以从空环境完成 Repository 接入、Schema/来源写入和权威读回；消费方可以发现 Workspace，在固定 pin 上完成 SEARCH、READ 和 GET_PROVENANCE。共享服务试点有条件可用，多实例生产服务尚未验收；完整边界和机器证据见 [`docs/MVP_ACCEPTANCE.md`](docs/MVP_ACCEPTANCE.md)。

先按角色进入，不必先读完整设计：

| 角色 | 最短闭环 | 入口 |
|---|---|---|
| 知识接入方 | `repo-add → put`（或 `ingest → commit`）`→ read --repo` | `kc help provider` |
| 知识消费方 | `read --catalog → resolve --workspace → search/read/provenance --pin` | `kc help consumer` |
| 治理方 | `define-workspace → allow → propose/validate/merge → audit` | `kc help governor` |

Workspace 是消费配方，不是写入前置条件；Schema 只在需要结构校验或 SEARCH 能力时进入接入闭环。下面再解释这些选择为什么成立。

**Catalog 语义只有一套**：身份、版本、来源、写边界、Workspace 组合、维护闭环、联邦读取。不同的是 store adapter。协议分层 ⓪–③（[`docs/LAYERS.md`](docs/LAYERS.md)；不要和介质梯子混名）：

```text
③ 检索派生     AccessSpec / RetrievalPlan / CandidateRef / 完整回读
M 访问物化      StateLookup 端口 + 外部 State / Stream runtime（上层产品）
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
- **目标 store**：`snapshot/filegit|gitea|dolt` 提供权威版本；`retrieval/opensearch` 提供可重建 Snapshot/State 派生。未配置检索时仍提供 Snapshot 精确 READ/VFS；Bound State READ 与动态字段 SEARCH 通过 `--resource-access-url` / `KC_RESOURCE_ACCESS_URL` 调用独立 runtime 服务。见 [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)。

### 概念与动词

| 概念 | 动词 | 输入 → 输出 |
|---|---|---|
| Object / Address | RESOLVE / READ / PUT / REMOVE | 见 Reader / Writer |
| 来源信封 | GET_PROVENANCE | 对象+commit → `ProvenanceTrace.chain` |
| 当前态/事件流/时序观测 | Aspect State/Stream Binding | 消费 READ 可 hydrate State；State 字段可进入独立动态投影并返回双 basis；Stream window、TTL/retention、cursor/checkpoint 由上层产品治理 |
| WorkspaceDefinition | DEFINE_WORKSPACE / ResolveWorkspace | 配方 → 一次命令内 `{仓 → commit}` |
| Object 历史 | LOG / DIFF | 对象+commit → `ObjectRevision[]`；两 commit → `ObjectDiff` |
| Schema 内省 | DESCRIBE_SCHEMA | 只暴露字段 `text/filter/sort` 逻辑访问语义 |
| 检索计划 | RetrievalPlan | ResolvedWorkspace + AccessSpec + provider capabilities → 每请求路由 |
| 工作投影 | SEARCH / describe-index | 仅 OpenSearch；本地未配置时 SEARCH 明确缺能力，Snapshot 精确 READ/VFS 不受影响 |
| 外部资源 | ResourceDescriptor | Agent 读取自包含访问句柄，再走全系统统一访问；默认不沉淀 |
| Proposal | propose / validateStructure / MERGE | 候选 Ref；结构检查后记录 PASSED/FAILED |
| 外部套件 | recordValidation | 只绑定传入的 PASSED/FAILED，不跑测试 |

## 结构

```text
kernel/             # 无依赖底座：错误、canonical digest、Repository/Commit 坐标
snapshot/           # ⓪ Store / TreeStore / ref / CAS / Advanced
├── filegit/        # 本机 Git Snapshot adapter
├── gitea/          # 远程 Gitea Snapshot adapter
├── dolt/           # 规模化 Dolt Snapshot adapter
├── commandlog/     # 跨写面的 command-id 重放/冲突机制
└── treewriter/     # 字面路径提交、CAS、RAW_WRITE
knowledge/          # ② Address / Aspect / Schema / Binding / ChangeSet / Repository
├── writer/         # Knowledge COMMIT / PROPOSAL
├── reader/         # 声明/快照精确读、拼装、固定 pin
└── serving/        # 消费逻辑 READ；State Binding hydrate + 双 basis
catalog/            # ① 组合（见 catalog/README.md）
index/              # ③ 工作投影控制器
retrieval/          # ③ AccessSpec / Search / Refine + 物理 provider
└── opensearch/
controlplane/       # PROPOSAL → Preview → validate → Merge
gate/               # merge 证据清单
hook/               # CLI 出站 pre/post
connector/          # Collector 的 STATE Address 对账 helper
observability/      # principal/onBehalfOf、版本化访问账、Agent trace/反馈、派生 hitmap
workspacefs/        # Linux go-fuse 宿主投影；只消费固定的应用层文件计划
cli/  cmd/kc/       # facade（命令表 command.go + 每组一个 verbs_*.go）
      cmd/kcfs/     # 本机多目录 mount 进程；不暴露为 HTTP 动词
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
├── PROJECTION_CONTROLLER.md
├── PERMISSIONS.md
├── HOOKS.md
├── GATES.md
├── CONNECTORS.md
├── OBSERVABILITY.md
├── SYSTEM_OBSERVABILITY.md
├── STORE_ADAPTERS.md
└── WALKTHROUGH_v5.1.md
```

## catalog/

`catalog/` 只做组合，不拥有对象内容。

- **WorkspaceDefinition** — 配方：哪些 repo、哪个 selector（通常是已发布分支）
- **ResolvedWorkspace** — 只钉 `{仓 → commit}`；动态 observation cut 由上层 Retrieval/Materialization 持有
- 消费读 / `object_id` 在 `reader.Serving`，不在 Catalog。无 mount 配方的 `kc checkout --workspace` 是固定坐标的只读 grep 树；mount 配方则物化可写成员 worktree，写回仍统一走 Writer
- Linux 上可用 `kcfs mount --workspace <id> --root <现有项目>` 把配方中的多个非根 `Path` 分别挂入同一用户工作区；用户、IDE、shell、`rg` 与 Agent 共用宿主 VFS。首版只读，目标 mountpoint 只需不存在或为空

Writer 幂等日志是 `.kc/writer.json`。Catalog 当前态 `kc read --catalog`；历史看登记表 git（`kc audit`）。`.kc/system.jsonl` / `audit.jsonl` 是本机过程账；`.kc/access.jsonl` / `feedback.jsonl` 保存非 Canonical 的访问与反馈证据，hitmap 由其派生。见 [`docs/OBSERVABILITY.md`](docs/OBSERVABILITY.md)。`.kc` 只是本机 `kc` 找文件用的。文件怎么拆见 [`catalog/README.md`](catalog/README.md)。

## 运行

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go < 1.23
make test                 # 临时 OpenSearch + component + boundary + local E2E
make test-state-runtime-e2e # 独立 Docker runtime + OpenSearch；HTTP index-sync/search 动态旅程
make test-plugin          # typed Agent tools、固定任务 pin、生命周期清理、build/package
make test-agent-e2e       # 真实付费模型：接入、治理、消费、审计、越权六角色
make test-agent-ux-e2e    # 真实付费模型：概念解释、入口选择和失败恢复语义
make test-all             # 再跑插件、Gitea / Dolt / OpenSearch / Linux FUSE
go run ./cmd/kc -- help   # 协议动词 CLI；默认工作区 ./.kc
go run ./cmd/kc -- serve --home /tmp/kc-demo   # 仅 API 的 HTTP facade，供 dsh-plugin / 服务客户端使用
dsh --profile dsh-loom                        # 人和 Agent 的产品入口
go run ./cmd/kcfs -- plan --home /tmp/kc-demo --workspace agent --root "$PWD"
./scripts/e2e-kcfs-docker.sh                   # Docker 内真实 Linux/FUSE 验收
# Linux + fuse3: 将 plan 改成 mount，进程存活期间提供多个只读宿主挂载
```

按角色进入可先用 `kc help consumer`、`kc help provider`、`kc help governor`；
三个角色帮助先给出同一套 Catalog/Repository/Workspace/pin 心智模型，再给最短
操作路径；`kc help` 保留完整协议表。

DSH Agent 使用 `dsh-plugin/` 时只需由宿主配置 `KC_CATALOG` 和
`KC_WORKSPACE`，随后可直接调用 `knowledge_read` / `knowledge_search`；无需先
Resolve 或调用初始化工具。对象和字段未知时分别使用 `knowledge_list` 与
`knowledge_schema`，所有 typed calls 在同一 DSH Agent 任务内自动复用一个 pin；
任务结束后插件释放本地上下文，不存在 KC `sessionId` 或服务端 Session Store。
随包 Skill 也直接回答概念、入口选择和失败恢复问题，不要求用户先知道命令名。
完整接入与无检索投影时的 VFS/`rg` 路径见
[`dsh-plugin/README.md`](dsh-plugin/README.md)。

```bash
kc init && kc repo-add --repo kr://acme/public/core
# Schema 是版本化知识；AccessHints 决定这份知识能否被 SEARCH 发现。
kc put --command-id schema-1 --repo kr://acme/public/core \
  --object schema/runbook.body \
  --value '{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}'
kc put --command-id sync-1 --repo kr://acme/public/core \
  --object runbook/payment-oncall --schema-ref schema/runbook.body \
  --value '{"body":"切换支付流量前先检查冻结窗口"}' \
  --origin-kind SOURCE --source-ref file:///source/runbooks/payment-oncall.md
kc define-workspace --workspace agent --revision 1 --source kr://acme/public/core=refs/heads/main
kc read --catalog
kc resolve --workspace agent > pin.json
kc read --workspace agent --pin pin.json --object runbook/payment-oncall
kc search --workspace agent --pin pin.json --query 冻结窗口
kc provenance --workspace agent --pin pin.json --object runbook/payment-oncall
kc audit
kc log --repo kr://acme/public/core --object runbook/payment-oncall --ref refs/heads/main
kc serve --home .kc   # 同一套动词的 API-only HTTP facade；/livez /readyz /metrics 是管理面
# 共享服务可验证 Gitea 登录；调用方带 Authorization，主体变为稳定的 gitea:<user-id>
kc serve --home .kc --auth gitea --auth-url https://git.acme.example --auth-admin gitea:1
```

上面三次消费复用同一份 `pin.json`，因此 READ / SEARCH / GET_PROVENANCE
回答的是同一组 Repository commit。若 SEARCH 返回 `CAPABILITY_UNSATISFIED`，先运行
`kc describe-access --workspace agent`：空 `fields` 表示还没有可用于该查询的
`schema/*` AccessHints；Projection 是否跟上再看 `kc inspect --workspace agent`。

## Conformance

自动化入口是 `scripts/testsuite.sh`，Make target 只是稳定别名：

| 组 | 命令 | 边界 |
|---|---|---|
| component | `make test-component` | 各 Go 组件单元测试、本地合同；live adapter 在 short 模式跳过 |
| boundary | `make test-boundary` | ⓪–③ import、类型归属、术语与 provider 边界 |
| e2e | `make test-e2e` | CLI、HTTP、Catalog 的公开旅程；结束时对账全部 `kc` 动词 |
| adapters | `make test-adapters` | 真实 Gitea、Dolt、OpenSearch |
| state-runtime | `make test-state-runtime-e2e` | 独立 Docker `resource-access/v1` runtime + OpenSearch；动态候选与 Snapshot 不变性 |
| docker | `make test-docker` | adapters + State runtime + Docker Linux/FUSE |
| all | `make test-all` | 上述全部；Docker 不可用即失败 |

普通开发跑 `make test`。不要用一次含隐式 skip 的 `go test ./...` 冒充完整外部适配器验收。

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 | 已退役：底座没有 APPEND/Stream surface；state/stream 通过 Aspect Binding 声明 |
| T6 FileGit Store | object_id、移动、CAS、GET_PROVENANCE、pinned tree read、DERIVATION 约束、Aspect 独立单元 |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation |
| T8 Retrieval Projection | `index/` 可重建投影定位 + Canonical 回读；非权威、basis/lag；`AspectSelector` 只裁显式 READ |
| T9 Maintenance Loop | 完整多 Repository Preview、validateStructure、Validation basis、Merge 后下次 `read --workspace` 可见 |
| T10 Refine | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup |
| T11 Catalog | Workspace Registry（含 git）、故障传播、来源不覆盖、跟已发布分支 |
| T12 Snapshot + Knowledge composition | Snapshot 身份/CAS/历史 + 上层 Reader/Writer 的 LOG/DIFF/REMOVE、幂等、schema_ref、PROPOSAL |
| Hook / Gate | pre 非 0 无 commit；REPLAYED 不打 hook；post 只含指针；缺 suite 不能 merge；Preview 变了旧 PASSED 作废 |
| Collector helper | `patch` 不误删；`reconcile` 只在 Observed∩Scope 上 REMOVE；超 Scope 拒绝；预览可 COMMIT |
| End-to-end journey | `cli/mvp_acceptance_test.go` 固定接入方/消费方最短闭环；`cli/user_journey_test.go` 再覆盖 HTTP 读写、proposal、权限和生命周期 |
| Layering | `internal/arch`：`docs/LAYERS.md` 的 import 与类型归属跑成断言。`catalog → snapshot`；②不得依赖③；Snapshot adapter 不得依赖 Knowledge/repofile/Retrieval；ObjectID/Address/Provenance 只能由 `knowledge` 声明 |
| CLI surface | `cli/command_test.go`：Help 与命令表双向对齐；退役动词仍报替代品；stage 归属（governed 需要工作区、home 级动词不需要）；`--limit` 全动词一致拒绝非法值 |

## 文档

- [`docs/README.md`](docs/README.md)：文档职责地图；设计、操作和验证信息分别由哪里维护
- [`docs/KNOWLEDGE_CATALOG_DESIGN.md`](docs/KNOWLEDGE_CATALOG_DESIGN.md)：问题、第一性原理、调研与核心 ADR/K 决策；具体协议看代码和包 README
- [`docs/TERMINOLOGY.md`](docs/TERMINOLOGY.md)：Repository、WorkspaceDefinition、ResolvedWorkspace、SearchView 等公开术语的唯一命名
- [`docs/SERVICE_ARCHITECTURE.md`](docs/SERVICE_ARCHITECTURE.md)：Catalog Server、Knowledge Server、统一 KC Client、远程 VFS 与 Writer API 的服务边界和落地顺序
- [`docs/ASPECT_ACCESS.md`](docs/ASPECT_ACCESS.md)：Aspect 写单元 vs 读/检索形态（业界对照与决策）
- [`docs/LIVE_MATERIALIZATION.md`](docs/LIVE_MATERIALIZATION.md)：Aspect State/Stream Binding、外部 Materialization Runtime、统一检索与学术对照
- [`docs/PROJECTION_CONTROLLER.md`](docs/PROJECTION_CONTROLLER.md)：Snapshot/Observation 统一投影控制、绑定后拼装、动态 State 索引与验收矩阵
- [`docs/PERMISSIONS.md`](docs/PERMISSIONS.md)：权限模型——按仓隔离、`kc allow` 发权；GRANT 快照是知识，强制在源系统
- [`docs/HOOKS.md`](docs/HOOKS.md)：出站接用户系统（`kc` 动词 × pre/post）
- [`docs/GATES.md`](docs/GATES.md)：`merge` 的证据清单（不是 hook）
- [`docs/CONNECTORS.md`](docs/CONNECTORS.md)：外部访问声明、Collector 与 integration runtime 边界
- [`docs/OBSERVABILITY.md`](docs/OBSERVABILITY.md)：`principal` / `onBehalfOf`、版本化访问账、Agent trace/反馈与派生 hitmap
- [`docs/SYSTEM_OBSERVABILITY.md`](docs/SYSTEM_OBSERVABILITY.md)：运行 metric/log/trace、传播、健康、SLI/SLO、采样与 Conformance
- [`hook/README.md`](hook/README.md)：`hook/` 目录——出站 dispatch / exec / HTTP / outbox
- [`gate/README.md`](gate/README.md)：`gate/` 目录——`Check` 与 `.kc/gates.json`
- [`connector/README.md`](connector/README.md)：`connector/` 目录——Collector 的 Address 级对账 helper
- [`catalog/README.md`](catalog/README.md)：`catalog/` 目录——Workspace 配方、ResolveWorkspace、Registry、CLI
- [`knowledge/writer/README.md`](knowledge/writer/README.md)：`knowledge/writer/` 目录——Knowledge Surface、ChangeSet 与 Aspect Binding 声明
- [`knowledge/reader/README.md`](knowledge/reader/README.md)：`knowledge/reader/` 目录——精确读、历史三问、Binding 与 GroundingCitation
- [`knowledge/serving/README.md`](knowledge/serving/README.md)：消费侧逻辑 READ——State Binding hydrate、双 basis 与 VFS 边界
- [`retrieval/README.md`](retrieval/README.md)：③ 逻辑检索合同、执行编排与物理 provider
- [`docs/WALKTHROUGH_v5.1.md`](docs/WALKTHROUGH_v5.1.md)：用 `kc` 命令走通全流程（每步：操作 → 进入的状态）
- [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)：Snapshot Store 与 Snapshot/State 检索派生介质；具体 State/Stream runtime 属于墙外产品
- 数仓领域定义、Connector 与真实源验证临时放在 gitignored 的 `.data/data-warehouse/`，稳定后迁为独立 integration repo
- 具体类型、CLI 参数、实现状态与历史不在设计文档里重复维护

## Store 扩展

权威 Adapter 实现 Snapshot capability，并与上层 Reader/Writer 组合通过 T12（FileGit、Dolt、Gitea）。检索引擎实现 `Retriever` / `ProjectionMaintainer`，当前唯一实现是 OpenSearch；协议和 CLI 检索验收也运行真实 OpenSearch。不要把 OpenSearch、分析投影或外部动态 runtime 当 Repository 权威，也不要给 Snapshot Store 加动态运行或索引方法。见 [`docs/STORE_ADAPTERS.md`](docs/STORE_ADAPTERS.md)。
