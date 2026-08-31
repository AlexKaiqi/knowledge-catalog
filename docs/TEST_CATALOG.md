# 底座验证目录：状态 × 操作

日期：2026-08-30

范围：仓库根协议（`kernel` / `catalog` / `writer` / `reader` / `controlplane` / `cli`）。

对照：[`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md)（操作→进入的状态）、`cli/mvp_acceptance_test.go`（接入/消费最短闭环）、`cli/user_journey_test.go`（通用端到端旅程）、T1–T12（`README.md` Conformance）。

本目录按当前 Snapshot + Aspect Binding + 声明式 AccessSpec 契约验收。旧 APPEND/Stream/AppendCuts surface 已退役，命令表与 HTTP 都必须拒绝它们。

这不是 TPC-H。数仓域验证材料位于受跟踪的 `.data/data-warehouse/` 黑盒 integration suite，属于墙外知识提供方。底座要用另一张图：任意知识仓在空 home 上，经过哪些状态、每个状态下哪些操作合法、失败必须落到哪个错误码。

补齐时：**判断归属 → 只改仓库根 → `go test` 能红能绿**。不要把数仓表名、Hive GRANT、compose 写进本目录的用例。

---

## 0. 怎么用

唯一自动化入口是仓库根 `scripts/testsuite.sh`：

```bash
make test           # 临时 OpenSearch + component + boundary + 应用/transport 合同
make quality        # gofmt/tidy/vet/staticcheck + 复杂度/文件体积/重复门禁
make test-e2e       # 共享应用语义 + typed Client/HTTP/Catalog 边界
make test-race      # command log / hook / Reader / Index / CLI 并发路径
make test-cover     # short suite + statement coverage 不回退门禁
make test-plugin    # DSH MountController、固定 pin、Skill 与 build/package
make test-agent-e2e # 真实模型六角色，严格检查 Skill/shell trace、状态 oracle 与权限边界
make test-agent-ux-e2e # 真实模型自然语言问答，检查概念/入口/恢复语义与 Skill-only trace
make test-service-e2e # Gitea + OpenSearch 下的 provider/consumer 双身份验收
make test-adapters  # Gitea + Dolt + OpenSearch
make test-docker    # adapters + 双角色 service E2E + Linux/FUSE
make test-all       # 全部；缺 Docker 或 live adapter 失败即红
```

`make test`、`make test-e2e`、race 与 coverage 组会启动一次性 OpenSearch；项目不保留第二套
检索语义。`testing.Short()` 只把 Gitea/Dolt 等其它 live authority 从快速组隔离；显式
adapter/Docker 组不得把环境缺失静默算作通过。命令覆盖由 CLI 测试进程实际记录调用结果，并在
`KC_ASSERT_E2E_COVERAGE=1` 时与唯一 `cliSurface` 命令表对账；每个公开命令必须至少被调用一次、
至少有一个通过 `body` 断言的成功场景。只读命令至少验证一个有意义的协议边界；按语义 action
识别的状态变更命令至少验证两个独立失败场景（例如形状/目标状态/授权），不用无价值的
unknown-flag 复制用例刷数。风险分级直接读取 `cliSurface` 的 action，新增别名或命令不能靠另一份
手工名单绕过门禁。无参数只读命令验证未授权枚举或身份形状。Help 只是展示文本，不作为覆盖分母。
这些逐命令测试使用 test-only embedded seam 验证 Server 与 transport 共用的应用服务，不将该 seam 暴露为产品调用方式。生产 `Run`、remote CLI 和 HTTP 测试另行证明：除 `kc local`、`kc serve` 外，无 Server 必须失败关闭。所有领域命令进入逐命令门禁；
四个 help 主题及未知主题恢复动作单独验证，serve 的 help/flag 边界由 Go 测试验证，真实监听、
ready 与优雅退出由 service/kcfs 进程级旅程验证。
需要逐命令审计时可运行
`KC_COMMAND_COVERAGE_REPORT=/tmp/kc-command-coverage.json make test-e2e`；报告区分调用数、成功运行数、
已断言成功场景、已断言失败场景、语义 action、风险要求及错误码；成功必须经过 `body` 的状态/JSON
验证，失败必须经过 `expectCode` / `expectMsg`，未断言调用不会伪装成验证证据。

Agent 验收也有显式分母：`dsh-plugin/scripts/agent-scenarios.json` 固定六个核心操作角色
（provider、governor、consumer、auditor、recovery、unauthorized）、四个首次使用/概念问答，
并登记 `DW-AGENT-01` 数仓 provider/consumer companion。两个核心 runner 启动前会把实现与清单逐项
对账；全量门禁必须生成每个场景的回答、trace、oracle 和汇总，过滤器只用于单场景调试。
核心角色通过宿主 shell 调公开 `kc` CLI 并使用人工注入的固定任务上下文，因此可在 macOS 运行；
真实 MountController、只读挂载和 FUSE 生命周期仍由 Linux Docker `make test-kcfs-e2e` 独立验收，
任何平台/能力缺失都不能在 Agent runner 中以成功码伪装为 PASS。

HTTP 使用独立分母：测试直接从三个生产 route registry 提取 57 条正式路由，不读取 CLI 命令表。
每条路由必须通过已声明 method 可达、未声明 method 返回 405，并且恰好有一个 transport 责任方：
44 条由 remote CLI typed-dispatch 合同拥有，其真实请求体还会回放到生产 handler 的严格 DTO
解码边界；13 条 HTTP/宿主专属入口由直接 handler 成功旅程拥有。领域成功/失败语义只在应用层
旅程验证一次，认证、固定 pin、Canonical 回读等高风险组合另做真实 HTTP E2E，不为每个 transport
机械复制整套领域用例。新增、删除或重复认领路由都会使门禁失败。`kcfs` 不伪装成领域 HTTP surface：help/plan/mount 及
daemon-mount/stop 控制器边界由 Go + Docker Linux/FUSE 验收。DSH 的 `/api/loom/vfs` 则单独覆盖
GET 列表/分页/预览、POST 偏好写入、只读与路径/游标/方法边界。

`make quality` 是防回退门禁，不把单一指标误当成设计结论：生产函数圈复杂度上限 50、Go 文件
上限 700 行、大段复制上限 150 token；`internal/testkit` 和测试文件不参与结构阈值，但仍参与
编译、`vet` 与 `staticcheck`。超过 30 圈或 500 行应在改动时人工复核职责是否需要拆分；覆盖率
继续由 `make test-cover` 的 55% statement gate 管理。阈值可通过对应 `KC_MAX_*` 环境变量收紧。

测试保留规则：

- 每条测试必须独占一种失败风险；同一属性只在拥有语义的最低层验证一次，除非 transport/provider 转换本身就是合同。
- E2E 只保留跨组件公开旅程、认证边界和固定 basis 等无法由组件测试证明的行为；参数解析、退役命令和纯形状错误用表驱动单测。
- 已退役能力的正路径测试直接删除，不用永久 `t.Skip` 伪装成证据；冻结能力只保留“公开入口明确拒绝”的反例。
- 新测试若不能说明“删掉后哪种真实回归会漏掉”，不进入套件。

每条用例四列：

| 列 | 含义 |
|---|---|
| 前置 | 系统已进入的状态（见 §1）。不是「随便有个 home」 |
| 操作 | 协议动词 + `kc` 命令（或 Go API）。一次只动一个 Surface |
| 预期 | 后态（哪一列变了）或错误码。只读操作写「状态不变」 |
| 现况 | `ok` 已有断言 / `partial` 有相关测试但没钉这一格 / `gap` 该补 / `frozen` 协议未实现，禁止当正路径测 |

观察点固定看这七列（推演里的四列 + 三条派生）：

```text
成员库 main / 候选 Ref     ⓪ Snapshot
Aspect declaration/version ② ValueSource / DeclarationDigest
Catalog 登记表             ① DumpState + 登记表 git
命令内 pin                 ① ResolveWorkspace，不落盘
Canonical 正文             ② READ / GET_PROVENANCE
ControlState               提案 / Preview / Validation（.kc/control.json）
工作投影                   ③ basis / lag；可丢
```

`--as` / hook / gate 是 facade，不进七列正文；失败时主状态必须不变。

---

## 1. 状态不是一条线

TPC-H 故事是线性的（空库 → 13 表 → 口径 merge）。底座是**正交维的乘积**。不要为每个组合各写一条；先覆盖维内跃迁，再覆盖下面标明的交叉格。

### 1.1 正交维

| 维 | 取值 | 谁改 |
|---|---|---|
| Home | 无 / 已 init | `init` |
| Catalog 生命周期 | 空登记表 → 已注册 Repository → 有 Workspace → Workspace 退役 → Catalog 归档 | `register` / `define-workspace` / `retire-workspace` / `archive-catalog` |
| 仓生命周期 | 未挂载 → 已挂载 root → 有 commit → 归档 | `repo-add` / `COMMIT` / `archive-repo` |
| Snapshot Ref | `main=root` → `main=U*` → 存在 candidate → merge 后 `main=C*` | `put`/`commit` / `propose` / `merge` |
| Binding | Snapshot value / inline state / inline stream / DescriptorRef / 非法声明 | `PUT Aspect --value-source`；Catalog 不感知 |
| Workspace 配方 | 无 / 单 source / 多 source / 同 `object_id` 多仓 | `define-workspace`（提高 revision） |
| 命令 pin | 无 Serving / 本次冻结 / 下次命令重解 | `ResolveWorkspace`；命令内不得跟 `latest` |
| 维护闭环 | 无 / 已 propose / 已 preview / PASSED\|FAILED / 已 merge | ControlPlane |
| 索引 | 空 / 跟 HEAD / lag / schema 触发 rebuild | `Catalog.Hook` `AfterSnapshot` |
| 授权 | 主人 / `--as` 命中 / `--as` 拒绝 | `kc admin grant add`（不改七列） |

### 1.2 典型正路径（补齐时按这条走通一遍）

```text
W0 无 home
 → init                         W1 空 Catalog（workspaces=[]，无仓）
 → repo-add                     W2 仓已挂，main=root
 → put / commit                 W3 Canonical 在 main=U1；Catalog 不变
 → define-workspace                  W4 配方已登记；read --workspace 立刻可读
 → propose                      W5 candidate=C1，main 仍 U1；read --workspace 仍旧值
 → preview + validate PASSED    W6 ControlState 有 Preview；登记表仍无 pin
 → merge                        W7 main=C1；下次 read --workspace 见新值
 → 再注册 Repository + define-workspace rev  W8 同 object_id 两条 FederatedValue，不覆盖
 → retire / archive             W9 Workspace 不可 Open；仓禁写；Catalog 禁 define；未归档仓仍可写
```

并行、不插入这条线：`resolve-binding`（只解析声明）、`search`/`describe-access`（命中回读这次 pin）、`--as`（拒绝则七列不动）。

### 1.3 和现有套件怎么对齐

| 现有 | 覆盖的是哪一段 | 不是什么 |
|---|---|---|
| `cli/write_flow_test.go` | W0→W3 的 CLI 动词 | 不含 Workspace / merge |
| `cli/read_flow_test.go` | W3 上维护读 | 不含 `--workspace` |
| `cli/consume_flow_test.go` | W4 消费口 + checkout | 不含提案 |
| `cli/mvp_acceptance_test.go` | 接入方/消费方最短 MVP 旅程 | 从空 Home 验证仓内发布、Catalog 发现、pin、SEARCH/READ/PROVENANCE |
| `cli/user_journey_test.go` | W1–W9 通用用户旅程 | 从空 Home 跨层验证，不绑定业务域 |
| T1–T12 | 不变量，不是状态机步骤 | 不代替「从 W5 merge」 |
| TPC-H graph canvas | 数仓域 S0–S8 | **不要**当底座覆盖率 |

---

## 2. 用例目录

状态栏写的是前置。`kc` 与 Go API 等价；CLI 未摊开的在预期里标明。

现况以 2026-08-23 仓库根 `go test` 为准。补齐后把 `gap`/`partial` 改成 `ok`，并填测试名。

### 2.1 W 工作区 / Store

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| W-01 | W0 | `kc local init --catalog acme/catalog` | 登记表 git 出生；`read --catalog` → `{catalogId, repositories:[], workspaces:[]}` | ok | `TestCatalogRepoWriteFlow` |
| W-02 | W1 | 再 `init` 同一 catalog | 幂等；不建第二间 | ok | 同上 |
| W-03 | W1 | `kc local init --catalog` 另一个 id | 拒绝（一间 home 先 `kc local catalog attach`） | ok | 同上 |
| W-04 | W1 | `kc local catalog attach --catalog kr://acme/docs/catalog` | 两间 Catalog；grant 不共享 | ok | `TestMultipleCatalogs` `TestCatalogIsolationDoesNotShareAllow` |
| W-05 | W1 | `repo-add --repo kr://acme/catalog` | 拒绝：登记表不是成员仓 | ok | `TestCatalogRepoWriteErrors` |
| W-06 | W1 | `repo-add --driver stream` / `--driver mysql` | 拒绝 | ok | `TestStoreConfigRejectsSecrets` |
| W-07 | W1 | `status` vs `read --catalog` vs `audit` | status=本机扫描；read=组合空间；audit=登记表 git | ok | `TestCatalogAuditIsGitLog` |
| W-08 | W0 | 业务命令无 `--server` / `KC_SERVER_URL` | `USAGE_INVALID`；不得回退为直开 `.kc` | ok | `TestProductCommandsRequireServer` |
| W-09 | W1 | 已移除的 redis/stream driver | unknown driver；不得写入配置 | ok | `TestStoreConfigRejectsSecrets` |
| W-10 | W1 | `store-set --profile scale` 后 `repo-add --driver dolt` | 本机 Dolt 仓；不是 mysql | ok | `TestScaleProfileRepoAddDolt` |

### 2.2 C 组合平面（①）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| C-01 | W1 | `register` 未挂载仓 | `unknown repository` | ok | write errors |
| C-02 | W2 | `define-workspace` 未挂载 source | `WORKSPACE_INVALID`；登记表无该 Workspace | ok | T11 / S0 |
| C-03 | W2 | `define-workspace` 同一 repo 出现两次 | `WORKSPACE_INVALID`（K-10） | ok | T11 |
| C-04 | W3 | `define-workspace` 合法 | 进入 W4；立刻 `OpenWorkspace` / `read --workspace` | ok | S2 / T11 |
| C-05 | W4 | `resolve --workspace`（无 `--object`） | pin 只有 `{仓→commit}`；不读正文、无动态 cut | ok | `TestConsumeViewFollowsPublishedBranch` |
| C-06 | W4 | `put` 再 COMMIT | **Catalog 不变**；main 前进；已打开的 pin 仍钉旧 commit | ok | S1 / `TestOpenedWorkspacePinDoesNotMoveWithLaterCommit` |
| C-07 | W4 | `retire-workspace` | `OpenWorkspace` → `WORKSPACE_INVALID`；其它 Workspace 仍可用 | ok | S6 / `TestWorkspaceAndCatalogLifecycle` |
| C-08 | W4 | `archive-repo` | 该仓 `COMMIT`/`PROPOSE` → `REPOSITORY_ARCHIVED`；新 OpenWorkspace 不选入 | ok | lifecycle / write errors / S6 |
| C-09 | W4 | `archive-catalog` | `define-workspace` → `CATALOG_ARCHIVED`；未归档成员仓仍可写 | ok | S6 |
| C-10 | W8 | `define-workspace` 提高 revision、改 sources | **下次** OpenWorkspace 用新配方；本次 pin 不变 | ok | S4 |
| C-11 | W4 | `CheckResolved` / `validate --preview` | 只检查 Snapshot 成员与 commit，不解析 Binding | ok | catalog/control tests |
| C-12 | W4 | `define-workspace --as steward --request-id …` | 登记表 git stamp 含 as / request-id / ruleId | ok | `TestCatalogGitStampsPrincipal` / F-03 HTTP evidence |
| C-13 | W4 | `audit --workspace` vs `log --workspace --object` | audit=配方历史；log=对象引入 commit | ok | consume_flow |
| C-14 | W3 | 无 Workspace 时 `read --workspace` | `WORKSPACE_INVALID` | ok | S0 / consume_flow |

### 2.3 K 快照写（COMMIT / PUT / REMOVE）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| K-01 | W2 | `put` 一条 Address | `main=U1`；Receipt `APPLIED`；Catalog 不变 | ok | write_flow / T6 |
| K-02 | W3 | 同 `command_id` + 同 digest | `REPLAYED`；HEAD 不变（K-18） | ok | T4 / S1 |
| K-03 | W3 | 同 `command_id` + 异内容 | `IDEMPOTENCY_CONFLICT`；HEAD 不变 | ok | T4 / write errors |
| K-04 | W3 | 过期 `expectedTargetCommit` | `NON_FAST_FORWARD`（K-06） | ok | T2 / S5 |
| K-05 | W3 | 空 ChangeSet | `USAGE_INVALID` | ok | T3 / write errors |
| K-06 | W3 | `originKind=DERIVATION` 无 VRV+algorithm | `PRECONDITION_FAILED`；Catalog 不变 | ok | S1 / write errors |
| K-07 | W3 | `schema_ref` 指向不存在的 `schema/*` | `SCHEMA_REVISION_UNRESOLVED` | ok | `knowledge/writer/schema_test.go` / S1 |
| K-08 | W2 | 同一 Changeset 先 PUT schema 再引用 | 接受 | ok | schema_test |
| K-09 | W3 | `schema_ref` 指向外仓 | 拒绝 `SCHEMA_REVISION_UNRESOLVED` | ok | schema_test |
| K-10 | W3 对象已在 | `--if-absent` | `PRECONDITION_FAILED`；HEAD 不变 | ok | S5 / write errors |
| K-11 | W3 | 再 PUT 同 `object_id`、换 `path-hint` | 身份不变；旧 commit 仍旧路径（T1 / K-04） | ok | T1 / S5 |
| K-12 | W3 | 先后 PUT 两个 Aspect | 拼装对象两分区独立；`readAddress` 单单元 | ok | T12 provider conformance |
| K-13 | W3 Entity blob 已在 | 再 PUT 同 id 的 Aspect | `OBJECT_ID_CONFLICT`；HEAD 不变 | ok | writer conformance |
| K-14 | W2 Tree fixture | 两文件同一 Address | 通用 Tree interpreter 拒绝重复 Address | frozen | provider-independent reader tests |
| K-15 | W2 | `kc writer ingest --dir` | 只出 ChangeSet 预览；frontmatter `object_id` 胜路径；报告身份/Schema/SEARCH readiness；既有 Schema 只报未验证、不越权探测；不 COMMIT | ok | T7 / `TestWritePath` / `TestIngestDoesNotProbeExistingSchema` |
| K-16 | ingest 预览 | `kc writer commit --changeset` | 与 K-01 同：只推进成员 Ref | ok | write_flow |
| K-17 | W3 | `remove` | 对象在新 commit 上 UNRESOLVED；旧 commit 仍可读 | ok | T12 / read_flow |
| K-18 | W2 | `put --repo` = Catalog id | `TARGET_REPOSITORY_DENIED` | ok | S0 |
| K-19 | W9 仓已归档 | `put` / `propose` | `REPOSITORY_ARCHIVED` | ok | write errors / S6 |
| K-20 | W3 多 op | 任一 op 失败 | 无部分提交（T3） | ok | T3 |
| K-21 | W3 | `writer.Reconcile` 预览 | 只出 ChangeSet；确认后走 `commit` | ok API / **frozen CLI** | `TestT7Reconcile`；Help 明示 connector kit 在墙外，无 `kc reconcile` |

### 2.4 B Aspect Binding

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| B-01 | W2 | PUT Aspect + inline state Binding | 声明可在同一 commit `resolve-binding`；不调用 runtime | ok | `TestResolveInlineStateAndStreamBindings` / CLI Binding E2E |
| B-02 | W2 | PUT Aspect + inline stream Binding | record schema 仍由 schema_ref 声明；底座无 APPEND | ok | 同上 |
| B-03 | W2 | Binding 引用 ResourceDescriptor | Descriptor 在同一 pinned commit 解析，返回 descriptorDigest | ok | `TestResolveDescriptorBindingAtPinnedCommit` |
| B-04 | W3 已有 Binding | PUT 相同 value，只改 operation/runtime | value digest 不变，declarationDigest 改变，LOG 保留 revision | ok | `TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged` |
| B-05 | W2 | PUT DescriptorRef 与 inline 字段并存 / 声明不完整 | `USAGE_INVALID`，失败关闭 | ok | `TestValidateBindingRejectsAmbiguousAndIncompleteDeclarations` |
| B-06 | W2 有手写 frontmatter | `ingest` 非法 value_source | Snapshot 扫描失败，不降级成普通 Snapshot value | ok | `TestIngestRejectsMalformedOrInvalidValueSource` |
| B-07 | W8 两仓同一 Address 都声明 Binding | `resolve-binding --workspace` | `ResolvedBinding[]` 两条；上层必须处理歧义 | ok API；DSH 工具拒绝多条 | Binding API tests |
| B-08 | W4 State Binding + 注入 StateLookup | `read --workspace` | 返回绑定后的值，同时携带 declaration/observation 双 basis | ok | `knowledge/serving` + HTTP E2E |
| B-09 | W4 State Binding、无 runtime | `read --workspace` | `CAPABILITY_UNSATISFIED`，不得把 `null` 占位当结果 | ok | CLI E2E |
| B-10 | W4 Stream Binding | 普通 `read --workspace` | `CAPABILITY_UNSATISFIED`，不隐式数组化 Stream | ok | `knowledge/serving` tests |
| B-11 | W4 State Binding | VFS read 同一单元 | 返回固定声明文件且不调用 StateLookup | ok | HTTP VFS/Binding E2E |
| B-12 | 任意 | `append` / `stream` CLI 或 HTTP | unknown command / 404 | ok | `TestAppendAndStreamSurfacesStayAbsent` |
| B-13 | W4 + 独立 `resource-access/v1` runtime | `read --workspace` | Knowledge Server 经 HTTP 传 pinned Binding、身份与关联信息；runtime 返回 value+basis | ok | `TestHTTPStateLookupCallsIndependentResourceRuntime` + `make test-state-runtime-e2e` Docker runtime + HTTP Binding/VFS E2E |
| B-14 | State Binding 缺 lookup/read 或 runtime 返回 bare result | `read --workspace` | `CAPABILITY_UNSATISFIED`，不猜 operation、不接受无 basis 正文 | ok | `TestHTTPStateLookupRejectsUnsupportedAndDishonestRuntime` |
| B-15 | SEARCH 命中含 State Binding | Snapshot-only query 命中后逻辑 hydrate；State-field query 使用独立动态投影 | 两条路径都返回绑定后的值和 observation basis | ok | `TestWorkspaceSearchHitUsesLogicalStateHydration` / `TestLiveHTTPDynamicStateSearchJourney` |
| B-16 | 无公开 Workspace LIST | 旧 list surface | 明确拒绝；State hydrate 只由 READ/SEARCH hit 使用，维护扫描与文件投影保持声明视图 | ok | `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` |
| B-17 | State refresh 已发布 | VFS/Repository read 同一 Address | HEAD 与占位值不变；observation 不进入 Snapshot | ok | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` / Docker journey |
| B-18 | W4 + ResourceDescriptor + 独立 runtime | `resource access --object … --operation … --input …` | Descriptor 在同一 pin 回读；只使用声明中的 runtime/protocol/call；透传固定仓/commit/object 与输入；缺失 operation 或混用 Binding 参数失败关闭 | ok | `TestCatalogViewsChecksSnapshotExportAndMountMaintenance` / `DW-AGENT-01` 实时 SQL |

### 2.5 R 维护读（`--repo` + `--commit`/`--ref`）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| R-01 | W3 | `resolve --repo --commit` | `RESOLVED`，无正文 | ok | read_flow |
| R-02 | W3 | resolve 不存在对象 | `UNRESOLVED`（不是错误信封） | ok | read_flow |
| R-03 | W3 | `read` 未知 sha | `VERSION_UNRESOLVED` | ok | T12 |
| R-04 | W3 | `read` 已知 commit、无对象 | `KNOWLEDGE_REF_UNRESOLVED` | ok | T12 |
| R-05 | W3 后续无关 commit | `log --object` | 只占引入该 digest 的 commit | ok | T12 / S5 |
| R-06 | W3 两版本 | `diff --from --to` | 两 pinned 上的对象值 | ok | T12 / S5 |
| R-07 | W3 | `provenance` | 本对象信封链；不是 git log，不爬 `sourceRefs` | ok | provider conformance / S5 / T7 citation |
| R-08 | W3 | `list --repo --commit` | 扁平枚举；路径不是身份 | ok | read_flow |
| R-09 | 有 `schema/*` | `describe-schema` | AccessHints；非 schema 对象忽略 | ok | `knowledge/reader/schema_test.go` |
| R-10 | 多 Aspect | `read --aspect` / `readAddress` | 单单元 | ok | S5 |
| R-11 | 有 permissions Aspect | READ 使用 `AspectSelector` exclude；SEARCH 只按 schema access hints | Canonical 仍在；Reader 不持第二套投影 | ok | T8 |
| R-12 | W4 | `--workspace` 兼 `--repo`/`--commit`/`--ref` | 拒绝组合 | ok | consume_flow |

### 2.6 V 消费 Serving（只 `--workspace`）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| V-01 | W4 然后又 COMMIT | **新** `read --workspace` | 解到新 HEAD（跟已发布 selector） | ok | consume_flow / T11 / serving |
| V-02 | 已 OpenWorkspace | 命令进行中再 COMMIT | 本次 pin 不动（K-11） | ok | `TestOpenedWorkspacePinDoesNotMoveWithLaterCommit`；Help 明示一条 CLI 命令只 resolve 一次，跨命令用 `--pin` |
| V-03 | W8 同 object_id 两仓 | `read --workspace --object` | 两条 FederatedValue，不按 scope 覆盖（K-13） | ok | T11 / S4 / checkout 两文件 |
| V-04 | W4 | `read --workspace` 不存在对象 | **空数组**，不是错误（维护口才是 `KNOWLEDGE_REF_UNRESOLVED`） | ok | 通用消费流覆盖 |
| V-05 | W1 | 未知 Workspace | `WORKSPACE_INVALID` | ok | consume_flow |
| V-06 | W4 | `search --workspace` | 各仓在**这次 pin** 上 SearchAt，不回绕 live | ok | consume_flow / `TestSearchAtDoesNotRewindLive` |
| V-07 | W4 知识仓无显式 mount | `checkout --workspace` | `CAPABILITY_UNSATISFIED`；禁止扫描知识仓伪造工作树，宿主投影使用 kcfs fixed plan | ok | `TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning` |
| V-11 | W4 | `inspect --workspace` | CatalogState + pin + AccessPlan + 各仓 index；不是新协议对象 | ok | consume_flow |
| V-12 | W4 | `list` / `log --object` / `provenance` / `describe-schema --workspace` | 钉在这次 pin | ok | consume_flow / S5 |
| V-14 | W4 | `define-workspace` | **不发权**；无 `--repo` allow 不能读成员 | ok | `TestWorkspaceAuthorizationCoverageIsHonest` |

### 2.7 M 维护闭环（提案）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| M-01 | W4 | `propose` | candidate=C1；**main 不动**；`read --workspace` 仍旧值（K-07） | ok | T9 / S3 |
| M-02 | W5 | `preview --workspace` | Preview 只写 ControlState；登记表无 pin yaml | ok | T9 / S3 |
| M-03 | W5 | `validate --preview` | 结构检查（仓已挂、commit 在）；写出 ValidationReport | ok | T9 / hook_gate |
| M-04 | W6 | `record-validation --suite --outcome` | 只绑定传入 PASSED/FAILED，不跑套件 | ok | T9 |
| M-05 | 结构 FAILED 或 suite FAILED | `merge` | `GATE_UNSATISFIED`；main 不动 | ok | T9 / hook_gate |
| M-06 | W6 PASSED | `merge` | main 快进到 C1；**下次** `read --workspace` 见新值 | ok | T9 / S3 / CLI |
| M-07 | preview 后 candidate 再提交 | `merge` | `CANDIDATE_MOVED` | ok | T9 / S3 |
| M-08 | preview 后别人推走 main | `merge` | `NON_FAST_FORWARD` | ok | T9 |
| M-09 | 有 `gate-add --on merge --require validate,suite:x` | 缺 suite 证据 | `GATE_UNSATISFIED` | ok | T9 / hook_gate |
| M-10 | Preview 成员变了，拿旧 PASSED | `merge` | `VALIDATION_BASIS_MISMATCH` | ok | T9 |
| M-12 | W6 | pre-merge hook 成功、gate 仍缺 suite | hook ≠ gate；仍 `GATE_UNSATISFIED` | ok | `TestPreMergeDoesNotSatisfyGate` |
| M-13 | W4 | `put` / `read` | 不查 gates.json | ok | `TestReadPathAndPutIgnoreGates` |

### 2.8 I 索引（③）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| I-01 | W3 | COMMIT | `AfterSnapshot` 增量；`search` 能命中；索引非权威 | ok | `TestCatalogHookUpdatesIndexAfterCommit` `TestSearchAfterPutIsIncremental` |
| I-02 | W5 | `propose` | **不**通知 Catalog Hook | ok | `TestProposalDoesNotNotifyCatalog` |
| I-03 | live 已到 U2 | `SearchAt(U1)` | 不把 live rewind 到 U1 | ok | `TestSearchAtDoesNotRewindLive` |
| I-04 | 改 `schema/*` AccessHints | COMMIT | rebuild（不是增量 content） | ok | `TestIndexSchemaChangeForcesRebuild` |
| I-05 | W8 | `describe-access --workspace` | 每仓一份逻辑 AccessSpec，不按 Workspace 建表 | ok | `TestPlanAccessTwoRepositories` |
| I-06 | permissions 无 text hint | 编索引 | 省略；声明了 access 才进 | ok | index_test |
| I-07 | W4 | `describe-index --repo` | basis / lag / compiled hints | ok | `TestDescribeIndexShowsCompiledSpec` / consume |
| I-08 | 未声明 MATCH 车道 | `search --query` | `CAPABILITY_UNSATISFIED` | ok | index / searchop |
| I-09 | Hook 失败 | `define-workspace` | 配方仍成功（hook 不回滚 ①） | ok | `TestCatalogHookFailureDoesNotFailDefineWorkspace` |
| I-10 | 两个 schema/aspect 有同名 path | 裸 path SEARCH | `USAGE_INVALID`；完整 FieldRef 可用 | ok | `TestCheckSearchRejectsAmbiguousBarePath` |
| I-11 | Provider 返回 authority 中不存在或 wrong-basis CandidateRef | hydrate | `PRECONDITION_FAILED`；错误坐标时 authority 零调用 | ok | `TestSearchRejectsCandidateMissingFromFixedAuthorityBasis` `TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate` |
| I-12 | Workspace 一成员不支持 query | 联邦 SEARCH | 整次 `CAPABILITY_UNSATISFIED` fail closed；不把能力缺口伪装成 partial | ok | `TestWorkspaceSearchFailsClosedWhenAnyMemberCannotSatisfyQuery` |
| I-13 | schema access 含 key/summary/stored/gin/hnsw | DESCRIBE_SCHEMA | `USAGE_INVALID`，不得静默忽略 | ok | `TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens` |
| I-14 | 同一 Repository basis 被多个 Workspace 引用 | 编译/查询投影 | 复用 `(repository,basis,provider,physicalDigest)`；`CompiledDoc` 不含 Workspace/PinID | ok | `TestCompiledDocumentDoesNotCarryWorkspaceScope` / Workspace search tests |
| I-15 | Binding 占位 null、未 observation | 编译投影 | 不进入 `EligibleFields`，不能误报 MISSING | ok | `TestProjectionCompilerRequiresObservationForBindingEligibility` |
| I-16 | Binding 成功 observation=null | 编译与 MISSING | 字段 eligible、无 cell，MISSING 可命中 | ok | compiler test / `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-17 | observation 值变化、commit 不变 | `RefreshState` | 有界 streaming warm rebuild；旧值不再命中；HEAD 不变 | ok | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` |
| I-18 | runtime refresh 失败 | `RefreshState` | `TEMPORARY_UNAVAILABLE`；已发布 revision 不被空/null 覆盖 | ok | `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-19 | State text + typed range + Snapshot filter | OpenSearch SEARCH | 同一完整 object 文档隐式 AND，并从同 revision Serving State hydrate | ok | `TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate` |
| I-20 | 动态 SEARCH | 返回 SearchView/hit | SearchView 仅含紧凑 `projectionRevisions`；逐 hit `KnowledgeVersion.Observations` 完整 | ok | live OpenSearch + Docker HTTP journey / `TestDynamicProjectionPublicEnvelopesStayCompact` |
| I-21 | 受权 `index-sync` + StateLookup | observer 只发 repo/ref 定位 | 控制器 pull runtime 并发布动态投影；notice 不携带正文 | ok | `TestLiveHTTPDynamicStateSearchJourney` |
| I-22 | 独立 runtime + OpenSearch 容器 | HTTP facade index-sync/search | 动态字段发现候选、同 basis hydrate、Snapshot 不变 | ok | `make test-state-runtime-e2e` |

`PROJECTION_CONTROLLER.md` 的 K/S/O/Q/B 核心能力已由 I-15..I-22 与 Knowledge Serving 测试覆盖；
完整 D-01..D-10（尤其真实 source observer、Gitea、KC 容器重启）仍是场景级未完成项，不能把当前
双容器适配器旅程记成整组 D 已通过。

### 2.9 P 授权 / Hook / Gate（facade）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| P-01 | 空 allow.json | 不带 `--as` | 主人放行 | ok | 现有 CLI 默认 |
| P-02 | 空 allow | `--as bot put` | `FORBIDDEN`；七列不动 | ok | write errors / LifecycleAndAllow |
| P-03 | 只 allow `read-workspace` | `--as` member consumption | `FORBIDDEN` | ok | `TestWorkspaceAuthorizationCoverageIsHonest` |
| P-04 | Catalog A 的 allow | Catalog B `--as` | 不继承 | ok | `TestCatalogIsolationDoesNotShareAllow` |
| P-05 | pre `put` hook 非 0 | `put` | `HOOK_DENIED`；无 commit | ok | `TestPrePutDeniedLeavesNoCommit` |
| P-06 | REPLAYED | hook | 不打 | ok | `TestReplayedSkipsHook` |
| P-07 | post hook | 成功写 | payload 只有指针，无正文 | ok | `TestPostPutPointersOnly` |
| P-08 | post 失败 | 已 define-workspace | **不**回滚登记表 | ok | `TestPostDefineWorkspacePointersOnlyAndFailureDoesNotRollback` |
| P-09 | `hook-add` / `gate-add` | CRUD | `.kc/hooks.json` / `gates.json` | ok | `TestHookAndGateConfigCRUD` |
| P-10 | 仓内 `permissions` Aspect | `kc knowledge read` | **不是**闸门；GRANT 不进 `allow.json` | ok | `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess`；T8 可裁 |
| P-11 | 已 allow | `revoke` / `whoami` / `allowed` | 规则消失后 `--as` 拒绝 | ok | `TestUserJourneyManageAgentAccess` |
| P-12 | `kc serve` | `X-Kc-As` | 等同 `--as` | ok | `TestXKcAsUsesTheSameAuthorizationRulesAsCLI` / HTTP telemetry coverage |
| P-13 | `kc serve --auth gitea` | PAT / Basic → `/api/v1/user` | `gitea:<id>`；伪造 `X-Kc-As` 和管理口提权被拒 | ok | `TestLiveServiceProviderConsumerJourney` / `make test-service-e2e` |
| P-14 | Workspace 两仓，只 allow 一仓 | READ / pin / inspect / SEARCH | 裸 READ / pin / inspect fail closed；SEARCH 只查授权仓并报 `partial`，SearchView 不泄露隐藏仓 | ok | `TestWorkspaceAuthorizationCoverageIsHonest` |

### 2.10 N 入站 connector（不是 hook）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| N-01 | Observed 多于 Desired | `Preview(patch)` | 不因多余 Observed 而 REMOVE | ok | `connector/preview_test.go` |
| N-02 | Desired 缺、Observed∩Scope 有 | `Preview(reconcile)` | REMOVE 那些 Address | ok | 同上 |
| N-03 | Desired 超 Scope | Preview | `SCOPE_DENIED` | ok | 同上 |
| N-04 | 无漂移 | Preview | `empty=true`，不强迫 COMMIT | ok | 同上 |
| N-05 | 预览非空 | `Writer.Commit` | SOURCE 落盘 | ok | `TestPreviewThenCommit` |
| N-06 | 任意 | `kc connector-run` | **不存在**（入站不是 CLI 插件宿主） | ok（负例） | walkthrough D.2 |
| N-07 | 任意 | `kc reconcile` | 不存在；外部 connector 调 kit 后提交 ChangeSet | **frozen（分层边界）** | CLI Help、CONNECTORS.md、T7 API |

### 2.11 S 适配器（K-23）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| S-01 | 私有 memory fake + Reader/Writer | provider-independent 组合合同 | Snapshot 身份/CAS/历史 + LOG/DIFF/REMOVE/Archive/schema_ref/PROPOSAL | ok | `TestProviderIndependentRepositoryContract` |
| S-02 | Dolt | 同一份 T12 + Writer contract | 语义不变；无 CLI 时才用可用 Docker daemon | ok | `TestNativeDoltRepositoryContract` |
| S-03 | Gitea + Reader/Writer | 同一份 T12 | Adapter 无工作区且不解释知识；上层读 pinned commit | ok | `TestT12GiteaContract` |
| S-04 | local profile 无 provider | SEARCH | `CAPABILITY_UNSATISFIED`；精确 READ/VFS 不受影响 | ok | `TestLocalProfileHasNoSearchProjection` |
| S-05 | OpenSearch Retriever/Maintainer | 原子 SEARCH 算子 | MATCH=superset/partial；未声明 → `CAPABILITY_UNSATISFIED` | ok | opensearch tests |
| S-06 | 所有 Retriever | CandidateRef | 不返回正文/stored payload | ok | engine interface + search tests |

### 2.12 F HTTP 门面

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| F-01 | serve 已起 | typed Writer 写入，再从 typed Knowledge API 读取 | Client 与 HTTP route 调用同一应用服务和 semantic action | ok | `TestLiveServiceProviderConsumerJourney` / `make test-service-e2e` |
| F-02 | 无 allow | `X-Kc-As: bot` | `FORBIDDEN` | ok | serve_test |
| F-03 | HTTP define-workspace | 登记表 git | stamp 含 as / request-id | ok | serve_test |
| F-04 | `kc serve` 已启动 | 旧 verb 路由或未知资源 | 404 | ok | service route contract |
| F-05 | `kc serve` 已启动 | 正式 Catalog/Knowledge/Writer/Governance route | 单机与共享部署均使用同一 typed Client/HTTP 语义；HTTP 不走 CLI command table | ok | `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` / remote CLI tests / live service journey |
| F-06 | MCP | — | 未实现 | **frozen** | walkthrough D.2 |

### 2.13 O 运行可观测性

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| O-01 | `kc serve` | 产品 HTTP 请求 | OTel metric + SERVER/application span；指标无 request/repo/object 等高基数标签 | ok | `internal/telemetry` / `cli/http_telemetry_internal_test.go` |
| O-02 | OTLP logs 已配置 | 产品 HTTP 请求 | 每请求至多一条 `kc.http.request.completed`；requestId、traceId、spanId 可关联，正文/凭证/query 不入日志 | ok | `TestObservedHTTPHandlerCorrelatesCompletionLogAndSuppressesManagementNoise` |
| O-03 | management 流量 | `/metrics` / `/health` / `/livez` / `/readyz*` | 保留 transport metric，不导出 completion log 和 trace，避免探针淹没业务信号 | ok | 同上 |
| O-04 | Compose observability profile | 真实 SEARCH、Canonical READ、Workspace resolve | Prometheus 原始指标/rules、Jaeger trace、Loki log、五个 provisioned Grafana dashboards 均可查询；同一 traceId 跨 log/trace 对账 | ok | `make dw-obs-smoke` |
| O-05 | Gitea/OpenSearch/resource-access/MySQL | 跨进程调用 | 标准 CLIENT/SERVER span 与 W3C context 覆盖完整依赖图 | gap | 当前 Jaeger 依赖视图只证明 `kc-server` 内部 span，不能冒充静态系统架构 |
| O-06 | Collector/Loki/Jaeger | 生产部署 | 持久存储、备份、租户隔离、tail sampling、容量与故障演练 | gap | Compose profile 仅是 24h/内存本地验收拓扑 |
| O-07 | 30 天 SLO | SEARCH/READ/Writer 可用性与 latency good-event ratio | 有 error-budget remaining，且 `1h+5m@14.4x`、`6h+30m@6x`、`1d+2h@3x` 多窗口 burn-rate 告警可证明 firing/recovery | partial | SEARCH/READ/Writer 已有多窗口 availability burn、latency good-event、30 天 budget recording 与面板；缺真实 30 天/规模基线及告警 firing/recovery 演练 |
| O-08 | Snapshot/Binding/identity provider/Hook/Gate | 真实依赖调用 | 实现 rate/error/duration/in-flight/bytes/backlog 所需的低基数原始指标和 child span | partial | 身份 provider、State Binding、Writer、Projection、Hook/outbox、Gate、VFS 已接真实边界和包测试；Snapshot authority 的 context-aware decorator、active/bytes 及 Gitea/OpenSearch 跨进程传播仍缺 |
| O-09 | OTel Collector/Jaeger/Loki/Prometheus | backend 慢、断开或队列满 | Collector accepted/refused/enqueue-failed/send-failed/queue 与 backend ingest/query/storage 自监控可见并告警 | partial | 已 scrape Collector internal metrics 并预置 unavailable/export failure/refused/queue saturation 告警；Jaeger/Loki ingest/query/storage 自监控与故障演练仍缺 |
| O-10 | 规模负载 | Workspace/Search/Writer/Projection/Evidence 放大 | 容量面板同时展示输入负载、fan-out/工作量、队列/饱和与用户延迟，并与 `SCALE_BENCHMARK.md` 档位对齐 | partial | 容量/行为面板已有 operation input、Writer payload/change、Projection docs/change/backlog、VFS bytes/entries，并关联旅程延迟；缺 authority calls/bytes、projection ETA、evidence bytes/disk 与压测基线 |
| O-11 | access/feedback/system/audit evidence | 身份与用户行为分析 | 分离采用、治理和安全视图；可聚合 DAU/WAU、委托、拒绝、仓/工作区采用、零结果/refine/feedback，不把 principal 做 metric/Loki label | partial | 原始可信 evidence、trace 查询、hitmap，以及 provider/principal-kind/delegated/authn/authz 有界聚合面板已有；缺受控高基数聚合存储/作业、权限分面、委托验证和异常规则 |
| O-12 | 专用 canary Repository | 定时 resolve→READ、commit→SEARCH、evidence reconciliation 与故障注入 | 黑盒 correctness/availability/freshness 信号与每类告警 firing/recovery 证据 | gap | `dw-obs-smoke` 只验证组件链路和查询定义，不是定时黑盒探针或告警故障演练 |
| O-13 | 发布/配置变化 | incident 调查 | service version、telemetry schema、受控 config digest 和 deployment annotation 可与 SLO/资源时序对齐 | partial | OTel Resource 已有 service/schema version；缺配置 digest 和 Grafana 发布标记 |

### 2.14 D 协议已冻结、参考实现未做

这些**不要**写成正路径用例。若暴露入口，预期是 `CAPABILITY_UNSATISFIED` 或「未知命令」。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| D-01 | 任意 | `CAPABILITIES` 独立清单 | 未暴露；缺失能力必须显式 | frozen | `TestRemovedCommandsAreRejected` |
| D-02 | 任意 | `EXPAND_RELATIONS` | 未实现 | frozen | 同上 |
| D-03 | 任意 | `WATCH_UPDATES` | 投递端是 post hook；无订阅口 | frozen | 同上 |
| D-04 | 任意 | `LIST_TREE` 父子枚举 | 仍扁平 `LIST` | frozen | 同上 |
| D-05 | 有 Stream Binding | 流 SEARCH / `tail` | SEARCH `CAPABILITY_UNSATISFIED`；tail 无入口。window 已实现 | frozen（负例 ok） | frozen command tests |
| D-06 | Fork 发布 | 自动三方 sync（K-15） | 当前发表路径是目标仓 `propose` 新对象；自动三方 sync 未做 | ok 当前路径 / frozen 自动 sync | `TestForkPublishDoesNotCopyPersonal` |
| D-07 | Vendor Repository | 生成只读副本（K-16） | 未做 | frozen | — |
| D-08 | 两次 OpenWorkspace | ViewDiff | 未做 | frozen | — |
| D-09 | 原子查询可用 | RQL（OR/NOT/括号） | 原子算子隐式 AND | frozen | — |
| D-10 | 上游知识更新 | 检查引用方仓 commit | 禁止跨 Repository merge；下次 ResolveWorkspace 重解 | ok | `TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository` |

---

## 3. 错误码覆盖

协议信封一律 `{error:{code,message}}`。无测试的码补齐时优先。

| code | 该出现的前置 | 现况 |
|---|---|---|
| `USAGE_INVALID` | 缺 flag、未知命令、空 changeset、search/stream 形状、未挂载仓/流、无 code 的 `fmt.Errorf` 归一 | ok CLI |
| `PRECONDITION_FAILED` | IfAbsent / DERIVATION / digest / stale cursor / dirty worktree | ok |
| `NON_FAST_FORWARD` | 过期 expected / merge 时 main 被推走 | ok |
| `OBJECT_ID_CONFLICT` | 重复 Address / blob+aspect 混 | ok |
| `IDEMPOTENCY_CONFLICT` | 同 command_id 异 digest | ok |
| `EVENT_ID_CONFLICT` | 同 eventId 异 payload | ok |
| `POSITION_REGRESSION` | 声明 `MONOTONIC_PER_PARTITION` 的实现才可达；Base profile=`NONE` | frozen；数仓 connector checkpoint 有独立可执行断言 |
| `WRITE_TARGET_REQUIRED` | 写未指定唯一 repository / ref（空 changeset 是 `USAGE_INVALID`） | ok |
| `SURFACE_MISMATCH` | Surface 与地址不符 | frozen：当前公开请求由独立结构表达 Surface，不接受可冲突字段 |
| `SCOPE_DENIED` | connector Desired 超 Scope | ok |
| `SCHEMA_UNSUPPORTED` | schema 形态不允许 | frozen：参考实现只解析 AccessHints，不承诺通用 schema validator |
| `SCHEMA_REVISION_UNRESOLVED` | 钉的 schema 不可解析 | ok |
| `TARGET_REPOSITORY_DENIED` | 把 Catalog id 当 Snapshot Repository 写目标 | ok |
| `KNOWLEDGE_REF_UNRESOLVED` | 维护读缺对象；lookup 缺 event | ok |
| `VERSION_UNRESOLVED` | 未知 commit / 不存在的 ref | ok |
| `CAPABILITY_UNSATISFIED` | 未声明 SEARCH 车道；stub 未实现 | ok |
| `TEMPORARY_UNAVAILABLE` | 瞬时 Backend I/O（Gitea/hook HTTP）；不是未挂载 | ok |
| `CANDIDATE_MOVED` | preview 后 candidate 前进 | ok |
| `VALIDATION_BASIS_MISMATCH` | 旧 PASSED 绑新 Preview | ok |
| `WORKSPACE_INVALID` | Workspace 配方不能用：无 Workspace / 重复 source / 已 retire / selector 无此 ref | ok |
| `FORBIDDEN` | `--as` 未命中 allow | ok |
| `CATALOG_ARCHIVED` | 归档后 define-workspace | ok |
| `REPOSITORY_ARCHIVED` | 归档后写 | ok |
| `GATE_UNSATISFIED` | merge 缺证据 | ok |
| `HOOK_DENIED` | pre 非 0 | ok |

---

## 4. 交叉格（正交维里必须测的乘积）

只测这些，不要笛卡尔爆炸。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| X-01 | W5 candidate 存在 | 消费读 | propose 期间 `read --workspace` 仍旧 main | ok | S3 |
| X-02 | W5 candidate 存在 | 观察索引 | propose 不 `AfterSnapshot` | ok | I-02 |
| X-03 | W6 Preview 存在 | 读取 Catalog | Preview 不写登记表 git 配方 | ok | M-02 |
| X-04 | 命令内 pin | 并发 merge | 本次结果仍旧 pin；**下次**命令见新 HEAD | ok | API serving；CLI 一命令一 pin，跨命令可 `--pin` 重放 |
| X-05 | Binding declaration pin | Descriptor 后续更新 | 旧 pin 仍解析旧 runtime/digest | ok | `TestResolveDescriptorBindingAtPinnedCommit` |
| X-06 | 联邦 Workspace | 只 allow 一仓 | 裸知识读 fail closed；SEARCH 为授权子集 `partial`，SearchView 不泄露隐藏仓 | ok | P-14 |
| X-07 | Catalog 已归档 | 写个人仓、define Workspace | 禁 define；个人仓仍 COMMIT | ok | S6 |
| X-08 | 有 schema_ref | propose | 与 COMMIT 同一套解析 | ok | `TestSchemaRefOnPropose` |
| X-09 | 已有成功 command_id | 重放带 Hook 的命令 | REPLAYED 不打 hook | ok | P-06 |
| X-10 | 已配置 Gate 与 Hook | pre-merge | pre-merge 成功 ≠ 清单满足 | ok | M-12 |
| X-11 | 已移除 driver 名 | store-set / repo-add | 不回流成 authority/index/cache | ok | W-09 |
| X-12 | permissions Aspect 存在 | `kc admin grant add` 与 READ | 快照可读；不放行 SELECT、不当 read 闸门 | ok | P-10 |

---

## 5. 补齐顺序（后续按这个打勾）

不要从 TPC-H 再加表。先把底座非法转移钉死，再补观察点，最后才是门面与适配器 stub。

### P0 非法转移 / 错误码（系统可能静默成功的洞）

- [x] **K-14** 两文件同一 Address → `OBJECT_ID_CONFLICT`（provider-independent Tree interpreter）
- [x] **K-13** blob+aspect 两个写入方向统一为 `OBJECT_ID_CONFLICT`
- [x] **B-05/B-06** 非法 Binding 声明失败关闭，不降级成 Snapshot value
- [x] **X-05** Descriptor 按声明 pin 解析，后续 commit 不污染旧任务
- [x] **P-11** `revoke` / `whoami` / `allowed` CLI；公开 `--workspace` 范围已钉住

### P1 状态观察点（正路径已有、缺「哪一列不变」）

- [x] **V-02 CLI**：Help 明示「一条 `kc` 命令 = 一次 ResolveWorkspace」，跨命令用 `--pin`；Go API 覆盖长任务内固定 pin
- [x] **K-21** 不做 `kc reconcile` facade；Help 明示 connector kit 在墙外，确认后只提交 ChangeSet
- [x] **D-10** 上游 COMMIT 后引用方仓 commit 不变
- [x] **B-01** CLI / Workspace `resolve-binding` 返回 declaration commit/digest
- [x] **F-05 核心治理链** HTTP：`resolve-binding` 与其它动词共用 command table

### P2 适配器与冻结面

- [x] **D-01..D-04** 冻结动词显式保持 `USAGE_INVALID`
- [x] **D-06** 当前发布路径钉为「目标仓 propose 新对象 + sourceRefs」；自动三方 sync 明确 frozen

### P3 不要补的

- 不要把 TPC-H / compose / 源客户端加进底座包或通用测试目录；它们只属于 `.data/data-warehouse/` integration suite，未来可整体迁出
- 不要把 ES / SR 当 Canonical
- 不要把 `permissions` Aspect 做成 `kc knowledge read` 闸门
- 不要把场景套件跑进 `kc governance preview validate`

---

## 6. 最小走通脚本（补齐时的手工对照）

自动化以 `make test` / `make test-all` 为准。Dolt/Gitea 合同归各自 adapter 包；Gitea testkit 由使用包的 `TestMain` 回收容器。手工只用来核对「进入的状态」七列，不代替测试。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
H=/tmp/kc-base-catalog
rm -rf "$H"
go run ./cmd/kc -- local init --home "$H" --catalog acme/catalog
go run ./cmd/kc -- local repository attach --home "$H" --repo kr://acme/public/core
go run ./cmd/kc -- local grant bootstrap --home "$H" --principal user:local-admin
go run ./cmd/kc -- serve --home "$H"          # 另一终端

export KC_SERVER_URL=http://127.0.0.1:8080
export KC_AS=user:local-admin
kc() { go run ./cmd/kc -- "$@"; }

kc catalog show
kc writer put --command-id u1 --repo kr://acme/public/core \
  --object runbooks/oncall --value '{"text":"freeze"}' --origin-kind SOURCE
kc catalog workspace define --workspace agent --revision 1 \
  --source kr://acme/public/core=refs/heads/main
kc knowledge read --workspace agent --object runbooks/oncall
kc catalog workspace resolve --workspace agent                 # 无 --object → pin
kc operations access describe --workspace agent
# 非法
go run ./cmd/kc -- local repository attach --home "$H" --repo kr://acme/catalog    # 必须失败
kc knowledge read --workspace agent --repo kr://acme/public/core --object runbooks/oncall
```

具体业务故事由墙外知识提供方维护，不并进本目录。数仓材料只在 `.data/data-warehouse/` 黑盒 integration suite 中维护。
