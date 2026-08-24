# Agent 须知

这是 **Knowledge Catalog 通用知识底座**：一套 Catalog 协议的 **Go 参考实现**（身份、来源、写边界、Workspace 组合、维护闭环）。不是检索应用，也不是某个开源元数据产品的 fork。

第一落地场景是 **数仓域**（物理表/列/作业/血缘 + 语义层指标/维度）。场景检出挂在本仓库的 gitignored 隐藏目录里，同一 Cursor 工作区能看到两棵树，**不要另开窗口、不要切分支**。

## 同一工作区里的两棵树

`.scenes/` 已被 `.gitignore` 忽略，不要放进 `.cursorignore`（agent 必须能看见）。不要在两个 worktree 检出同一分支。场景树可能仍有旧 TypeScript 拷贝；协议只在仓库根 Go 包。**改文件前先看路径**。

| 路径 | 分支 | 职责 |
|---|---|---|
| 仓库根（本目录） | `main` | 协议、store adapter、conformance |
| `.scenes/data-warehouse/` | `scene/data-warehouse` | 数仓域场景落地 |

发现缺口时：**判断归属 → 只改对应路径 → 在该树里提交 → 同步**。

- 协议/契约/Writer/Repository/Workspace/T1–T12 不够 → 改 **仓库根** 并提交，然后把 main **合入**场景（见下）。
- 采集、source key、Recipe、源消息翻译、本地数据 → 只改 **`.scenes/data-warehouse/`**，不回写根目录。
- 场景本地数据、schema 草稿、源表清单、决策记录只放 `.scenes/data-warehouse/.data/`；SR 连接在 `.env`。都已 ignore，不要提交。
- 数仓验证集配方在 `.scenes/data-warehouse/validation/fixtures/tpch-sf001/`（TPC-H SF0.01 + MySQL + Metric View 种子）。生成物进该树 `.data/tpch-sf001/`。墙外 connector / Skill / 消费 agent 在 `.scenes/data-warehouse/validation/`（`./validation/playbook.sh`）。不要在仓库根加 compose / 源库客户端。
- 物理层业界对照与决策：`.scenes/data-warehouse/.data/decisions/physical-layer-industry.md`。
- Catalog 产品对照（WeData / Unity / Google）与语义层切分：`.scenes/data-warehouse/.data/decisions/catalog-industry.md`。
- **Schema 是知识**，不是项目源码。正式形态是知识 Repository 里的 `schema/*` 对象（Writer COMMIT，可 RESOLVE/READ/GET_PROVENANCE）。未入库前只能临时放 `.data/`，禁止 `schemas/`、`src/schemas/` 或任何会进 git 的路径。
- 场景树里改了协议口（kernel / repository / catalog / writer / reader / index / controlplane / connector / conformance）→ 停手，把变更做到仓库根 Go 包再合入。不要在场景分支演进协议。
- 搜索或打开协议代码时，默认用仓库根 **Go 包**（`kernel/`、`repository/`、`catalog/`、`connector/` …）。只有任务明确是场景落地才进 `.scenes/`。不要在仓库根加回 `src/`。

同步方向只准 **main → scene**。随时合入用 merge（只合已提交的 main，工作区未提交的不算）：

```bash
git -C .scenes/data-warehouse merge main
```

底座不合并场景实现。不要对已共享的场景分支 rebase。场景侧其它 git 命令同样加 `-C .scenes/data-warehouse`。`git clean -fdx` 会删掉 `.scenes/`，不要对仓库根用 `-x`。

## 仓库根（main）可以改

```text
③ 检索派生     index/     IndexPlan / AccessHints / 命中后回读
② 知识内容     kernel Address、writer PUT、reader 拼装、connector 对账
① 组合         catalog/   仓引用 + Workspace 配方（selector）；一次命令内解 {仓 → commit}
⓪ 操作语义     repository.SnapshotStore = git；repository.Stream = 有序段（不是 git）
               local.FileGit + JSONLStream；gitea.Repository；scale.DoltRepository + OpenStream stub
```

完整表：`docs/LAYERS.md`。Store 介质梯子（权威/索引/缓存/投影）见 `docs/STORE_ADAPTERS.md`，不要和 ⓪–③ 混名。

```text
kernel/            ② 身份、Address（Aspect）、错误、来源信封、digest；RepositoryID 名的是 ⓪ Snapshot
repository/        ⓪ Snapshot / Stream 口；② Knowledge 解释；Store 收 SnapshotStore（普通 git 仓可挂），Knowledge 是可选能力；APPEND 走 Store 上流
local/             本机 ⓪：FileGit + JSONLStream；③ SQLite 索引；不要 Redis
gitea/             远程 ⓪ Snapshot：Gitea Git 对象 API + 分支 CAS（无工作区，1.26+）；token 走 KC_GITEA_TOKEN
scale/             规模化：DoltRepository（Snapshot 口）、Stream stub、ES 全文、SR 列索引 stub、Redis 仅缓存
index/             ③ 工作投影 Engine；经 Catalog.Hook 订阅（from→to，自己算 object_id）；不进核心包
catalog/           ① 组合平面：承认仓、Workspace、ResolveWorkspace（commit + AppendCuts）；不解知识协议
writer/            COMMIT/PROPOSAL → Snapshot；APPEND → Stream；PUT Aspect 是 ②
reader/            ② 精确读 / 拼装；消费 Serving（ResolveWorkspace 坐标）与 ③ IndexPlan
controlplane/      PROPOSAL → Preview → validate → Merge（Merge 查 gate）
gate/              merge 证据清单（纯 Check；不是 hook）
hook/              CLI 出站 pre/post（Writer/Catalog 不 import）
connector/         ② 入站 kit：外部权威 → Address 对账预览（Writer/Catalog/CLI 不 import）
cli/  cmd/kc/      facade（Writer / Reader / Catalog / ControlPlane + allow/hook/gate）
internal/gitdir    git 目录 plumbing + commit 签名/trailer；⓪ 适配器与 ① 登记表共用，不认识 object_id
internal/repofile  ② 磁盘单元格式（frontmatter + JSON body）；不是 store
internal/arch      分层守卫测试：把 LAYERS.md 的 import 规则跑成断言
docs/              设计、分层、Aspect 读策略、kc 走通
```

`writer.Ingest` / `Reconcile` 是 COMMIT 之上的薄编排，**不是**采集框架。Address 级源对账在 `connector/`（`docs/CONNECTORS.md`）。不要在 Writer 或 CLI 里长仓库/数仓 connector，也不要做成 `kc connector-run` 插件宿主。

分层不是口头约定：`go test ./internal/arch/` 断言 import 图。加依赖前先看那张表；要破规则就连表一起改，别只改代码。**间接违规也算**——① 曾经因为持有 `*local.FileGitRepository`，把 `reader`/`index` 拖进了依赖图。⓪ 和 ① 要共用一段机制时，下沉到 `internal/`，不要让 ① 认识 ⓪ 的实现类型。

CLI 按变化轴拆文件：`cli/command.go` 是唯一命令表（`stage` = 跑之前要准备多少工作区），`cli/verbs_{write,read,index,catalog,control,home,allow}.go` 一组一个文件，`cli/operand.go` 放跨动词共享的 flag 解析，`cli/help.go` 只放帮助文本。**加动词 = 表里加一项 + 对应 `verbs_*.go` 加一个函数**，不要改分发器，也不要在 `run.go` 里重新长 switch。`kc serve` 读同一张表，HTTP 与 CLI 的动词集合不会漂；`cli/command_test.go` 断言 Help 与表双向对齐。默认 ref 用 `repository.DefaultRef`，不要写字面量 `refs/heads/main`。

## 不要做

- 不要在仓库根加 `collectors/`、`src/`、`tests/scenarios/`、具体源系统客户端或业务故事包；这些只属于 `.scenes/data-warehouse/` 或墙外独立仓。`connector/` 只是对账 kit，不连源。跨层通用旅程放现有包的 `_test.go`，具体业务验收放 scene 的 `validation/`。
- 不要把 schema 写成项目文件。Schema 是知识对象，走 Writer；草稿只放 `.data/`。
- 不要把 `.scenes/` 提交进 git，也不要写进 `.cursorignore`。
- 不要为场景新增 Write Surface。采集输出仍是 ChangeSet 预览，经 Writer `commit` / `append`。
- 不要把路径、URN、文件名当成 `object_id`。`object_id` 在文件 frontmatter；源系统标识是 source key，映射表属于场景侧。
- 不要把知识协议写进 `catalog/`（`object_id`、Aspect、IndexPlan、event payload）。Catalog 只做组合与坐标；上层再包装。
- 不要把 Stream 登记成仓。APPEND 是 ⓪ 流；Catalog 只钉 cursor。不要 `repo-add --driver stream`。
- 不要把 Workspace 做成又一个 Repo，不要把 public 知识拷进 personal。用户看见的是 Workspace（每次任务把各仓 selector 解析为固定 pin）。
- 不要按 public/group/personal 覆盖联邦读结果。
- 不要把 Projection/FTS/Redis 当权威。索引只定位、命中后回读 Canonical。Redis 只可加速热尾，miss 必须回权威；不要把 Redis 当 snapshot Canonical 的前置 cache，也不要用它做 GT。`summary`/`stored` 是投影载荷，不是索引车道。
- 不要直写 git / 工作区文件来绕过 Writer。
- 不要新增通用 PATCH、跨 Repo 事务。一次命令内不得中途跟随 `latest`；跨命令重新解析 Workspace 的 selector。
- 不要把 Catalog 权限做成文件 ACL，也不要按 Ranger/Unity 表 GRANT 拆知识仓。按治理边界拆 `--repo`；`repo-add` / `define-workspace` 不发权；发权是 `kc allow`。`permissions` Aspect 是知识，不是 `kc read` 闸门，也不能放行 SELECT。见 `docs/PERMISSIONS.md`。
- 不要把 gate 做成一种 hook，也不要把场景套件跑进 `kc validate`。Hook 出站见 `docs/HOOKS.md`；gate 查钉死的 Preview，见 `docs/GATES.md`。入站镜像见 `docs/CONNECTORS.md`（对方调 Writer，不是 hook）。
- 不要提交、不要改 git config，除非用户明确要求。

## 协议要点

- Catalog 语义只有一套。公司级默认 **一间 Catalog + 多 Repo**；单 source 是 Workspace 成员数为 1。Catalog **不是**文件仓库（⓪ SnapshotStore），也 **不是**知识协议（②/③ 上层包装）。挂 git 是 ⓪+①；Aspect 从 ② 才感知；APPEND 是 ⓪ 流，Catalog 只冻结 `AppendCuts`。入侵检查见 `docs/LAYERS.md`。
- ① 依赖 Snapshot 坐标。Writer `COMMIT`/`PROPOSAL` 打 Snapshot；`APPEND` 打 Stream。② 的 `READ`/`PUT` 目前由 git 形 Snapshot 解析 frontmatter。消费方走 `ResolveWorkspace` + `reader.Open` / `kc read --workspace`，不传仓和 commit。一次命令开始时解 selector 与附属 stream cursor，**命令内冻结、不落盘**。`object_id` 从 ② 消费读和 ③ 索引才出现，不进 Catalog 配方 / Hook。`Repository` = SnapshotStore + Knowledge，不是「仓 = 流」；但 `Store.Add` 只要 SnapshotStore，`Knowledge` 是可选能力，需要 ② 的接缝上问 `Store.Knowledge`（见 `docs/COMPOSITION.md`）。派生介质：权威 / 索引 / 缓存 / 投影（`STORE_ADAPTERS.md`）。**两套目录**：`local/`（FileGit + JSONLStream + SQLite，不要 Redis）与 `scale/`（DoltRepository、Stream stub、ES、StarRocks stub、Redis 缓存）。远程 Snapshot：`gitea/`（无工作区，token 走 `KC_GITEA_TOKEN`）。**目标引擎（冻结）**：Snapshot = FileGit/Dolt/Gitea；APPEND = 有序段；列索引 = StarRocks；全文 = ES/SQLite FTS；热尾 = Redis。不要 `--driver mysql`。不要 `repo-add --driver stream`。Iceberg/SR 是 ③ 消费投影。见 `docs/STORE_ADAPTERS.md`。
- 写选唯一 target：`COMMIT`/`PROPOSAL` → Snapshot；`APPEND` → Stream。变更代数只有 PUT / REMOVE（②）。`PUT Aspect` 替换一个分区，不是通用 PATCH。带 `schema_ref` 的 PUT 必须在 target 仓解析到 `schema/*`，否则 `SCHEMA_REVISION_UNRESOLVED`。
- 唯一键是 Address：`object_id` + `aspectName` + `memberKey`。同一 `object_id` 可有多个 Aspect 文件。禁止把 Entity blob 和 Aspect 混在同一对象上。
- Reader：`READ(ref)` 拼装（可 `AspectSelector`）；`readAddress` 读单单元。检索字段默认来自 `schema/*` AccessHints（`DESCRIBE_SCHEMA`）：属性上声明检索面（`access[]` + `type`），不在 schema 上列 `EQ/IN/GT`。`SEARCH` 原子算子是查询用法，由 `AllowsOp` 推出（隐式 AND，见 `reader.SearchRequest`），不是 RQL。`AspectSelector` 可再裁。`permissions` 是 SOURCE 知识，与 `structure` 同构；GRANT 正文通常不声明 `text`。声明了 AccessHints 就进 IndexPlan。见 `docs/ASPECT_ACCESS.md`、`reader/README.md`。知识仓 ACL 默认整仓，动作用 `kc` 动词，见 `docs/PERMISSIONS.md`（`kc allow` / `--as` 已求值）。表 GRANT **强制**不进 `allow.json`；`define-workspace` 不发权。
- `expectedTargetCommit` 过期 → `NON_FAST_FORWARD`；同 `command_id` 异 digest → `IDEMPOTENCY_CONFLICT`。重试用同一 command_id；内容变了换新 id 并重做 diff。
- DERIVATION 必须带固定 `inputViewReadVersionRef` + algorithm，否则拒写。源同步标 `SOURCE`。
- `COMMIT` / `merge` 推知识仓 Ref。Agent 用 `read --workspace`：命令开始时解各 source 的已发布 selector，命令内冻结。不要自己跟仓 `HEAD` 中途换 commit。
- `GET_PROVENANCE` 返回该对象各单元上贴的来源信封，不爬 `sourceRefs`，也不等于 git log。
- `ResolveWorkspace` 把 WorkspaceDefinition 的 selector 各解析一次，得到命令内 `{仓 → commit}`，不落登记表。消费方看这次坐标：`kc resolve --workspace`（不带 `--object`）出 pin；带 `--object` 仍是 Resolution。拼装系统状态用 `kc inspect --workspace`（CatalogState + pin + IndexPlan + 各仓 **这次 pin** 上的 index）。错误信封一律 `{error:{code,message}}`。选码：缺 flag/形状/`fmt.Errorf` → `USAGE_INVALID`；对象/digest/cursor CAS → `PRECONDITION_FAILED`；Ref 被推走 → `NON_FAST_FORWARD`；未挂载不是 `TEMPORARY_UNAVAILABLE`（那是瞬时 I/O）。`WORKSPACE_INVALID` 表示 Workspace 配方或 pin 不能使用。`CheckResolved` / `validateStructure` 检查仓已挂载且 commit 存在。`recordValidation` 只绑定传入的 PASSED/FAILED，不跑测试套件。自定义门禁是 `gate-add --on merge --require`（`docs/GATES.md`），不是 hook（`docs/HOOKS.md`）。
- Catalog 改动的记录就是登记表 git（`Catalog.Log` / `kc audit`）。当前组合空间是 `kc read --catalog`（`DumpState`：catalogId / repositories / views），不是 git 历史，也不是 `status`（`status` 混本机 stores）。`--as` / `--request-id` / `ruleId` 写进这次 commit。不要另开 ops 流。知识写入的记录在那个 Repository 的 git 里。`.kc/system.jsonl` / `audit.jsonl` 是本机过程账。Writer 不能把 Catalog id 当 `--repo`。
- 索引在 **② 之上**（③），实现在 `index/`，不是仓内对象，也不是 Workspace 的库。介质上的权威/索引/缓存/投影见 `STORE_ADAPTERS.md`。一把索引对应一个 Snapshot + `(仓, basisCommit)` + 该仓 `schema/*`。live 跟着 AfterSnapshot；消费 SEARCH 用这次 `ResolveWorkspace` 解开的 commit，不回绕 live。不要每 Workspace 一张表。`IndexPlan` 是 Workspace 这次解析上各仓一份配方。Writer / Catalog 核心不 import `index/`；通知点用 `Catalog.Hook`（只 `AfterSnapshot`）。不要给 Snapshot 口加索引方法。
- `LOG` 返回对象引入各 digest 的 commit（后续未改该对象的 commit 不占一条）。消费面 `kc log --workspace --object` 钉在这次解开的坐标；登记表 git 是 `kc audit`。当前态是 `kc read --catalog`。`DIFF` 是两个 pinned commit 上的对象值（维护口）。`GET_PROVENANCE` 不是 git log。
- Catalog 操作口就是 `catalog.Catalog`。登记表落盘是 `catalog.Registry`，历史是 `Catalog.Log`。收场：`retire-workspace` / `archive-catalog`；仓用 `register`（`repo-add` 登记到默认 Catalog）。仓归档 `archive-repo`。`kc allow` / `--as` 求值 `.kc/allow.json`（不带 `--as` = 主人）。消费 allow 是 `read-workspace` + `--workspace`。出站 hook 见 `docs/HOOKS.md`；gate 查钉死的 Preview，见 `docs/GATES.md`。外部权威入站见 `docs/CONNECTORS.md`。HTTP facade 是 `kc serve`（`POST /v1/<动词>`，JSON 旗标，`X-Kc-As` → `--as`，`X-Kc-Request-Id` → `--request-id`；本机操作台 `GET /`）。跨进程幂等与 MCP 尚未实现。权限设计见 `docs/PERMISSIONS.md`。缺这些先问归属，再决定补 main 还是场景。

## 命令

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go 过旧
go test ./...
go run ./cmd/kc -- help
go run ./cmd/kc -- serve --home /tmp/kc-demo   # 浏览器打开 http://127.0.0.1:7380/
```

CLI（`cli/` + `cmd/kc`）是 facade：只谈 Writer / Reader / Catalog / ControlPlane；`index/` 经 Catalog.Hook 挂上，不进那四个核心包。`.kc` 是本机找 Registry / 知识仓目录用的，不是协议对象。登记表 git 在 `.kc/catalogs/<encoded-id>/`（`init --catalog acme/catalog` → `kr://acme/catalog`）；知识仓在 `.kc/repos/<encoded-id>/`。有哪些对象扫目录（Catalog 身份在 `catalog.yaml`，知识仓在 `git config kc.repositoryId`），不要 `workspace.json`。`--catalog` 选哪一间；仅一间时可省略。登记表不是 Workspace 成员，不要 `repo-add`。Catalog 当前态是 `kc read --catalog`；历史是这份登记表 git（`kc audit`）。`.kc/audit.jsonl` / `system.jsonl` 是本机过程账。Writer 幂等日志是 `.kc/writer.json`。本机目录是 `.kc/layout.yaml`（repos / catalogs / projections / checkouts），引擎与托管 host 是 `.kc/stores.yaml`（`kc store-set --profile local|scale` / `store-ls`）；密码只走 `KC_REDIS_PASSWORD` / `KC_ELASTICSEARCH_PASSWORD`（或 `KC_ELASTICSEARCH_API_KEY`）/ `KC_STARROCKS_PASSWORD`。local profile 拒绝 Redis。`kc allow` / `--as` 求值 `.kc/allow.json`。`kc hook-*` 出站（`.kc/hooks.json`）；`kc gate-*` 是 merge 清单（`.kc/gates.json`）。`kc serve` 是同一套动词的 HTTP facade（`POST /v1/<verb>`，`X-Kc-As` → `--as`，`X-Kc-Request-Id` → `--request-id`，`--home` 钉在进程上）。消费读 `read --workspace` = `ResolveWorkspace` + `reader.Open`（命令内解 selector 与 AppendCuts）。`kc checkout --workspace` 遇 mount 配方把这次坐标落成可写 git worktree 树（`--to`，默认 `layout.checkouts/<workspace>`）；联邦读配方仍是只读 grep 树（路径 = 仓 / `object_id`）。`kc commit --workspace` 把 worktree 脏文件按 mount 走 `Writer.RawWrite`。`kc stream --workspace` 用这次钉的 cursor。`kc validate` 跑结构检查；`record-validation` 只记录外部套件结果。不要把采集器写进这个 CLI；入站镜像走 `commit --changeset` / `POST /v1/commit`（`docs/CONNECTORS.md`）。

用 `.venv` 跑 Python。协议代码是 Go（1.23+）。投影是可重建内存索引，命中后回读 Canonical；不要把它当权威。

## 文档

- `README.md` — 结构与 conformance 表
- `docs/LAYERS.md` — 协议分层 ⓪–③（git/流、Catalog、Aspect、索引各在哪层感知）
- `docs/COMPOSITION.md` — Loom：多仓组合层（底座）。目标形态、mount 路径布局与写回路由、场景、业界对照、当前实现差距。mount 配方的便携文件是成员仓根 `.kc-workspace.yaml`（跟着 git 走）
- `docs/KNOWLEDGE_CATALOG_DESIGN.md` — 设计与 K-01..K-24；读协议见第 7 章；分层见第 0.15 节
- `docs/ASPECT_ACCESS.md` — Aspect 读/检索业界对照与决策
- `docs/PERMISSIONS.md` — 权限：按仓隔离、`define-workspace` 组合、`kc allow` 发权；GRANT 快照是知识，强制在源系统
- `docs/HOOKS.md` — 出站：在 `kc` 动词 pre/post 调用户系统
- `docs/GATES.md` — `merge` 的证据清单；不是 hook
- `docs/CONNECTORS.md` — 入站：外部权威、感知→拉当前态、Address 对账 kit
- `docs/WALKTHROUGH_v5.1.md` — 用 `kc` 走通：操作与进入的状态
- `.scenes/data-warehouse/validation/docs/WALKTHROUGH_WORKBENCH.md` — 数仓公司工作台逐步实跑
- `docs/STORE_ADAPTERS.md` — 介质梯子：`local/` vs `scale/`；与 ⓪–③ 的关系见 `LAYERS.md`
