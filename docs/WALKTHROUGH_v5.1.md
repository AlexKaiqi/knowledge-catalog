# Knowledge Catalog 全流程推演

**用 `kc` 从工作区启动、写入、定义 Workspace，走到读者跟已发布分支读取；再展开提案合入与多 Repository。**

每步写两件事：**做了哪个操作**（协议动词 + 命令），**系统进入哪些状态**。`[K-xx]` 是不变量；`[代码]` 是当前实现。

入口：

```bash
go run ./cmd/kc -- help                       # 动词与 I/O
dsh --profile dsh-loom                        # 人和 Agent 的产品入口
```

“本地”仅表示 Server 和 Store 部署在本机。下文只有 `kc local ... --home .kc` 是宿主 bootstrap；Catalog、Writer、Knowledge、Governance 命令都是 typed Client，必须经 `kc serve`。A.1 启动服务后，下文省略 `--server` / `--as`，使用当时导出的环境变量。

目标不变：第一次接入能回答 **知识放哪、怎样组合成可读 Workspace、用什么坐标访问**。分层（挂 git vs Aspect vs 索引）见 [`LAYERS.md`](LAYERS.md)。本文只使用通用知识对象；具体业务验收由墙外知识提供方维护。

---

# 0. 推演前提

## 0.1 基数

```text
本机 Server Home（默认 ./.kc；不是协议对象）
├── layout.yaml                      本机目录（repos / catalogs / projections / checkouts）
├── stores.yaml                      引擎 + 托管 host（无密码）
├── audit.jsonl                      kc 时间线（init / allow / argv / --as）
├── system.jsonl                     协议面过程账（不是知识）
├── writer.json                      command_id 幂等日志
├── control.json                     proposal / preview / validation
├── projections/                     layout.projections，工作投影（非权威）
├── checkouts/                       layout.checkouts（可丢的内部投影；不是公开产品入口）
├── catalogs/                        layout.catalogs
│   └── <encoded-catalog-id>         这一间登记表 git（catalog.yaml / workspace-*.yaml / …）
└── repos/                           layout.repos
    └── <encoded-repo-id>            本机 Dolt 知识仓库
```

调用方指名 Catalog / Repository 就操作。不要为每个 Repository 建一个 Catalog，也不要把登记表 `repo-add` 成成员库。

- **Repository 按权威边界拆**，不按文件夹或数据源机械拆。
- **Workspace 面向消费场景**，不复制知识。
- **Catalog 按组织或大域拆**（数仓 vs 文档，或两个法人），不按微服务拆。

`kc local init --catalog acme/catalog`（或 `--catalog kr://acme/catalog`）创建第一间空登记表。当前组合空间看 `kc catalog show`；改动历史看 `kc catalog audit`；`--as` / `--request-id` 写进 commit。再开一间用 `kc local catalog attach --catalog <id>`；Catalog 命令加 `--catalog` 选。空 Home 用一次性 `kc local grant bootstrap` 建立第一个管理主体，后续 `kc admin grant ...` 也经 Server。本机开发认证要求显式 principal；认证模式从 `Authorization` 验证稳定主体并禁用自报身份。MCP 还没有。权限与认证见 `docs/PERMISSIONS.md`。

默认闭环是 **接入 Repository → 写入 → `read --repo`**。Workspace 只在需要联邦拼读时再做，不要为了写入去 `define-workspace`。

## 0.2 四个对象（状态会反复出现）

| 对象 | 回答的问题 | 是否可变 |
|---|---|---|
| Catalog | 组合对象住哪（配方）？ | `kc local init` / `kc local catalog attach` 创建；不是权威、不发权 |
| Repository | 值在哪为真？哪张图、哪套 Ref？ | ref 可前移，commit 不可变 |
| WorkspaceDefinition | 怎么拼哪些 repo/已发布 selector？ | 靠 revision 演化；不是边界 |
| ResolvedWorkspace | 本次读取落在哪组 repo→commit？ | 一次命令内冻结、不落盘 |

Canonical 内容在成员 Repository。Catalog 只登记组合配方。

## 0.3 描述状态时看这四列

| 列 | 看什么 |
|---|---|
| 成员库 `main` | `kc local status` 的 repo head；`read --ref main` 读到的活数据 |
| 候选 Ref | `propose` 写入的 branch；未 merge 前 main 不动 |
| Catalog 登记表 | Workspace；当前态 `kc catalog show`（`catalogId` / `repositories` / `workspaces`）；历史 `kc catalog audit` |
| 读者 | `read --workspace`；跟 Workspace 的已发布 selector；一次命令内冻结 |

---

# Phase A　从零写入并给读者跟已发布分支

场景：Alice 先把值班知识放进个人库，再让值班 Agent 读 Workspace（跟 `main`）。一次命令内冻结；发布者再 COMMIT 后，**下次**命令才看见新内容。

下文用占位符：`U1`/`U2` = 成员库 commit。真实输出是 40 位 git hash。

## A.1 启动工作区，挂上第一个 Repository

**操作** 工作区 init + 挂载成员库（不是协议写面）。

```bash
go run ./cmd/kc -- local init --home .kc --catalog acme/catalog
go run ./cmd/kc -- local repository attach --home .kc --repo kr://acme/personals/alice
go run ./cmd/kc -- local grant bootstrap --home .kc --principal user:local-admin
go run ./cmd/kc -- local status --home .kc # 宿主布局，不是 Catalog 正文
go run ./cmd/kc -- serve --home .kc        # 终端 A

export KC_SERVER_URL=http://127.0.0.1:8080 # 终端 B
export KC_AS=user:local-admin
go run ./cmd/kc -- catalog show            # 当前组合空间
go run ./cmd/kc -- catalog audit           # 登记表 git 历史
```

**进入状态**

| 项 | 值 |
|---|---|
| Catalog 库 | `kr://acme/catalog` 已存在；git 有 `init kr://acme/catalog`；无 Workspace |
| Catalog git | `init kr://acme/catalog`；无 Workspace |
| `.kc/audit.jsonl` | facade：已记下 `init` |
| `.kc/system.jsonl` | 协议面过程账：Catalog `init` 出生 |
| 成员库 | `kr://acme/personals/alice` 已挂载，`main` = root（空知识） |
| 读者 | 没有 Workspace，`kc knowledge read --workspace` 会失败 |

把 `kr://acme/catalog` 交给 `kc local repository attach` 会被拒绝：登记表不是成员 Workspace 的 source。

- `[代码]` `kc local init` / `kc local repository attach` / `Registry` ✅

## A.2 写入知识（Writer：PUT + COMMIT）

导入不是把目录直接变成线上知识。`kc writer ingest --dir` 只出 ChangeSet 预览（有 frontmatter 的用里面的 `object_id`，不用路径）；确认后 `kc writer commit --changeset`。单条 Address 用 `put`。`--if-absent` / `--if-digest` 是 Create / Update 前置条件。

`ingest` 的 stdout 同时给出 `diagnostics`：身份来自 frontmatter 还是路径、Schema
对象数、显式绑定数、可检索绑定数，以及可行动的 warnings。`--out` 文件只保存可重放的
ChangeSet，不把诊断混进 Writer 输入。重点警告：

- `PATH_DERIVED_OBJECT_ID`：移动文件会改变身份，正式接入前补 frontmatter。
- `SCHEMA_BINDING_UNDECLARED`：精确 READ 仍成立，但 SEARCH 没有显式字段契约。
- `SCHEMA_HAS_NO_ACCESS_HINTS`：本批 Schema 已验证，但没声明 `text/filter/sort`。
- `SCHEMA_ACCESS_UNVERIFIED`：Schema 不在本次预览内；`ingest` 不越权探测既有知识，后续 COMMIT 负责解析，发布后用 `describe-schema` 验证访问提示。

因此最可验证的批量接入，是把新 `schema/*` 与绑定它的知识放进同一预览；这时诊断可在写入前确认 SEARCH readiness。

**操作** `COMMIT` 一条 Address。

```bash
go run ./cmd/kc -- writer put \
  --command-id import-alice-001 \
  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall \
  --value '{"text":"切换流量前先检查冻结窗口"}' \
  --origin-kind SOURCE \
  --source-ref 'file://~/knowledge/runbooks/payment-oncall.md'
```

同一 `--command-id` 再跑是 `REPLAYED`。内容变了必须换新 id。

**进入状态**

| 项 | 值 |
|---|---|
| 成员库 `main` | `U1`（回执 `result.newCommit`） |
| 对象 `runbooks/payment-oncall` | 在 `U1` 上存在，frontmatter 内嵌 `object_id` |
| Writer 日志 | `import-alice-001` 已记下（含当时的 CAS） |
| Catalog | 不变（仍无 Workspace） |
| 读者 | 仍无配方；只有 `kc knowledge read --ref main` 能看到 `U1` |

再 `put` 另外两个对象会得到 `U2`、`U3`。一次原子导入用 `commit --changeset`，`main` 只前进一步。

- `[K-21]` 导入必须经过 Writer，不能直写 git。
- `[K-04]` `object_id` 不在路径里。
- `[代码]` `Writer.CommitIntent` / `Propose` + authority ChangeStore/TreeStore ✅
- `[代码]` `kc writer ingest` 预览、`kc writer receipt` 查幂等日志 ✅

## A.3 验收 Canonical（Reader，不改变状态）

**操作** `READ` / `GET_PROVENANCE` / `LOG`。必须钉版本：`--commit U1` 或 `--ref refs/heads/main`。公开消费面没有 Knowledge 枚举，也没有宿主路径式 snapshot export；未来的全量导出必须先定义 typed streaming API。

```bash
go run ./cmd/kc -- knowledge read --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
# → KnowledgeValue { value, repository, commit }

go run ./cmd/kc -- knowledge provenance --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
# → ProvenanceTrace.chain（本对象信封，不是 git log）

go run ./cmd/kc -- knowledge log --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
```

**进入状态**：无。只读。失败就改 ChangeSet 再 COMMIT。

## A.4 定义 Workspace，立刻可读

**操作** `DEFINE_WORKSPACE`，然后 `kc knowledge read --workspace`。

```bash
go run ./cmd/kc -- catalog workspace define --workspace payments-agent --revision 1 \
  --source kr://acme/personals/alice=refs/heads/main

go run ./cmd/kc -- knowledge read --workspace payments-agent --object runbooks/payment-oncall
go run ./cmd/kc -- knowledge log --workspace payments-agent --object runbooks/payment-oncall
go run ./cmd/kc -- catalog audit --workspace payments-agent
```

`ResolveWorkspace` 在这次命令开始时解析一次 `main`。命令进行中 `main` 再前移，这次结果仍指向开始时的 commit。下次 `kc knowledge read --workspace` 会解到新 HEAD。

**进入状态**

| 项 | `define-workspace` 之后 |
|---|---|
| 成员库 `main` | 仍 `U1` |
| Workspace `payments-agent` | revision 1 已登记 |
| Catalog git | 一条 `define-workspace` |
| 读者 `kc knowledge read --workspace` | `U1` 上的值 |

- `[代码]` `Catalog.defineView` / `ResolveWorkspace` / `Registry` ✅

## A.5 检索投影

找候选只走 OpenSearch；未配置 OpenSearch 时仍有 Snapshot 精确 READ/VFS，SEARCH 明确返回 `CAPABILITY_UNSATISFIED`。Bound State 消费 READ 通过 `--resource-access-url` / `KC_RESOURCE_ACCESS_URL` 接入独立 runtime 服务。工作投影按**仓和 basis commit**建、不按 Workspace；经 `Catalog.Hook`（`AfterSnapshot`）增量更新。Provider 只返回 CandidateRef，公开结果回读同一 commit 的 Canonical；Workspace 命中随后 hydrate State Binding，并携带 completeness/claims/version/evidence/observation。CLI：`kc knowledge search`、`kc operations projection describe|sync`、`kc operations access describe`。跨仓 SEARCH 是扇出，不把联邦结果抄成一个索引；动态 State 字段本身尚不参与候选发现。

SEARCH 不是“整包 JSON contains”。接入方必须先把可访问字段声明为知识，并让正文绑定
对应 `schema_ref`；最小可执行例见 README 的 Quickstart。排障顺序固定为：

```bash
go run ./cmd/kc -- operations access describe --workspace payments-agent
# fields 为空：先补 schema/* 的 text/filter/sort AccessHints，并让正文绑定 schema_ref

go run ./cmd/kc -- operations projection describe --repo kr://acme/personals/alice
# 核对 pin、AccessPlan、每仓 index basis/lag

go run ./cmd/kc -- knowledge search --workspace payments-agent --query 冻结窗口
```

`CAPABILITY_UNSATISFIED` 表示逻辑访问声明或物理 provider 不满足这次查询，不等于“零命中”；
真正的零命中仍返回 `completeness` 和空 `hits`。有成员因 provider 能力不足或授权被裁剪时，
结果必须是 `partial`，不能宣称完整。

**进入状态**：配方不变。索引不是权威；命中后仍按这次解开的 commit 回读 Canonical。

- `[K-19]` Projection 可丢、可重建，必须声明 basis/lag。
- `permissions` Aspect 进 Canonical；检索面走 AccessHints（GRANT 正文通常无 `text`）。

## A.6 Agent 应跟 Workspace，一次命令内冻结

推荐配置只保存 `catalog=kr://acme/catalog`、`workspace=payments-agent`。一次请求先 `ResolveWorkspace`，后续 READ / SEARCH / PROVENANCE 复用同一组 commit。CLI 可先 `resolve --workspace > pin.json`，再给所有 Workspace 消费动词传 `--pin pin.json`；不传时每条新命令会有意重新跟随 selector。

当前 CLI 消费侧只有 `kc knowledge search/read/relations/provenance/log/schema describe/binding resolve`。Workspace、身份与固定 pin 由任务宿主注入，冲突的显式坐标会被拒绝；没有 Knowledge LIST、checkout 或 snapshot-export fallback。知识目录通过 `kcfs` 经 Workspace File Gateway 只读 mount 给 `rg`。`kc catalog show` 是组合空间当前态；`kc catalog audit` 是登记表 git，不是对象历史。人和 Agent 通过 DSH 插件进入；`kc serve` 只保留正式 HTTP API 和基础设施端点，不提供操作台。MCP 网关尚未实现。

**进入状态**：无（读）。Agent 不自己选“最新 commit”；跨命令自然跟已发布分支。

## A.7 引用长什么样

```text
问题：切换支付流量前要检查什么？
回答：先确认不在冻结窗口，再按 payment-oncall 执行。
引用：kr://acme/personals/alice @ U1 / runbooks/payment-oncall
```

commit 仍出现在 `FederatedValue` / citation。

## A.8 更新知识，下次读才看见

**操作** 再 `COMMIT` 一次（换新 `command-id`）。

```bash
go run ./cmd/kc -- writer put \
  --command-id import-alice-002 \
  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall \
  --value '{"text":"先核对冻结窗口，再通知 payments oncall"}' \
  --origin-kind SOURCE

go run ./cmd/kc -- knowledge read --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --ref refs/heads/main
# 活数据 = 新正文

go run ./cmd/kc -- knowledge read --workspace payments-agent \
  --object runbooks/payment-oncall
# 新命令：解到 U2，看见新正文
```

已经开始的那次 `kc knowledge read --workspace`（若跨多步）仍钉在命令开始时的 commit。新开一条命令才解新 HEAD。

- `[K-11]` 命令内不跟随 latest。

## A.9 再挂团队库（不建第二套 Catalog）

**操作** 再 `kc local repository attach` + 写入 + 提高 Workspace revision。

```bash
go run ./cmd/kc -- local repository attach --repo kr://acme/public/core
go run ./cmd/kc -- local repository attach --repo kr://acme/groups/payments

go run ./cmd/kc -- writer put --command-id pub-1 --repo kr://acme/public/core \
  --object policy/P-103 --value '{"statement":"production requires owned runbook"}'
go run ./cmd/kc -- writer put --command-id grp-1 --repo kr://acme/groups/payments \
  --object policy/P-103 --value '{"statement":"applies within production"}'

go run ./cmd/kc -- catalog workspace define --workspace payments-agent --revision 3 \
  --source kr://acme/public/core=refs/heads/main \
  --source kr://acme/groups/payments=refs/heads/main \
  --source kr://acme/personals/alice=refs/heads/main
```

**进入状态**

| 项 | 值 |
|---|---|
| 三个成员库 `main` | 各自独立 head（如 `P1` / `R1` / `U2`） |
| Workspace | revision 3，三个 source |
| 下次 `kc knowledge read --workspace --object policy/P-103` | **两条** FederatedValue，不覆盖 |

- `[K-12]` `[K-13]` 多来源并存，不按 public/group/personal 覆盖。

## A.10 接入完成的判据（状态清单）

```text
kc local init --catalog …            → 空 Catalog 登记表（id 就是这一间）
kc local repository attach                    → 成员库已挂载，main = root
kc writer put / commit                → 成员库 main = 不可变 commit
kc catalog workspace define                 → 配方已登记
kc knowledge read --workspace                 → 读者解已发布 selector，读到这次冻结的 commit
（可选）再 put                 → main 前进；下次 read --workspace 看见新内容
```


---

# Phase B　单 source 时的协议边角（同一套 `kc`）

不是另一套语义。Workspace 只有一个 source 时，ResolvedWorkspace 的 Map 长度为 1。

## B.1 路径移动，身份不变

**操作** 对同一 `object_id` 再 PUT，换 `path-hint`。

```bash
go run ./cmd/kc -- writer put --command-id move-1 \
  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall \
  --value '{"text":"…"}' \
  --path-hint notes/oncall-v2.json
```

**进入状态**：`main = U'`；`kc knowledge read` 仍按同一 `object_id` 解析，`pathHint` 更新。KnowledgeRef 不跟路径走。`[T1]` `[K-04]`

## B.2 对象历史 vs 来源信封

```bash
go run ./cmd/kc -- knowledge log --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --ref refs/heads/main
# 引入各 digest 的 commit；后面没改这个对象的 commit 不占一条

# 公开 Client 暂无 DIFF route；需要时应增加 typed Knowledge/Operations API，
# 不应让 CLI 直开 Home 调内部 diff handler。
# 两个 pinned commit 上的对象值

go run ./cmd/kc -- knowledge provenance --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U2
# 单元信封。不是 git log，也不爬 sourceRefs
```

**进入状态**：无（只读）。`[K-12]`

## B.3 Aspect Binding（② 声明；State 逻辑 READ 经墙外 runtime）

**操作** PUT 一个带 `value_source.kind=binding` 的 Aspect。它推进 Snapshot commit，因为稳定访问声明本身是知识；它不调用 runtime，也不把瞬时值塞进 Catalog pin。

```bash
go run ./cmd/kc -- writer put --command-id bind-health \
  --repo kr://acme/personals/alice \
  --object Service:payments --aspect health --value null \
  --value-source '{"kind":"binding","binding":{"mode":"state","runtime":"payments","protocol":"mcp","operations":{"read":{"call":"health.read"}}}}'

go run ./cmd/kc -- knowledge binding resolve --repo kr://acme/personals/alice \
  --object Service:payments --aspect health --ref refs/heads/main
```

**进入状态**

| 项 | 值 |
|---|---|
| 成员库 `main` | 前进到声明 commit |
| ResolvedBinding | declarationCommit + declarationDigest + mode/runtime/protocol/operations |
| live observation | 不在底座；上层 runtime 另行固定 generation/cut |
| 后续声明变化 | 独立 DeclarationDigest，旧 pin 仍解析旧声明 |

- `resolve-binding` 始终只返回声明。面向消费者的 `read --workspace` 由 Knowledge Server 经
  `resource-access/v1` 调用独立 runtime 服务，返回绑定值与 declaration/observation 双 basis；
  未配置 runtime 时返回 `CAPABILITY_UNSATISFIED`，不会返回 `null` 冒充业务值。
- VFS/checkout/`read --repo` 仍是固定 Snapshot/声明视图，不调用 runtime。
- Stream Binding 不进入普通 READ；后续使用显式 window/query surface。
- 动态观察若需要沉淀，由 Collector 显式翻译为 ChangeSet 再 COMMIT。

## B.4 INGEST / RECONCILE（API，CLI 尚未摊开）

`ingest` / `reconcile` 只出 ChangeSet 预览，确认后：

```bash
go run ./cmd/kc -- writer commit --command-id rec-1 --changeset preview.json
```

**进入状态**：与 A.2 相同——只推进成员库 Ref。`[K-21]`

---

# Phase C　提案合入（内容仍经 Writer；merge 后下次读可见）

人工改知识走这条。无人值守同步继续用 A 的 `put`，不要用 PROPOSAL。

假定 A.4 之后：`main = U1`，Workspace `payments-agent` 跟 `refs/heads/main`。

## C.1 PROPOSE：只写候选 Ref

**操作** `PROPOSAL` = 对 candidate Ref 做 `COMMIT`。

```bash
go run ./cmd/kc -- governance proposal create \
  --proposal-id PR-42 \
  --repo kr://acme/personals/alice \
  --target refs/heads/main \
  --candidate refs/heads/candidates/PR-42 \
  --object runbooks/payment-oncall \
  --value '{"text":"候选：先通知 payments，再切流"}'
```

**进入状态**

| 项 | 值 |
|---|---|
| `main` | 仍 `U1` |
| `refs/heads/candidates/PR-42` | `C1`（新 commit） |
| `read --ref main` | 旧正文 |
| `read --workspace` | 旧正文（main 还没动） |

- `[K-07]` Proposal 不改 target Ref。

## C.2 PREVIEW：完整一代，只换这一个成员

**操作** `CREATE_PREVIEW`。

```bash
go run ./cmd/kc -- governance preview create --proposal PR-42 --workspace payments-agent
# → previewId，repositories = {alice: C1}；只写 ControlState
```

**进入状态**：`.kc/control.json` 多一个 Preview（其余成员若已在 Workspace 里则保持）。`main` 仍不动。登记表不增加 pin yaml。

- `[K-09]` 校验必须绑这一完整 Preview，不能只绑候选 Repository。

## C.3 VALIDATE：结构门禁 vs 外部套件

**操作** `validateStructure`（真的检查：成员库已挂载、commit 还在），然后可选 `recordValidation`（只记录外来结果，不跑套件）。

```bash
go run ./cmd/kc -- governance preview validate --preview preview-<id>
# → reportId、outcome=PASSED|FAILED、check.issues

go run ./cmd/kc -- governance validation record --preview preview-<id> \
  --suite S7 --outcome PASSED
```

**进入状态**：`control.json` 多一条 ValidationReport。任何 Ref 都不动。`FAILED` 不能 merge。自定义套件仍走 `record-validation`，不进 `validate`。多条必过清单：`kc operations gate add --on merge --repo … --require validate,suite:<名>`（见 `docs/GATES.md`）。无 `gates.json` 时本推演仍是单门雏形（`merge --validation`）。

## C.4 MERGE：快进 main，下次 read --workspace 可见

```bash
go run ./cmd/kc -- governance proposal merge \
  --proposal PR-42 \
  --preview preview-<id>
```

这里假定已配置上节的 Gate；`merge` 自动检查绑定该 Preview 的 structure 与
外部 suite 证据，并从 Proposal 推导目标 Repository/Ref 的授权范围。若没有
匹配 Gate，则追加单个 `--validation val-<id>`。成功回执会明确给出
Repository、target Ref、Preview basis 和 required checks。

**进入状态**

| 项 | 值 |
|---|---|
| `main` | `C1`（与候选相同） |
| 候选 Ref | 仍指向 `C1` |
| `read --ref main` | 候选正文 |
| 下次 `read --workspace` | **新正文** |

candidate 若在校验后又被提交，merge 返回 `CANDIDATE_MOVED`。main 若已被别人推走，返回 `NON_FAST_FORWARD`。

- `[K-06]` Merge 是 Ref CAS。

## C.5 回退与配方修正

没有第二步「给读者切一代」。权威内容回退要在成员库再 COMMIT/REVERT。配方错了就 `define-workspace` 改 selector。

```text
Projection 错 → 重建索引（不进这条 CLI）
Serving 组合错 → kc catalog workspace define（改配方）
权威内容错 → 成员库再 put / commit（保留历史）
```

## C.6 三层回滚不要混

见上。**Phase C 结论**：提案闭环改变的是 **候选 → main**；下次 `read --workspace` 自然解到新 HEAD。

---

# Phase D　推演总评

## D.1 用命令能走通的闭环

```text
init / repo-add     工作区 + 成员库
put / commit        成员库 Ref
put value_source / resolve-binding   动态访问声明（观察在墙外）
resolve / read / provenance / list / log / diff
define-workspace / read --catalog / read --workspace / audit
propose / preview / validate / record-validation / merge
```

单 source 只是 ResolvedWorkspace Map 长度为 1。一次命令内解 Snapshot commit。Catalog 登记表是独立 Git registry，define-workspace 历史即该库 git log。

## D.2 能力对照（命令行 vs 仍缺）

| 能力 | 状态 | 归属 |
|---|---|---|
| `ingest` / `reconcile` 出预览 | `kc writer ingest` 有；`reconcile` 仍 API | Writer 之上的薄编排 |
| 跨成员 `search` / Projection 调度 | `kc knowledge search` 单仓 pinned commit；跨仓是扇出，无联邦抄写索引 | `index/` + Reader |
| HTTP service | `kc serve`：按 Catalog、Knowledge、Writer、Governance、Admin、Operations 分区注册 typed API；无自带 UI | Application |
| 人 / Agent 入口 | 分组 `kc` CLI + 普通宿主文件工具；`dsh-plugin/` 只提供 Skill、MountController 与人用只读浏览 | Application |
| MCP Agent 网关 | 无 | Application |
| `kc admin grant add` / `--as` / 仓级 ACL | `.kc/allow.json`；见 `docs/PERMISSIONS.md` | facade 求值；authority 本身不代替 KC 授权 |
| `kc hook-*` | `.kc/hooks.json`；见 `docs/HOOKS.md` | 出站调用户系统 |
| `kc gate-*` | `.kc/gates.json`；见 `docs/GATES.md` | `merge` 查证据清单 |
| source key → object_id | 无 | 场景 / 外部 Connector，不进仓库根 |
| 外部 STATE 的 Address 对账 | `connector.Preview` | Collector helper；无 `kc` 动词。见 `docs/CONNECTORS.md` |

## D.3 最终判断

> **参考实现里，`kc local` 只完成宿主 bootstrap；从 Writer 到 `read --workspace` 的语义闭环始终是 Client → Server。**

`make test` 跑 component、分层边界和应用/transport 合同；`make test-service-e2e` 验收真实 Server/Client 旅程，`make test-all` 再跑
Gitea、Dolt、OpenSearch 与 Linux/FUSE。受跟踪的数仓提供方 integration suite 在
`.data/data-warehouse/` 中通过公开 `kc` surface 独立验收，不参与根模块测试；其
`runs/` 执行证据不提交。
