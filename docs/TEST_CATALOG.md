# 底座验证目录：状态 × 操作

日期：2026-08-27

范围：仓库根协议（`kernel` / `catalog` / `writer` / `reader` / `controlplane` / `cli`）。

对照：[`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md)（操作→进入的状态）、`cli/mvp_acceptance_test.go`（接入/消费最短闭环）、`cli/user_journey_test.go`（通用端到端旅程）、T1–T12（`README.md` Conformance）。

本目录按当前 Snapshot + Aspect Binding + 声明式 AccessSpec 契约验收。旧 APPEND/Stream/AppendCuts surface 已退役，命令表与 HTTP 都必须拒绝它们。

这不是 TPC-H。数仓域验证材料位于受跟踪的 `.data/data-warehouse/` 黑盒 integration suite，属于墙外知识提供方。底座要用另一张图：任意知识仓在空 home 上，经过哪些状态、每个状态下哪些操作合法、失败必须落到哪个错误码。

补齐时：**判断归属 → 只改仓库根 → `go test` 能红能绿**。不要把数仓表名、Hive GRANT、compose 写进本目录的用例。

---

## 0. 怎么用

唯一自动化入口是仓库根 `scripts/testsuite.sh`：

```bash
make test           # 临时 OpenSearch + component + boundary + local E2E
make test-e2e       # CLI/HTTP/Catalog；强制每个公开 kc 动词至少被真实旅程调用
make test-race      # command log / hook / Reader / Index / CLI 并发路径
make test-cover     # short suite + statement coverage 不回退门禁
make test-plugin    # DSH typed Agent tools、固定 pin、build/package
make test-agent-e2e # 真实模型六角色，严格检查 Skill/tool trace 与 host 旁路
make test-agent-ux-e2e # 真实模型自然语言问答，检查概念/入口/恢复语义与 Skill-only trace
make test-service-e2e # Gitea + OpenSearch 下的 provider/consumer 双身份验收
make test-adapters  # Gitea + Dolt + OpenSearch
make test-docker    # adapters + 双角色 service E2E + Linux/FUSE
make test-all       # 全部；缺 Docker 或 live adapter 失败即红
```

`make test`、`make test-e2e`、race 与 coverage 组会启动一次性 OpenSearch；项目不保留第二套
本地检索实现。`testing.Short()` 只把 Gitea/Dolt 等其它 live authority 从本地组隔离；显式
adapter/Docker 组不得把环境缺失静默算作通过。命令覆盖由 CLI 测试进程实际记录 CLI/HTTP 调用，并在
`KC_ASSERT_E2E_COVERAGE=1` 时与唯一命令表对账，不靠手工维护一份“已覆盖”名单。

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
| 授权 | 主人 / `--as` 命中 / `--as` 拒绝 | `kc allow`（不改七列） |

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
| W-01 | W0 | `kc init --catalog acme/catalog` | 登记表 git 出生；`read --catalog` → `{catalogId, repositories:[], workspaces:[]}` | ok | `TestCatalogRepoWriteFlow` |
| W-02 | W1 | 再 `init` 同一 catalog | 幂等；不建第二间 | ok | 同上 |
| W-03 | W1 | `init --catalog` 另一个 id | 拒绝（一间 home 先 `catalog-add`） | ok | 同上 |
| W-04 | W1 | `catalog-add --catalog kr://acme/docs/catalog` | 两间 Catalog；`allow` 不共享 | ok | `TestMultipleCatalogs` `TestCatalogIsolationDoesNotShareAllow` |
| W-05 | W1 | `repo-add --repo kr://acme/catalog` | 拒绝：登记表不是成员仓 | ok | `TestCatalogRepoWriteErrors` |
| W-06 | W1 | `repo-add --driver stream` / `--driver mysql` | 拒绝 | ok | `TestStoreConfigRejectsSecrets` |
| W-07 | W1 | `status` vs `read --catalog` vs `audit` | status=本机扫描；read=组合空间；audit=登记表 git | ok | `TestCatalogAuditIsGitLog` |
| W-08 | W0 | 任意动词无 `--home` 且无 `.kc` | `no kc home` | ok | `TestCatalogRepoWriteErrors` |
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
| C-07 | W4 | `retire-workspace` | `OpenWorkspace` → `WORKSPACE_INVALID`；其它 Workspace 仍可用 | ok | S6 / `TestLifecycleAndAllow` |
| C-08 | W4 | `archive-repo` | 该仓 `COMMIT`/`PROPOSE` → `REPOSITORY_ARCHIVED`；新 OpenWorkspace 不选入 | ok | lifecycle / write errors / S6 |
| C-09 | W4 | `archive-catalog` | `define-workspace` → `CATALOG_ARCHIVED`；未归档成员仓仍可写 | ok | S6 |
| C-10 | W8 | `define-workspace` 提高 revision、改 sources | **下次** OpenWorkspace 用新配方；本次 pin 不变 | ok | S4 |
| C-11 | W4 | `CheckResolved` / `validate --preview` | 只检查 Snapshot 成员与 commit，不解析 Binding | ok | catalog/control tests |
| C-12 | W4 | `define-workspace --as steward --request-id …` | 登记表 git stamp 含 as / request-id / ruleId | ok | `TestCatalogLogStampsAuthor` `TestHTTPFacadeStampsCatalogGit` |
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
| K-12 | W3 | 先后 PUT 两个 Aspect | 拼装对象两分区独立；`readAddress` 单单元 | ok | T12 aspect / FileGit |
| K-13 | W3 Entity blob 已在 | 再 PUT 同 id 的 Aspect | `OBJECT_ID_CONFLICT`；HEAD 不变 | ok | `TestFileGitRejectBlobAspectMix` |
| K-14 | W2 已挂普通 Git | 两文件同一 Address | 读取拒绝 `OBJECT_ID_CONFLICT`，不按路径顺序静默覆盖 | ok | `TestFileGitRejectsDuplicateAddressInExistingGit` |
| K-15 | W2 | `kc ingest --dir` | 只出 ChangeSet 预览；frontmatter `object_id` 胜路径；报告身份/Schema/SEARCH readiness；既有 Schema 只报未验证、不越权探测；不 COMMIT | ok | T7 / `TestWritePath` / `TestIngestDoesNotProbeExistingSchema` |
| K-16 | ingest 预览 | `kc commit --changeset` | 与 K-01 同：只推进成员 Ref | ok | write_flow |
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
| B-12 | 任意 | `append` / `stream` CLI 或 HTTP | unknown command | ok | `TestRemovedVerbsAreNotRegistered` / HTTP command table |
| B-13 | W4 + 独立 `resource-access/v1` runtime | `read --workspace` | Knowledge Server 经 HTTP 传 pinned Binding、身份与关联信息；runtime 返回 value+basis | ok | `TestHTTPStateLookupCallsIndependentResourceRuntime` + `make test-state-runtime-e2e` Docker runtime + HTTP Binding/VFS E2E |
| B-14 | State Binding 缺 lookup/read 或 runtime 返回 bare result | `read --workspace` | `CAPABILITY_UNSATISFIED`，不猜 operation、不接受无 basis 正文 | ok | `TestHTTPStateLookupRejectsUnsupportedAndDishonestRuntime` |
| B-15 | SEARCH 命中含 State Binding | Snapshot-only query 命中后逻辑 hydrate；State-field query 使用独立动态投影 | 两条路径都返回绑定后的值和 observation basis | ok | `TestWorkspaceSearchHitUsesLogicalStateHydration` / `TestLiveHTTPDynamicStateSearchJourney` |
| B-16 | Workspace LIST 含 State Binding | `list --workspace` | 与 READ 相同返回逻辑值与 observation；checkout/VFS 仍为声明视图 | ok | `TestListUsesTheSameLogicalStateHydrationAsRead` |
| B-17 | State refresh 已发布 | VFS/Repository read 同一 Address | HEAD 与占位值不变；observation 不进入 Snapshot | ok | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` / Docker journey |

### 2.5 R 维护读（`--repo` + `--commit`/`--ref`）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| R-01 | W3 | `resolve --repo --commit` | `RESOLVED`，无正文 | ok | read_flow |
| R-02 | W3 | resolve 不存在对象 | `UNRESOLVED`（不是错误信封） | ok | read_flow |
| R-03 | W3 | `read` 未知 sha | `VERSION_UNRESOLVED` | ok | T12 |
| R-04 | W3 | `read` 已知 commit、无对象 | `KNOWLEDGE_REF_UNRESOLVED` | ok | T12 |
| R-05 | W3 后续无关 commit | `log --object` | 只占引入该 digest 的 commit | ok | T12 / S5 |
| R-06 | W3 两版本 | `diff --from --to` | 两 pinned 上的对象值 | ok | T12 / S5 |
| R-07 | W3 | `provenance` | 本对象信封链；不是 git log，不爬 `sourceRefs` | ok | FileGit / S5 / T7 citation |
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
| V-07 | W4 | `checkout --workspace` | `layout.checkouts/<workspace>/`；pin 与 resolve 相同；只读 | ok | `TestCheckoutWorkspacePin` |
| V-08 | W8 | checkout 路径 | `仓/object_id`，不是 pathHint；同 id 两文件 | ok | checkout_test / consume |
| V-09 | W4 后再 put | 不重跑 checkout | 树仍旧 pin | ok | consume |
| V-10 | V-09 后 | 再 `checkout` | 跟上已发布分支 | ok | consume |
| V-11 | W4 | `inspect --workspace` | CatalogState + pin + AccessPlan + 各仓 index；不是新协议对象 | ok | consume_flow |
| V-12 | W4 | `list` / `log --object` / `provenance` / `describe-schema --workspace` | 钉在这次 pin | ok | consume_flow / S5 |
| V-13 | W4 只对一仓发 read | `--as` checkout | 未授权仓不落盘 | ok | `TestCheckoutWorkspacePin` bot |
| V-14 | W4 | `define-workspace` | **不发权**；无 `--repo` allow 不能读成员 | ok | `TestCompanyCatalogDoesNotGrantByView` |

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
| I-11 | Provider 返回 removed / wrong-basis CandidateRef | hydrate | 结果 `partial`，Claims 解释每个丢弃项 | ok | `TestSearchMarksStaleCandidatesPartialInsteadOfSilentlyDropping` |
| I-12 | Workspace 一成员不支持 query | 联邦 SEARCH | 保留其它 hydrated hit；整体 `partial` | ok | `TestWorkspaceSearchReportsUnsupportedMemberAsPartial` |
| I-13 | schema access 含 key/summary/stored/gin/hnsw | DESCRIBE_SCHEMA | `USAGE_INVALID`，不得静默忽略 | ok | `TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens` |
| I-14 | 同一 Repository basis 被多个 Workspace 引用 | 编译/查询投影 | 复用 `(repository,basis,provider,physicalDigest)`；`CompiledDoc` 不含 Workspace/PinID | ok | `TestCompiledDocumentDoesNotCarryWorkspaceScope` / Workspace search tests |
| I-15 | Binding 占位 null、未 observation | 编译投影 | 不进入 `EligibleFields`，不能误报 MISSING | ok | `TestProjectionCompilerRequiresObservationForBindingEligibility` |
| I-16 | Binding 成功 observation=null | 编译与 MISSING | 字段 eligible、无 cell，MISSING 可命中 | ok | compiler test / `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-17 | observation 值变化、commit 不变 | `RefreshState` | 动态投影增量 Apply；旧值不再命中；HEAD 不变 | ok | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` |
| I-18 | runtime refresh 失败 | `RefreshState` | `TEMPORARY_UNAVAILABLE`；已发布 revision 不被空/null 覆盖 | ok | `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-19 | State text + typed range + Snapshot filter | OpenSearch SEARCH | 同一完整 object 文档隐式 AND，并从同 revision Serving State hydrate | ok | `TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate` |
| I-20 | 动态 SEARCH | 返回 SearchView/hit | `projectionRevisions`、逐 Address observations、`KnowledgeVersion.Observations` 完整 | ok | live OpenSearch + Docker HTTP journey |
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
| P-03 | 只 allow `read-workspace` | `--as` `put` | `FORBIDDEN` | ok | `TestCompanyCatalogDoesNotGrantByView` |
| P-04 | Catalog A 的 allow | Catalog B `--as` | 不继承 | ok | `TestCatalogIsolationDoesNotShareAllow` |
| P-05 | pre `put` hook 非 0 | `put` | `HOOK_DENIED`；无 commit | ok | `TestPrePutDeniedLeavesNoCommit` |
| P-06 | REPLAYED | hook | 不打 | ok | `TestReplayedSkipsHook` |
| P-07 | post hook | 成功写 | payload 只有指针，无正文 | ok | `TestPostPutPointersOnly` |
| P-08 | post 失败 | 已 define-workspace | **不**回滚登记表 | ok | `TestPostDefineWorkspacePointersOnlyAndFailureDoesNotRollback` |
| P-09 | `hook-add` / `gate-add` | CRUD | `.kc/hooks.json` / `gates.json` | ok | `TestHookAndGateConfigCRUD` |
| P-10 | 仓内 `permissions` Aspect | `kc read` | **不是**闸门；GRANT 不进 `allow.json` | ok | `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess`；T8 可裁 |
| P-11 | 已 allow | `revoke` / `whoami` / `allowed` | 规则消失后 `--as` 拒绝 | ok | `TestUserJourneyManageAgentAccess` |
| P-12 | `kc serve` | `X-Kc-As` | 等同 `--as` | ok | `TestHTTPFacadeAsForbidden` |
| P-13 | `kc serve --auth gitea` | PAT / Basic → `/api/v1/user` | `gitea:<id>`；伪造 `X-Kc-As` 和管理口提权被拒 | ok | `TestHTTPFacadeAuthenticatesWithGitea` |
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
| S-01 | FileGit + Reader/Writer | T12 组合合同 | Snapshot 身份/CAS/历史 + LOG/DIFF/REMOVE/Archive/schema_ref/PROPOSAL | ok | `TestT12FileGitContract` |
| S-02 | Dolt | 同一份 T12 + Writer contract | 语义不变；无 CLI 时才用可用 Docker daemon | ok | `TestNativeDoltRepositoryContract` |
| S-03 | Gitea + Reader/Writer | 同一份 T12 | Adapter 无工作区且不解释知识；上层读 pinned commit | ok | `TestT12GiteaContract` |
| S-04 | local profile 无 provider | SEARCH | `CAPABILITY_UNSATISFIED`；精确 READ/VFS 不受影响 | ok | `TestLocalProfileHasNoSearchProjection` |
| S-05 | OpenSearch Retriever/Maintainer | 原子 SEARCH 算子 | MATCH=superset/partial；未声明 → `CAPABILITY_UNSATISFIED` | ok | opensearch tests |
| S-06 | 所有 Retriever | CandidateRef | 不返回正文/stored payload | ok | engine interface + search tests |

### 2.12 F HTTP 门面

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| F-01 | serve 已起 | `POST /v1/put` 再 `POST /v1/read` | 与 CLI 同一 `Invoke` | ok | `TestHTTPFacadeWriteRead` |
| F-02 | 无 allow | `X-Kc-As: bot` | `FORBIDDEN` | ok | serve_test |
| F-03 | HTTP define-workspace | 登记表 git | stamp 含 as / request-id | ok | serve_test |
| F-04 | `kc serve` 已启动 | `POST /v1/serve` / 未知动词 | 拒绝 | ok | serve_test |
| F-05 | `kc serve` 已启动 | 核心组合/治理动词（resolve / resolve-binding / inspect / checkout / proposal / merge） | HTTP 与 CLI 同语义 | ok | CLI Binding E2E / `TestUserJourneyGovernedPublishOverHTTP`；HTTP 走同一 command table |
| F-06 | MCP | — | 未实现 | **frozen** | walkthrough D.2 |

### 2.13 D 协议已冻结、参考实现未做

这些**不要**写成正路径用例。若暴露入口，预期是 `CAPABILITY_UNSATISFIED` 或「未知命令」。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| D-01 | 任意 | `CAPABILITIES` 独立清单 | 未暴露；缺失能力必须显式 | frozen | `TestUserJourneyFrozenCommandsDoNotPretendToWork` |
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
| X-06 | 联邦 Workspace | 只 allow 一仓 | 裸知识读 fail closed；SEARCH 为授权子集 `partial`；checkout 不落第二仓 | ok | P-14 / V-13 |
| X-07 | Catalog 已归档 | 写个人仓、define Workspace | 禁 define；个人仓仍 COMMIT | ok | S6 |
| X-08 | 有 schema_ref | propose | 与 COMMIT 同一套解析 | ok | `TestSchemaRefOnPropose` |
| X-09 | 已有成功 command_id | 重放带 Hook 的命令 | REPLAYED 不打 hook | ok | P-06 |
| X-10 | 已配置 Gate 与 Hook | pre-merge | pre-merge 成功 ≠ 清单满足 | ok | M-12 |
| X-11 | 已移除 driver 名 | store-set / repo-add | 不回流成 authority/index/cache | ok | W-09 |
| X-12 | permissions Aspect 存在 | `kc allow` 与 READ | 快照可读；不放行 SELECT、不当 read 闸门 | ok | P-10 |

---

## 5. 补齐顺序（后续按这个打勾）

不要从 TPC-H 再加表。先把底座非法转移钉死，再补观察点，最后才是门面与适配器 stub。

### P0 非法转移 / 错误码（系统可能静默成功的洞）

- [x] **K-14** 两文件同一 Address → `OBJECT_ID_CONFLICT`（FileGit 扫描路径）
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
- 不要把 `permissions` Aspect 做成 `kc read` 闸门
- 不要把场景套件跑进 `kc validate`

---

## 6. 最小走通脚本（补齐时的手工对照）

自动化以 `make test` / `make test-all` 为准。Dolt/Gitea 合同归各自 adapter 包；Gitea testkit 由使用包的 `TestMain` 回收容器。手工只用来核对「进入的状态」七列，不代替测试。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
H=/tmp/kc-base-catalog
rm -rf "$H"
kc() { go run ./cmd/kc -- --home "$H" "$@"; }

kc init --catalog acme/catalog          # W1
kc read --catalog
kc repo-add --repo kr://acme/public/core
kc put --command-id u1 --repo kr://acme/public/core \
  --object runbooks/oncall --value '{"text":"freeze"}' --origin-kind SOURCE
kc define-workspace --workspace agent --revision 1 \
  --source kr://acme/public/core=refs/heads/main
kc read --workspace agent --object runbooks/oncall
kc resolve --workspace agent                 # 无 --object → pin
kc inspect --workspace agent
# 非法
kc repo-add --repo kr://acme/catalog    # 必须失败
kc read --workspace agent --repo kr://acme/public/core --object runbooks/oncall
```

具体业务故事由墙外知识提供方维护，不并进本目录。数仓材料只在 `.data/data-warehouse/` 黑盒 integration suite 中维护。
