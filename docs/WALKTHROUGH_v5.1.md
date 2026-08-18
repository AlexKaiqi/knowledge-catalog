# Knowledge Catalog 全流程推演

**用 `kc` 从工作区启动、写入、发布，走到读者在固定版本上读取；再展开提案合入与多 Repo。**

每步写两件事：**做了哪个操作**（协议动词 + 命令），**系统进入哪些状态**。`[K-xx]` 是不变量；`[代码]` 是当前实现。

入口：

```bash
npm run kc -- help          # 动词与 I/O
# 默认 --home 是 ./.kc，下文省略
```

目标不变：第一次接入能回答 **知识放哪、怎样发布成稳定可读版本、用什么坐标访问**。

---

# 0. 推演前提

## 0.1 基数

```text
工作区 ./.kc                         （进程胶水，不是第四种写面）
├── workspace.json                   namespace + 成员库挂载
├── writer.json                      command_id 幂等日志
├── control.json                     proposal / preview / validation
└── repos/
    ├── _catalog                     Catalog 登记表 FileGit
    │                                kr://<namespace>/catalog
    └── <encoded-repo-id>            成员知识库 FileGit
```

一个 namespace 管一组成员 Repository，以及它们的 View、Generation、Release。不要为每个 Repo 建一个 Catalog，也不要把 `_catalog` `repo-add` 成成员库。

- **Repository 按权威边界拆**，不按文件夹或数据源机械拆。
- **View 面向消费场景**，不复制知识。

`kc init --namespace acme` 对应产品管理面上的 bootstrap。还没有多租户 `createCatalog`、WriteBinding、HTTP/MCP。

## 0.2 五个对象（状态会反复出现）

| 对象 | 回答的问题 | 是否可变 |
|---|---|---|
| Catalog Namespace | 哪个组织的组合空间？ | 配置可变 |
| Repository | 谁负责、在哪独立提交？ | ref 可前移，commit 不可变 |
| ViewDefinition | 消费者要组合哪些 repo/selector？ | 靠 revision 演化 |
| ViewGeneration | 本次读取落在哪组 repo→commit？ | 不可变 |
| Release | 当前对读者发布哪一代？ | CAS 移动 |

Canonical 内容在成员 Repository。Catalog 只登记组合与指针。

## 0.3 描述状态时看这四列

| 列 | 看什么 |
|---|---|
| 成员库 `main` | `kc status` 的 repo head；`read --ref main` 读到的活数据 |
| 候选 Ref | `propose` 写入的 branch；未 merge 前 main 不动 |
| Catalog 登记表 | View / Generation / Release；`kc log --catalog` |
| 读者 | `read-release`；跟 Release，不跟 `main` |

---

# Phase A　从零写入并发布给读者

场景：Alice 先把值班知识放进个人库，再让值班 Agent 读 **可重放** 的一代，而不是漂着的 `main`。

下文用占位符：`U1`/`U2` = 成员库 commit，`G1`/`G2` = Generation id。真实输出是 40 位 git hash / sha256。

## A.1 启动工作区，挂上第一个 Repository

**操作** 工作区 init + 挂载成员库（不是协议写面）。

```bash
npm run kc -- init --namespace acme
npm run kc -- repo-add --repo kr://acme/personals/alice
npm run kc -- status
```

**进入状态**

| 项 | 值 |
|---|---|
| namespace | `acme` |
| Catalog 库 | `kr://acme/catalog` 已存在，只有 root commit；无 View / Release |
| 成员库 | `kr://acme/personals/alice` 已挂载，`main` = root（空知识） |
| 读者 | 没有 Release，`read-release` 会失败 |

`repo-add kr://acme/catalog` 会被拒绝：登记表不是成员 View 的 source。

- `[代码]` `kc init` / `repo-add` / `GitCatalogRegistry` ✅

## A.2 写入知识（Writer：PUT + COMMIT）

导入不是把目录直接变成线上知识。`ingest()` 仍是 API，只出 ChangeSet 预览；确认后走 Writer。CLI 上等价于 `put`（单 Address）或 `commit --changeset`（一批）。

**操作** `COMMIT` 一条 Address。

```bash
npm run kc -- put \
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
| Catalog / Release | 不变（仍无发布） |
| 读者 | 仍无 `stable`；只有 `read --ref main` 能看到 `U1` |

再 `put` 另外两个对象会得到 `U2`、`U3`。一次原子导入用 `commit --changeset`，`main` 只前进一步。

- `[K-21]` 导入必须经过 Writer，不能直写 git。
- `[K-04]` `object_id` 不在路径里。
- `[代码]` `Writer.commitIntent` + FileGit `applyCommit` ✅

## A.3 验收 Canonical（Reader，不改变状态）

**操作** `RESOLVE` / `READ` / `GET_PROVENANCE` / `LIST` / `LOG`。必须钉版本：`--commit U1` 或 `--ref refs/heads/main`。

```bash
npm run kc -- resolve --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
# → Resolution { status: RESOLVED }，没有正文

npm run kc -- read --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
# → KnowledgeValue { value, repository, commit }

npm run kc -- provenance --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
# → ProvenanceTrace.chain（本对象信封，不是 git log）

npm run kc -- list --repo kr://acme/personals/alice --commit U1
npm run kc -- log  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U1
```

**进入状态**：无。只读。失败就改 ChangeSet 再 COMMIT，不要去改已经发布的 Generation（此时也还没有）。

## A.4 定义 View，发布第一个 Generation

**操作** `DEFINE_VIEW`，再 `PIN_VIEW` + `PROMOTE`（`promote --view` = 现在 pin 再 CAS Release）。

```bash
npm run kc -- define-view --view payments-agent --revision 1 \
  --source kr://acme/personals/alice=refs/heads/main

npm run kc -- promote --release stable --view payments-agent
# → { release: "stable", generationId: G1 }

npm run kc -- read-release --release stable --object runbooks/payment-oncall
npm run kc -- log --catalog --release stable
```

`pin-view` 只在此刻解析一次 `main`。之后 `main` 前移，`G1` 仍指向 `U1`。

**进入状态**

| 项 | `define-view` 之后 | `promote --view` 之后 |
|---|---|---|
| 成员库 `main` | 仍 `U1` | 仍 `U1` |
| View `payments-agent` | revision 1 已登记 | 同左 |
| Generation | 无（配方还没钉） | `G1 = {alice: U1}` |
| Release `stable` | 无 | `→ G1` |
| Catalog git | 一条 `define-view` | 再一条 `promote stable` |
| 读者 `read-release` | 失败 | `U1` 上的值 |

- `[代码]` `Catalog.defineView` / `publish` / `GitCatalogRegistry` ✅

## A.5 检索投影（未进 CLI）

规模化找候选需要对 `G1` 里每个 `repo@commit` 建 Projection。当前是 `SqliteProjection.build`，没有 `kc search`。

**进入状态**：Serving 指针不变。索引不是权威；命中后仍按 `G1` 回读 Canonical。

- `[K-19]` Projection 可丢、可重建，必须声明 basis/lag。
- `[缺口]` 无 Catalog 级索引调度，无 `kc search`。

## A.6 Agent 应跟 Release，不应跟 main

推荐配置只保存 `namespace=acme`、`release=stable`。一次请求先解析 Release → Generation，后续 READ/PROVENANCE 复用同一个 `generationId`。

当前 CLI 对应读者侧的是 `read-release` / `provenance`（后者要自己带 Generation 里的 commit）。HTTP/MCP 网关尚未实现。

**进入状态**：无（读）。Agent 不保存 `main`，也不自己选“最新 commit”。

## A.7 引用长什么样

```text
问题：切换支付流量前要检查什么？
回答：先确认不在冻结窗口，再按 payment-oncall 执行。
引用：kr://acme/personals/alice @ U1 / runbooks/payment-oncall
      generation: G1
```

`main` 已更新后仍能重放当时的依据。

## A.8 更新知识，但不让读者立刻看到半成品

**操作** 再 `COMMIT` 一次（换新 `command-id`）。

```bash
npm run kc -- put \
  --command-id import-alice-002 \
  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall \
  --value '{"text":"先核对冻结窗口，再通知 payments oncall"}' \
  --origin-kind SOURCE

npm run kc -- read --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --ref refs/heads/main
# 活数据 = 新正文

npm run kc -- read-release --release stable \
  --object runbooks/payment-oncall
# 仍是 G1 / U1 的旧正文
```

**进入状态（写完、未再 promote）**

| 项 | 值 |
|---|---|
| 成员库 `main` | `U2` |
| Release `stable` | 仍 `→ G1 → {alice: U1}` |
| `read --ref main` | 新正文 |
| `read-release` | 旧正文 |
| `log` 该对象 | 两条引入 commit：`U2`，`U1` |

确认要给读者看了，再发布：

```bash
npm run kc -- promote --release stable --view payments-agent --expected G1
# → stable → G2 = {alice: U2}
```

**进入状态**：`stable → G2`；新请求读 `U2`；已经按 `G1` 钉住的进行中请求继续读 `U1`。

Promotion CAS 失败则继续服务旧 Generation。Commit 失败不产生部分对象。

- `[K-22]` Promote 不改成员库。

## A.9 再挂团队库（不建第二套 Catalog）

**操作** 再 `repo-add` + 写入 + 提高 View revision + promote。

```bash
npm run kc -- repo-add --repo kr://acme/public/core
npm run kc -- repo-add --repo kr://acme/groups/payments

npm run kc -- put --command-id pub-1 --repo kr://acme/public/core \
  --object policy/P-103 --value '{"statement":"production requires owned runbook"}'
npm run kc -- put --command-id grp-1 --repo kr://acme/groups/payments \
  --object policy/P-103 --value '{"statement":"applies within production"}'

npm run kc -- define-view --view payments-agent --revision 3 \
  --source kr://acme/public/core=refs/heads/main \
  --source kr://acme/groups/payments=refs/heads/main \
  --source kr://acme/personals/alice=refs/heads/main

npm run kc -- promote --release stable --view payments-agent --expected G2
```

**进入状态**

| 项 | 值 |
|---|---|
| 三个成员库 `main` | 各自独立 head（如 `P1` / `R1` / `U2`） |
| View | revision 3，三个 source |
| Generation `G3` | `{public:P1, group:R1, personal:U2}` |
| `stable` | `→ G3` |
| `read-release --object policy/P-103` | **两条** FederatedValue，不覆盖 |

- `[K-12]` `[K-13]` 多来源并存，不按 public/group/personal 覆盖。

## A.10 接入完成的判据（状态清单）

```text
kc init --namespace …          → namespace + 空 Catalog 登记表
kc repo-add                    → 成员库已挂载，main = root
kc put / commit                → 成员库 main = 不可变 commit
kc define-view                 → 配方已登记
kc promote --view              → Generation 已钉死，Release 已指向它
kc read-release                → 读者读到 pinned 一代
（可选）再 put                 → main 前进，Release 不动
（可选）再 promote             → 读者切到新一代
```

---

# Phase B　单 source 时的协议边角（同一套 `kc`）

不是另一套语义。View 只有一个 source 时，Generation 的 Map 长度为 1。

## B.1 路径移动，身份不变

**操作** 对同一 `object_id` 再 PUT，换 `path-hint`。

```bash
npm run kc -- put --command-id move-1 \
  --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall \
  --value '{"text":"…"}' \
  --path-hint notes/oncall-v2.json
```

**进入状态**：`main = U'`；`resolve` 仍 `RESOLVED`，`object_id` 不变，`pathHint` 更新。KnowledgeRef 不跟路径走。`[T1]` `[K-04]`

## B.2 对象历史 vs 来源信封

```bash
npm run kc -- log --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --ref refs/heads/main
# 引入各 digest 的 commit；后面没改这个对象的 commit 不占一条

npm run kc -- diff --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --from U1 --to U2
# 两个 pinned commit 上的对象值

npm run kc -- provenance --repo kr://acme/personals/alice \
  --object runbooks/payment-oncall --commit U2
# 单元信封。不是 git log，也不爬 sourceRefs
```

**进入状态**：无（只读）。`[K-12]`

## B.3 APPEND（与 snapshot 并行）

**操作** `APPEND`。不进 git tree，不改 `main`，不改 Release。

```bash
npm run kc -- append --command-id run-1 \
  --repo kr://acme/personals/alice \
  --stream evidence --event-id evt-1 \
  --payload '{"outcome":"PASSED"}'

npm run kc -- stream --repo kr://acme/personals/alice --stream evidence
```

**进入状态**

| 项 | 值 |
|---|---|
| 成员库 `main` / Release | 不变 |
| stream `evidence` | cursor = `1`，一条 `evt-1` |
| 同 eventId + 同 payload | `REPLAYED` |
| 同 eventId + 异 payload | `EVENT_ID_CONFLICT` |

- `[K-17]` Entry 不原地修订。

## B.4 INGEST / RECONCILE（API，CLI 尚未摊开）

`ingest` / `reconcile` 只出 ChangeSet 预览，确认后：

```bash
npm run kc -- commit --command-id rec-1 --changeset preview.json
```

**进入状态**：与 A.2 相同——只推进成员库 Ref。`[K-21]`

---

# Phase C　提案合入（内容仍经 Writer；merge 不发布）

人工改知识走这条。无人值守同步继续用 A 的 `put`，不要用 PROPOSAL。

假定 A.4 之后：`main = U1`，`stable → G1 = {alice: U1}`。

## C.1 PROPOSE：只写候选 Ref

**操作** `PROPOSAL` = 对 candidate Ref 做 `COMMIT`。

```bash
npm run kc -- propose \
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
| Release `stable` | 仍 `→ G1` |
| `read --ref main` | 旧正文 |
| `read-release` | 旧正文 |

- `[K-07]` Proposal 不改 target Ref。

## C.2 PREVIEW：完整一代，只换这一个成员

**操作** `CREATE_PREVIEW`。

```bash
npm run kc -- preview --proposal PR-42 --view payments-agent
# → previewId，generation = {alice: C1}
```

**进入状态**：Catalog 多一个 Preview Generation（其余成员若已在 View 里则保持）。`main` 与 `stable` 仍不动。

- `[K-09]` 校验必须绑这一完整 Preview，不能只绑候选 Repo。

## C.3 VALIDATE：结构门禁 vs 外部套件

**操作** `validateStructure`（真的检查：成员库已挂载、commit 还在），然后可选 `recordValidation`（只记录外来结果，不跑套件）。

```bash
npm run kc -- validate --preview preview-<id>
# → reportId、outcome=PASSED|FAILED、check.issues

npm run kc -- record-validation --preview preview-<id> \
  --suite S7 --outcome PASSED
```

**进入状态**：`control.json` 多一条 ValidationReport。任何 Ref、Release 都不动。`FAILED` 不能 merge。

## C.4 MERGE：快进 main，Release 不动

```bash
npm run kc -- merge \
  --proposal PR-42 \
  --preview preview-<id> \
  --validation val-<id>
```

**进入状态**

| 项 | 值 |
|---|---|
| `main` | `C1`（与候选相同） |
| 候选 Ref | 仍指向 `C1` |
| Release `stable` | **仍 `→ G1 → U1`** |
| `read --ref main` | 候选正文 |
| `read-release` | **仍是旧正文** |

candidate 若在校验后又被提交，merge 返回 `CANDIDATE_MOVED`。main 若已被别人推走，返回 `NON_FAST_FORWARD`。

- `[K-06]` Merge 是 Ref CAS。

## C.5 再 PROMOTE：读者才切到新一代

```bash
npm run kc -- promote --release stable --view payments-agent --expected G1
```

**进入状态**：`stable → G'`（`{alice: C1}`）。`read-release` 现在是新正文。

回滚只动指针：

```bash
npm run kc -- rollback --release stable --expected G' --prior G1
```

**进入状态**：`stable → G1`；成员库 `main` 仍是 `C1`。权威内容回退要在成员库再 COMMIT/REVERT，不要拿 rollback 当删内容。

```text
Projection 错 → 重建索引（不进这条 CLI）
Serving 组合错 → kc rollback（不动 Repo）
权威内容错 → 成员库再 put / commit（保留历史）
```

## C.6 三层回滚不要混

见上。**Phase C 结论**：提案闭环改变的是 **候选 → main**；读者切版本必须另一次 **promote**。

---

# Phase D　推演总评

## D.1 用命令能走通的闭环

```text
init / repo-add     工作区 + 成员库
put / commit        成员库 Ref
append / stream     事件流（并行）
resolve / read / provenance / list / log / diff
define-view / pin-view / promote / rollback / read-release / log --catalog
propose / preview / validate / record-validation / merge
```

单 source 只是 Generation Map 长度为 1。协议层只依赖 `Repository`。Catalog 登记表是独立 FileGit，promote 历史即该库 git log。

## D.2 本推演已按 CLI 改写后仍未进命令行的能力

| 能力 | 状态 | 归属 |
|---|---|---|
| `ingest` / `reconcile` 出预览 | API 有，无 `kc ingest` | Writer 之上的薄编排 |
| 跨成员 `search` / Projection 调度 | API 有单库 FTS，无 `kc search` | Reader / Projection |
| HTTP/MCP Agent 网关 | 无 | Application |
| WriteBinding / ACL | 无 | 协议层，先问归属 |
| source key → object_id | 无 | 场景 Collector，不进仓库根 |

## D.3 最终判断

> **参考实现里，从 `kc init` 到 `read-release` 的语义闭环可以用命令走通；Agent 网关和检索编排仍是产品层。**

`npm test` 跑仓库根 conformance + CLI walk（不收集 `.scenes/`）。场景树要同步已提交的 main：`git -C .scenes/data-warehouse merge main`。
