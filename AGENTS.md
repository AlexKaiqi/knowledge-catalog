# Agent 须知

这是 **Knowledge Catalog 通用知识底座**：一套 Catalog 协议的 **Go 参考实现**（身份、来源、写边界、Workspace 组合、维护闭环）。不是检索应用，也不是某个开源元数据产品的 fork。

第一批协议验证使用 **数仓知识提供方**（物理表/列/加工/血缘 + 语义层指标/维度），但它不是本项目的产品场景、分支或定制实现。

## 本地知识提供方夹具

数仓材料临时放在 gitignored 的 `.data/data-warehouse/`，身份接近黑盒测试夹具。稳定后可以整体迁移为独立 integration repo；仓库根不维护 `scene/data-warehouse` 分支、嵌套 worktree 或 main→scene 同步流程。

```text
.data/data-warehouse/
├── definitions/   实体、Aspect、关系的入库前草稿
├── connectors/    源系统采集、source key 与 ChangeSet 翻译
├── fixtures/      源输入与固定 oracle
├── tests/         只经 kc 公开 surface 验收
└── runs/          kc home、preview、运行证据等一次性生成物
```

- 协议/契约/Writer/Repository/Workspace/T1–T12 的缺口只改仓库根。
- 数仓实体、Aspect、关系、source key、源消息翻译和 Connector 只放 `.data/data-warehouse/`；不要把它们做成底座包、CLI 动词或运行宿主。
- 本地夹具不得复制 `kernel/`、`snapshot/`、`knowledge/`、`catalog/`、`knowledge/writer/`、`knowledge/reader/` 等底座实现。测试应构建或调用仓库根的 `kc`，从公开 surface 观察结果。
- **Schema 是知识**，不是项目源码。正式形态是知识 Repository 里的 `schema/*` 对象（Writer COMMIT，可 RESOLVE/READ/GET_PROVENANCE）；`.data/data-warehouse/definitions/` 只是入库前草稿。
- `.data/` 不提交。删除或清理前先判断其中是否有尚未迁出的领域决定、Connector 或测试证据；不要对仓库根使用 `git clean -fdx`。
- 搜索或打开协议代码时，使用仓库根 Go 包（`snapshot/`、`knowledge/`、`catalog/`、`connector/` …），不要在仓库根加回 `src/`。

## 仓库根（main）可以改

```text
③ 检索派生     index/ + retrieval/     AccessSpec / Retriever / ProjectionMaintainer / 命中后回读
② 知识内容     knowledge Address/Binding、writer PUT、reader 拼装、自包含 ResourceDescriptor
① 组合         catalog/   仓引用 + Workspace 配方（selector）；一次命令内解 {仓 → commit}
⓪ 操作语义     snapshot.Store；snapshot/filegit、gitea、dolt adapters
```

完整表：`docs/LAYERS.md`。Store 介质梯子（权威/索引/缓存/投影）见 `docs/STORE_ADAPTERS.md`，不要和 ⓪–③ 混名。

```text
kernel/            无依赖基础类型；⓪/① 对②类型的使用由 internal/arch 禁止
snapshot/          ⓪ Store / TreeStore / ref / CAS / Advanced；不认识 object_id
snapshot/filegit/  本机 Git authority；实现可选 Knowledge capability
snapshot/gitea/    远程 Gitea authority（无工作区，1.26+）；token 走 KC_GITEA_TOKEN
snapshot/dolt/     规模化 Dolt authority
knowledge/         ② Address / Aspect / ValueSource / Binding / ChangeSet / Repository
knowledge/writer/  ② COMMIT/PROPOSAL；PUT Aspect，可声明 Binding 但不调用 runtime
knowledge/reader/  ② 精确读 / 拼装 / ResolveBinding
index/             ③ 工作投影 Engine；经 Catalog.Hook 订阅（from→to，自己算 object_id）；不进核心包
retrieval/         ③ AccessSpec / SearchResult / Refine + OpenSearch provider
catalog/           ① 组合平面：承认仓、Workspace、ResolveWorkspace（只含 commit）；不解知识协议
snapshot/treewriter/ ⓪ 字面路径提交与 RAW_WRITE；不认识 Address / Aspect
controlplane/      PROPOSAL → Preview → validate → Merge（Merge 查 gate）
gate/              merge 证据清单（纯 Check；不是 hook）
hook/              CLI 出站 pre/post（Writer/Catalog 不 import）
connector/         Collector 的 STATE Address 对账 helper；不连源、不持 Writer
observability/     横切过程证据：principal/onBehalfOf、版本化访问账、Agent trace/反馈、派生 hitmap；不是 Canonical
workspacefs/       Linux go-fuse 宿主投影；只消费应用层固定文件计划，不拥有协议语义
cli/  cmd/kc/      facade（Writer / Reader / Catalog / ControlPlane + allow/hook/gate）
      cmd/kcfs/     本机多目录只读 mount 进程；不进入 kc serve 动词表
internal/gitdir    git 目录 plumbing + commit 签名/trailer；⓪ 适配器与 ① 登记表共用，不认识 object_id
internal/repofile  ② 磁盘单元格式（frontmatter + JSON body）；不是 store
snapshot/commandlog 跨写面的 command_id 重放/冲突机制；不拥有 Surface 语义
internal/arch      分层守卫测试：把 LAYERS.md 的 import 规则跑成断言
docs/              设计、分层、Aspect 读策略、kc 走通
```

`writer.Ingest` / `Reconcile` 是 COMMIT 之上的薄编排，**不是**采集框架。墙外 Collector 可用 `connector.Preview` 做 STATE Address 级对账（`docs/CONNECTORS.md`）。业务共建的 integration repo 与运行环境在墙外或场景侧；不要在 Writer 或 CLI 里长源实现或插件宿主。

分层不是口头约定：`go test ./internal/arch/` 断言 import 图与关键语义使用。加依赖前先看那张表；要破规则就连表一起改，别只改代码。**间接违规也算**——① 曾经因为持有具体 FileGit adapter，把 `reader`/`index` 拖进依赖图。⓪ 和 ① 要共用一段机制时，下沉到 `internal/`，不要让 ① 认识 ⓪ 的实现类型。

CLI 按变化轴拆文件：`cli/command.go` 是唯一命令表（`stage` = 跑之前要准备多少工作区），`cli/verbs_{write,read,index,catalog,control,home,allow}.go` 一组一个文件，`cli/operand.go` 放跨动词共享的 flag 解析，`cli/help.go` 只放帮助文本。**加动词 = 表里加一项 + 对应 `verbs_*.go` 加一个函数**，不要改分发器，也不要在 `run.go` 里重新长 switch。`kc serve` 读同一张表，HTTP 与 CLI 的动词集合不会漂；`cli/command_test.go` 断言 Help 与表双向对齐。默认 ref 用 `snapshot.DefaultRef`，不要写字面量 `refs/heads/main`。

术语以 `docs/TERMINOLOGY.md` 为准。Repository 接入使用 `kc repo-add`，宿主文件系统挂载只使用 `kcfs mount`；不要恢复含义冲突的 `kc mount`。固定 Workspace 坐标叫 `ResolvedWorkspace`/pin；远程消费仍逐请求认证授权，不增加 WorkspaceSession/sessionId；检索观察 basis 叫 `SearchView`。

## 不要做

- 不要在仓库根加 `collectors/`、`src/`、`tests/scenarios/`、具体源系统客户端或业务故事包；临时内容放 `.data/data-warehouse/`，稳定后迁到墙外 integration repo。`connector/` 只放 Collector 对账 helper，不放源实现、凭证、网络或运行宿主。跨层通用旅程放现有包的 `_test.go`，具体业务验收属于提供方夹具。
- 不要把 schema 写成项目文件。Schema 是知识对象，走 Writer；草稿只放 `.data/`。
- 不要重新建立 `.scenes/`、场景分支或底座代码副本。
- 不要为场景新增 Write Surface。采集输出仍是 ChangeSet 预览，经 Writer `commit` / `append`。
- 不要把路径、URN、文件名当成 `object_id`。`object_id` 在文件 frontmatter；源系统标识是 source key，映射表属于知识提供方。
- 不要把知识协议写进 `catalog/`（`object_id`、Aspect、Binding、AccessSpec）。Catalog 只做组合与 Snapshot 坐标；上层再包装。
- 不要把动态 state/stream 登记成仓或塞进 Workspace pin；底座没有 APPEND Surface。
- 不要把 live 外部资源伪装成 `snapshot.Store`。Aspect 可直接内联 Binding，也可引用同一 commit 的 `ResourceDescriptor`；两者只保存稳定访问声明，不保存 live 内容。身份、授权和调用 trace 走全系统统一能力。访问默认不沉淀；Collector 更新知识仍显式走 COMMIT。
- 不要把 Workspace 做成又一个 Repo，不要把 public 知识拷进 personal。用户看见的是 Workspace（每次任务把各仓 selector 解析为固定 pin）。
- 不要按 public/group/personal 覆盖联邦读结果。
- 不要把 Projection/FTS 当权威。Provider 只返回 CandidateRef，命中后在同一 basis 回读 Canonical；Schema 不得声明 `summary/stored/key` 或物理引擎词。
- 不要直写 git / 工作区文件来绕过 Writer。
- 不要新增通用 PATCH、跨 Repo 事务。一次命令内不得中途跟随 `latest`；跨命令重新解析 Workspace 的 selector。
- 不要把 Catalog 权限做成文件 ACL，也不要按 Ranger/Unity 表 GRANT 拆知识仓。按治理边界拆 `--repo`；`repo-add` / `define-workspace` 不发权；发权是 `kc allow`。`permissions` Aspect 是知识，不是 `kc read` 闸门，也不能放行 SELECT。见 `docs/PERMISSIONS.md`。
- 不要把 gate 做成一种 hook，也不要把场景套件跑进 `kc validate`。Hook 出站见 `docs/HOOKS.md`；gate 查钉死的 Preview，见 `docs/GATES.md`。外部资源访问与显式同步/捕获见 `docs/CONNECTORS.md`（墙外编排调 Writer，不是 hook）。
- 不要提交、不要改 git config，除非用户明确要求。

## 协议要点

- Catalog 语义只有一套。公司级默认 **一间 Catalog + 多 Repo**；单 source 是 Workspace 成员数为 1。Catalog **不是**文件仓库（⓪ `snapshot.Store`），也 **不是**知识协议（②/③ 上层包装）。Aspect/Binding 从 ② 才感知；Catalog pin 只冻结 `{repo → commit}`。入侵检查见 `docs/LAYERS.md`。
- ① 只依赖 Snapshot 坐标。Writer `COMMIT`/`PROPOSAL` 打 Snapshot；② 的 `READ`/`PUT` 由 Snapshot 文件解释 frontmatter。消费方走 `ResolveWorkspace` + `reader.Open`，一次命令只解一次 selector，**命令内冻结、不落盘**。`object_id`、Aspect、Binding、AccessSpec 不进 Catalog。`snapshot.Registry.Add` 只要 `snapshot.Store`；需要②时由应用装配处用 `knowledge.Of` / `knowledge.Lookup` 显式取得 capability。FileGit/Dolt/Gitea 是 Snapshot authority；OpenSearch 是 Retrieval provider。见 `docs/STORE_ADAPTERS.md`。
- 写选唯一 target：`COMMIT`/`PROPOSAL` → Snapshot。变更代数只有 PUT / REMOVE（②）。`PUT Aspect` 替换一个分区，不是通用 PATCH；可携带 `value_source` 声明 Snapshot 或 Binding。带 `schema_ref` 的 PUT 必须在 target 仓解析到 `schema/*`，否则 `SCHEMA_REVISION_UNRESOLVED`。
- 唯一键是 Address：`object_id` + `aspectName` + `memberKey`。同一 `object_id` 可有多个 Aspect 文件。禁止把 Entity blob 和 Aspect 混在同一对象上。
- Reader：`READ(ref)` 拼装（可 `AspectSelector`）；`readAddress` 读单单元；`ResolveBinding` 只解析固定声明，不调用 runtime。检索字段来自 `schema/*` 的 `text/filter/sort + type`，完整身份是 `(schema, aspect, path)`；裸 path 有歧义必须拒绝。MATCH 有 AllTerms/AnyTerms/Phrase；filter 推出 typed EQ/IN/NEQ/EXISTS/MISSING、number/time range 和 string PREFIX。Provider 逐 clause Probe，候选回读同 basis Canonical；公开 continuation 绑定 query/SearchView/projection。OpenSearch 只覆盖其如实声明的算子；未配置 provider 时 SEARCH 明确失败。见 `docs/ASPECT_ACCESS.md`、`knowledge/reader/README.md`、`retrieval/README.md`。
- `expectedTargetCommit` 过期 → `NON_FAST_FORWARD`；同 `command_id` 异 digest → `IDEMPOTENCY_CONFLICT`。重试用同一 command_id；内容变了换新 id 并重做 diff。
- DERIVATION 必须带固定 `inputViewReadVersionRef` + algorithm，否则拒写。源同步标 `SOURCE`。
- `COMMIT` / `merge` 推知识仓 Ref。Agent 用 `read --workspace`：命令开始时解各 source 的已发布 selector，命令内冻结。不要自己跟仓 `HEAD` 中途换 commit。
- `GET_PROVENANCE` 返回该对象各单元上贴的来源信封，不爬 `sourceRefs`，也不等于 git log。
- `ResolveWorkspace` 把每个 selector 解析一次，得到命令内 `{仓 → commit}`。`kc resolve --workspace` 出 pin；`kc inspect --workspace` 拼 CatalogState + pin + AccessPlan + 各仓该 pin 上的 index。错误信封统一 `{error:{code,message}}`。形状错误 → `USAGE_INVALID`；对象/digest CAS → `PRECONDITION_FAILED`；Ref 被推走 → `NON_FAST_FORWARD`；瞬时 I/O 才是 `TEMPORARY_UNAVAILABLE`。`CheckResolved` / `validateStructure` 只检查仓和 commit。`recordValidation` 只绑定外部 PASSED/FAILED。
- Catalog 改动的记录就是登记表 git（`Catalog.Log` / `kc audit`）。当前组合空间是 `kc read --catalog`（`DumpState`：catalogId / repositories / workspaces），不是 git 历史，也不是 `status`（`status` 混本机 stores）。`--as` / `--request-id` / `ruleId` 写进这次 commit。不要另开 ops 流。知识写入的记录在那个 Repository 的 git 里。`.kc/system.jsonl` / `audit.jsonl` 是本机过程账；消费访问与反馈分别进 `.kc/access.jsonl` / `feedback.jsonl`，hitmap 只从版本化访问证据派生。Agent 代理用户时 `principal=Agent`、`onBehalfOf=用户`，见 `docs/OBSERVABILITY.md`。Writer 不能把 Catalog id 当 `--repo`。
- 索引在 **② 之上**（③），实现在 `index/`，不是仓内对象，也不是 Workspace 的库。一把物理投影对应 `(仓, basisCommit, provider, physicalDigest)`；Workspace 只给出本次 pin，`AccessPlan` 只做逻辑内省。live 跟着 AfterSnapshot；消费 SEARCH 用这次解开的 commit，不回绕 live。Writer / Catalog 核心不 import `index/`；通知点用 `Catalog.Hook`。不要给 Snapshot 口加索引方法。
- `LOG` 返回对象引入各 digest 的 commit（后续未改该对象的 commit 不占一条）。消费面 `kc log --workspace --object` 钉在这次解开的坐标；登记表 git 是 `kc audit`。当前态是 `kc read --catalog`。`DIFF` 是两个 pinned commit 上的对象值（维护口）。`GET_PROVENANCE` 不是 git log。
- Catalog 操作口就是 `catalog.Catalog`。登记表落盘是 `catalog.Registry`，历史是 `Catalog.Log`。收场：`retire-workspace` / `archive-catalog`；仓用 `register`（`repo-add` 登记到默认 Catalog）。仓归档 `archive-repo`。`kc allow` / `--as` 求值 `.kc/allow.json`（不带 `--as` = 主人）。消费 allow 是 `read-workspace` + `--workspace`。出站 hook 见 `docs/HOOKS.md`；gate 查钉死的 Preview，见 `docs/GATES.md`。外部权威入站见 `docs/CONNECTORS.md`。HTTP facade 是 `kc serve`（`POST /v1/<动词>`，JSON 旗标，`X-Kc-As` → `--as`，`X-Kc-Request-Id` → `--request-id`；本机操作台 `GET /`）。跨进程幂等与 MCP 尚未实现。权限设计见 `docs/PERMISSIONS.md`。缺这些先问归属，再决定补 main 还是场景。

## 命令

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go 过旧
make test                         # component + boundary + local E2E
make test-all                     # 再跑 Gitea / Dolt / OpenSearch / Linux FUSE
go run ./cmd/kc -- help
go run ./cmd/kc -- serve --home /tmp/kc-demo   # 浏览器打开 http://127.0.0.1:7380/
go run ./cmd/kcfs -- plan --home /tmp/kc-demo --workspace <id> --root <现有项目>
```

CLI（`cli/` + `cmd/kc`）是 facade：`index/` 经 Catalog.Hook 装配，不进核心包。登记表 git 在 `.kc/catalogs/<encoded-id>/`，知识仓在 `.kc/repos/<encoded-id>/`；登记表不是 Workspace 成员。Catalog 当前态是 `kc read --catalog`，历史是 `kc audit`。本机布局在 `.kc/layout.yaml`，引擎在 `.kc/stores.yaml`；`.kc/access.jsonl` / `feedback.jsonl` 保存版本化访问与反馈证据。密码只走 `KC_ELASTICSEARCH_PASSWORD` / `KC_ELASTICSEARCH_API_KEY` / `KC_GITEA_TOKEN`。`kc serve` 将 `X-Kc-As` / `X-Kc-On-Behalf-Of` 与 trace/span 头注入统一观测上下文；认证模式下 principal 和 onBehalfOf 必须由可信认证器注入。`kc resolve-binding` 返回声明，不调用墙外 runtime。Collector 要沉淀动态观察时只调用 `commit --changeset`。

用 `.venv` 跑 Python。协议代码是 Go（1.23+）。投影是可重建内存索引，命中后回读 Canonical；不要把它当权威。

## 文档

- `README.md` — 结构与 conformance 表
- `docs/TERMINOLOGY.md` — Repository、WorkspaceDefinition、ResolvedWorkspace、SearchView 的规范名称
- `docs/SERVICE_ARCHITECTURE.md` — Catalog Server、Knowledge Server、KC Client、Workspace File Gateway 与 Writer API
- `docs/LAYERS.md` — 协议分层 ⓪–③（git/流、Catalog、Aspect、索引各在哪层感知）
- `docs/COMPOSITION.md` — Loom：多仓组合层（底座）。目标形态、mount 路径布局与写回路由、场景、业界对照、当前实现差距。mount 配方的便携文件是成员仓根 `.kc-workspace.yaml`（跟着 git 走）
- `docs/KNOWLEDGE_CATALOG_DESIGN.md` — 设计与 K-01..K-24；读协议见第 7 章；分层见第 0.15 节
- `docs/ASPECT_ACCESS.md` — Aspect 读/检索业界对照与决策
- `docs/PERMISSIONS.md` — 权限：按仓隔离、`define-workspace` 组合、`kc allow` 发权；GRANT 快照是知识，强制在源系统
- `docs/HOOKS.md` — 出站：在 `kc` 动词 pre/post 调用户系统
- `docs/GATES.md` — `merge` 的证据清单；不是 hook
- `docs/CONNECTORS.md` — 入站：外部权威、感知→拉当前态、Address 对账 kit
- `docs/OBSERVABILITY.md` — principal/onBehalfOf、版本化访问账、Agent trace/反馈与派生 hitmap
- `docs/WALKTHROUGH_v5.1.md` — 用 `kc` 走通：操作与进入的状态
- `.data/data-warehouse/README.md` — 本地数仓知识提供方夹具的边界、内容和迁出条件（不提交）
- `docs/STORE_ADAPTERS.md` — 介质梯子：Snapshot authority vs Retrieval provider；与 ⓪–③ 的关系见 `LAYERS.md`
