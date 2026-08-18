# Knowledge Catalog 全流程推演

**从 Catalog 启动、知识导入和发布，一直推演到 Agent 在固定版本上检索、读取和引用；再展开单 source 与 public/group/personal 多 Repo 形态。**

每步标注：`[K-xx]` = 不变量；`[代码]` = 当前实现对应。

目标：让第一次接入的用户能回答三件事：**我的知识放到哪里、怎样发布成稳定可读的版本、Agent 究竟通过什么坐标访问它**；同时检验协议是否只有一套、store 是否可替换、端到端是否闭环。

---

# 0. 推演前提

## 0.1 Service、Catalog 与 Repository 的基数

这里采用以下边界：

```text
Knowledge Catalog Service（可水平扩展的运行时）
└── Catalog Namespace: acme（租户/组织级逻辑边界）
    ├── Repository Registry
    │   ├── kr://acme/public/core
    │   ├── kr://acme/groups/payments
    │   └── kr://acme/personals/alice
    ├── View Definitions / immutable Generations
    └── Serving Channels: stable / preview / ...
```

- **Catalog 要初始化，但不需要为每个 Repository 创建一个 Catalog。** 一个 Catalog namespace 管理一组可独立演化的 Repository，以及它们的 View、Generation 和 Channel。
- **一个服务进程不等于一个 Catalog。** MVP 可以采用“一次部署默认一个 Catalog namespace”来降低运维复杂度；多租户实现可以让同一服务托管多个隔离 namespace。服务进程数属于部署问题，不进入协议身份。
- **Repository 按权威边界拆分，不按文件夹或数据源机械拆分。** ACL、负责人、审批流程、保留策略或发布节奏不同，才应拆成不同 Repository；同一权威下的多个来源可以导入同一个 Repository。
- **View 面向消费场景。** 同一个 Catalog 中可以有 `alice-default`、`payments-agent`、`company-public` 等多个 View，不需要复制知识。

当前参考代码以 `new Store()` + `new Catalog(store)` 完成进程内 bootstrap；`Store.addRepository()` 完成挂载。它还没有 Catalog identity、持久化 registry、`createCatalog` 管理 API、租户隔离或授权。因此下文把 `BOOTSTRAP_CATALOG` 作为产品管理面应有的动作；标注为 `[代码]` 的部分才是当前已实现能力。

## 0.2 五个容易混淆的对象

| 对象 | 回答的问题 | 是否可变 | 典型基数 |
|---|---|---|---|
| Catalog Namespace | 哪个组织/租户的知识组合空间？ | 配置可变 | 一个租户通常一个 |
| Repository | 谁对这批知识负责，在哪里独立提交？ | ref 可前移，commit 不可变 | 一个 Catalog 多个 |
| ViewDefinition | 某类消费者要组合哪些 Repository/selector？ | 通过 revision 演化 | 一个 Catalog 多个 |
| ViewGeneration | 本次读取精确落在哪组 repo→commit？ | 不可变 | 一个 View 多个历史版本 |
| Channel | 当前对消费者发布哪个 Generation？ | CAS 移动 | 每种发布轨道一个 |

Catalog 不是知识内容容器；Canonical 内容在 Repository。View 也不是复制或 merge，它只把多个 Repository 的精确 commit 组成一个不可变读取切面。

## 0.3 一套语义，多个 store 实现

Catalog 语义只有一种：Identity、Write Surface、Repository、Access、ViewDefinition→ViewGeneration、维护闭环、联邦读取。当前 store 是 `FileGitRepository`；未来可按规模新增 Dolt/PostgreSQL adapter，但协议层不变。

```text
Ingress / Access / ControlPlane / Catalog
                  │
                  ▼
             Repository 接口
                  │
                  ▼
        FileGit（当前） / Dolt（未来）
```

## 0.4 当前 Git store 的 Collection 映射

| Collection | 物理实现 | 语义 |
|---|---|---|
| Snapshot | 真实文件 + git commit/ref/update-ref CAS | 不可变版本状态 |
| Append | `streams/<ref>.jsonl`（gitignored side file） | append-only、event-id 幂等 |
| Projection | SQLite FTS5 | 可重建、非权威、记录 basis/lag |

---

# Phase A　用户从零接入知识并构建 Agent 访问

场景：Alice 有一个本地目录 `~/knowledge`，里面是值班手册、支付故障说明和个人笔记。她希望先只发布自己的知识，随后让一个支付值班 Agent 在可重放的知识版本上回答问题。

## A.1 启动 Catalog，创建并注册第一个 Repository

产品管理面的理想动作是：

```text
BOOTSTRAP_CATALOG(namespace="acme")
CREATE_REPOSITORY(
  repositoryId="kr://acme/personals/alice",
  storeProfile="file-git",
  location="~/alice-kc"
)
REGISTER_REPOSITORY(catalog="acme", repository=personal)
```

当前参考代码对应：

```ts
const store = new Store();
const personal = new FileGitRepository(
  "/Users/alice/alice-kc",
  "kr://acme/personals/alice",
);
store.addRepository(personal);

const catalog = new Catalog(store);
const ingress = new Ingress(store);
const access = new Access(store);
```

`FileGitRepository` 会初始化真实 Git repository、`main` 和 root commit。这里创建的是一个独立权威边界；不是“每个文件一个 Repo”，也不是“每个 Agent 一个 Catalog”。

- `[代码]` `FileGitRepository`、`Store.addRepository`、`Catalog/Ingress/Access` 构造器 ✅
- `[缺口]` Catalog/Repository registry 当前只在内存中，进程重启后的恢复与管理 API 尚未实现。

## A.2 扫描知识源，先预览再提交

源目录：

```text
~/knowledge/
├── runbooks/payment-oncall.md
├── incidents/refund-timeout.md
└── notes/freeze-window.txt
```

导入不是直接把目录变成线上知识。`ingest()` 先生成一个 `ChangeSet` 预览，用户或接入程序检查对象身份、目标 Repo、来源和变更范围，再通过 Ingress 提交：

```text
base = personal.head("refs/heads/main")
preview = ingest("/Users/alice/knowledge", personal.repositoryId, base)

→ files:
  runbooks/payment-oncall.md   → object_id=runbooks/payment-oncall
  incidents/refund-timeout.md → object_id=incidents/refund-timeout
  notes/freeze-window.txt      → object_id=notes/freeze-window
→ provenance.originKind=SOURCE
→ ChangeSet.targetRepository=personal

用户确认 preview
→ Ingress.commit(commandId="import-alice-001", preview.changeSet)
→ CommitReceipt{newCommit:U1, disposition:APPLIED}
```

当前参考实现采用“一文件一 object”，首次导入时用相对路径生成 `object_id`。一旦进入 Repository，`object_id` 会写入文件 frontmatter，之后文件移动不改变身份。生产接入器还应保存 source key → object_id 映射，否则源端重命名可能被误判为删除加新增。

- `[K-21]` INGEST 是 COMMIT 上的薄编排，必须经过 Ingress。
- `[代码]` `ingest()` + `Ingress.commit()` ✅
- `[边界]` PDF 分段、网页抓取、OCR、去重和 source key 映射属于 Collector/Connector，不由当前 `ingest()` 自动完成。

## A.3 验收 Repository 中的 Canonical 知识

提交后先做机械验收，再发布给 Agent：

```text
Access.list(personal, U1)
→ 3 个 KnowledgeValue，每个都带 repository + commit + object_id

Access.resolve(
  {repository:personal, object:"runbooks/payment-oncall"}, U1)
→ {status:RESOLVED, pathHint:"runbooks/payment-oncall.md", digest:...}

Access.origin(同一 KnowledgeRef, U1)
→ ProvenanceTrace{commit:U1, chain:[{originKind:SOURCE, sourceRefs:["/Users/alice/knowledge"]}]}
```

验收失败时修正 ChangeSet 并产生新 commit，不直接修改已发布 Generation。路径只是当前物理位置；Agent 应保存 KnowledgeRef 或 PinnedKnowledgeRef，不能把路径当长期身份。

## A.4 定义消费视图，冻结并发布第一个 Generation

Repository 有内容后，才定义 Agent 应看到什么：

```text
def = catalog.defineView("payments-agent", revision=1, [
  personal@refs/heads/main
])

G1 = catalog.resolveView(def)
→ ViewGeneration{
    generationId:"vg-...",
    repositories:{personal:U1}
  }

catalog.promote(channel="stable", expected=undefined,
                newGenerationId=G1.generationId)
→ stable → G1.generationId
```

`resolveView` 只在此刻解析一次 `main`。发布后即使 `main` 前移，`G1` 仍固定指向 `U1`；Agent 通过 `stable` 得到的是可重放 Generation，不是持续漂移的 branch head。

- `[代码]` `Catalog.defineView/resolveView/promote/channel/generation` ✅
- `[缺口]` ViewDefinition、Generation registry 和 Channel 当前均为进程内状态；生产服务必须持久化，并为 definition revision 与 promotion 提供审计记录。

## A.5 为固定版本构建检索投影

如果 Agent 只按已知 object_id 读取，不需要索引；规模化文本候选发现需要为 Generation 中每个 Repository 的精确 commit 构建 Projection：

```text
for (repository, commit) in G1.repositories:
  SqliteProjection.build(repository, commit)
  → basis={repository, commit}

describeIndex(repository)
→ basisCommit=U1, lagBehindHead=false
```

Projection 只负责找候选 object_id。最终值必须回读 `personal@U1` 的 Git Canonical，不能直接把索引副本当事实。

- `[K-19]` Projection 可丢失、可重建、必须声明 basis/lag。
- `[代码]` `SqliteProjection` 支持单 Repository 固定 commit 的构建与查询 ✅
- `[缺口]` 当前没有 Catalog 级索引调度器，需要服务层按 Generation 成员构建、选择和回收投影。

## A.6 给 Agent 暴露最小只读工具

Agent 不应获得 Git 路径或任意 Repository 写权限。Catalog Service 在 HTTP、SDK 或 MCP 传输层上包装以下只读工具即可：

| Agent 工具 | 服务端协议调用 | 返回关键字段 |
|---|---|---|
| `kc_search(query, channel)` | Channel → Generation；逐成员在固定 commit 上 SEARCH；可选 Refine | generation_id、候选 KnowledgeRef、repo、commit、摘要 |
| `kc_read(knowledge_ref, generation_id)` | 从 Generation 取该 Repo 的 commit，再 READ | KnowledgeValue + PinnedKnowledgeRef |
| `kc_origin(pinned_ref)` | ORIGIN | 来源、actor、活动、固定输入链 |

一次 Agent 请求必须先固定读取切面，后续所有工具调用复用同一个 `generation_id`：

```text
Agent question
  → channel("stable") = G1_ID              # 每个请求只解析一次
  → generation(G1_ID) = {personal:U1}
  → Access.search(query, personal, U1)      # 找候选
  → SemanticRefine.run(fixedCandidates)     # 可选，不能扩展候选集
  → Access.read(selectedRef, U1)            # 回读 Canonical
  → Access.origin(selectedRef, U1)          # 取来源
  → groundingCitation(KnowledgeValue)
  → answer + pinned citation
```

推荐的 Agent 配置只保存 `catalog_namespace=acme`、`channel=stable` 和服务凭证。它不保存 `main`，也不自行选择“最新 commit”。服务端应校验：请求的 Repository 必须属于该 Generation，调用者必须有对应读取权限。

- `[代码]` Channel/Generation、`Access.search/read/origin`、Semantic Refine、`groundingCitation` 分别已实现 ✅
- `[缺口]` 当前仓库没有 HTTP/MCP gateway、跨 Repo `kc_search` 编排、身份认证、ACL 和会话级 Generation pinning；因此这里是可直接实现的接入契约，不是已可启动的 Agent 服务。

## A.7 Agent 回答及引用长什么样

```text
问题：切换支付流量前要检查什么？

回答：先确认当前不在冻结窗口，再按 payment-oncall runbook 执行切流。

引用：
kc://acme/personals/alice@U1/notes/freeze-window
kc://acme/personals/alice@U1/runbooks/payment-oncall
generation: G1
```

回答中的 Repository 事实必须有 pinned citation；模型综合、比较或推断的部分要显式标为推断。这样可以在 `main` 已更新后仍重放当时的依据。

## A.8 更新知识而不让 Agent 读到半成品

```text
源目录发生变化
→ reconcile(externalSnapshot, currentDigests, personal, U1)
→ preview {added:1, updated:1, removed:0}
→ 用户确认
→ Ingress.commit(...) → U2

此时：main=U2，但 stable=G1={personal:U1}

完成索引与验证
→ resolveView(revision=2) → G2={personal:U2}
→ catalog.promote("stable", expected=G1.generationId,
                  newGenerationId=G2.generationId)
→ 新请求读取 G2；已固定 G1 的进行中请求继续读 U1
```

内容提交与服务发布是两个动作。Commit 失败不产生部分状态；Promotion CAS 失败继续服务旧 Generation。

## A.9 从个人知识扩展到团队知识

以后接入 public 和 payments group 时，不创建第二套 Catalog，也不把三个 Repo merge 成一个：

```text
CREATE + REGISTER publicRepo
CREATE + REGISTER groupRepo

payments-agent revision 3 sources = [
  public@main,
  group@main,
  personal@main
]
→ G3={public:P3, group:G2, personal:U2}
→ validate/index
→ stable: G2 --CAS--> G3
```

同一 object_id 在多个 Repo 出现时，Catalog 返回多个带来源的值，不按 public/group/personal 覆盖。选择、汇总和冲突呈现属于 Application/Agent policy。

## A.10 用户完成接入的判据

从零接入不是“文件已经上传”，而是以下链路均成立：

```text
Catalog 已 bootstrap
→ Repository 已创建并注册
→ ingest/reconcile 预览已确认
→ COMMIT 返回不可变 commit
→ View 已解析为不可变 Generation
→ Generation 所需的 Projection/验证已就绪
→ Channel 已 CAS 发布
→ Agent 每次请求固定 Generation
→ 最终回答携带 PinnedKnowledgeRef 与来源
```

---

# Phase B　单 source ViewGeneration 的自然退化

这不是“单人语义”，而是同一 Catalog 协议在 ViewDefinition 只有一个 source 时的退化形态。

## B.1 创建 Repository + COMMIT

```text
FileGitRepository(rootDir="~/alice-kc", repositoryId="kr://acme/personals/alice")
→ git init（main）+ root commit

Ingress.commit(PUT note/payment-oncall,
               value={text:"切换流量前先检查冻结窗口"},
               pathHint="notes/oncall.json")
→ 写 notes/oncall.json（frontmatter 内嵌 object_id）
→ git commit → U1
```

- `[K-04]` object_id 不在路径。
- `[K-05]` commit 不可变。
- `[K-06]` ref 更新由 git CAS 语义约束。
- `[代码]` `FileGitRepository.applyCommit` ✅

## B.2 ViewDefinition → ViewGeneration（单 source）

```text
catalog.defineView("alice", 1, [personal@main])
catalog.resolveView(def)
→ ViewGeneration{ repositories:{personal:U1}, generationId:H(...) }
```

单 source 时 ViewGeneration 仍是 `repo→commit` Map，只是成员数量为一；没有切换另一套模式。

- `[K-10]` 同 Repo 只能出现一次。
- `[代码]` `Catalog.resolveView` ✅

## B.3 RESOLVE + READ

```text
Access.resolve({repository:personal, object:"note/payment-oncall"}, U1)
→ 扫描 frontmatter 找 object_id
→ {status:RESOLVED, pathHint, digest}

Access.read(同 ref, U1)
→ {repository, commit, address, value, provenance}
```

- address-map 是可重建 Projection，不是 Canonical。
- `[代码]` `FileGitRepository.resolve/read` ✅

## B.4 移动文件（身份不变）

```text
Ingress.commit(PUT note/payment-oncall, 同 value,
               pathHint="notes/oncall-v2.yaml")
→ 删除旧路径，写新路径，git commit → U2

RESOLVE(note/payment-oncall, U2)
→ 仍 RESOLVED，object_id 不变，pathHint 更新
```

- `[K-04]` 路径移动不破坏 KnowledgeRef。
- `[T1]` `.yaml` 移动场景在真实 git 上通过。✅

## B.5 ORIGIN

```text
Access.origin(note/payment-oncall, U2)
→ frontmatter provenance + git commit
→ ProvenanceTrace{value, repository, commit, chain}
```

- DERIVATION 必须记录 input version + algorithm。
- `[代码]` `FileGitRepository.origin` ✅

## B.6 APPEND

```text
Ingress.append(stream="evidence",
               entries=[{eventId:"evt-1", payload:{outcome:"PASSED"}}])
→ streams/evidence.jsonl 追加 record
→ cursor 前移

同 eventId + 同 payload → 返回原 recordId
同 eventId + 异 payload → EVENT_ID_CONFLICT
```

- `[K-17]` Entry 不原地修订。
- Append 不进入 git tree，保留非 Git 演化语义。
- `[代码]` `FileGitRepository.append/streamCursor` ✅

## B.7 INGEST / RECONCILE

```text
ingest(dir, repositoryId, baseCommit)
→ 一个文件 = 一个 object，provenance originKind=SOURCE
→ ChangeSet 预览 → Ingress.commit

reconcile(snapshot, currentDigests, repositoryId, baseCommit)
→ added/updated/removed → ChangeSet 预览
```

- `[K-21]` 是 COMMIT 之上的薄编排，不绕过 Ingress。
- `[代码]` `api/ingestion.ts` ✅

## B.8 GroundingCitation

```text
groundingCitation(KnowledgeValue)
→ {knowledgeRef, pinnedRef, provenanceSummary}
```

AI 事实断言必须有 PinnedKnowledgeRef 或显式标“推断”。

**Phase B 结论**：同一协议在单 source ViewGeneration 下完成被覆盖的核心读写闭环，Repository store 是真实 Git。

---

# Phase C　同一语义在 public/group/personal 三 Repo 上展开

## C.1 三 Repo 独立

```text
public:   policy/P-103 = {statement:"production requires owned runbook"}
group:    policy/P-103 = {statement:"applies within production"}
          assertion/A-27 = {about:"policy/P-103"}
personal: note/payment-oncall = {...}
```

三个都是 `FileGitRepository`，共同实现 `Repository` 接口。

- `[K-02]` 独立 commit 图/ref/生命周期。
- `[K-03]` Scope 不是目录优先级。
- `[代码]` `Store.addRepository(Repository)` ✅

## C.2 RESOLVE_VIEW（多 source）

```text
catalog.defineView("alice-default", 1,
  [public@main, group@main, personal@main])

catalog.resolveView(def)
→ ViewGeneration{
    generationId = H(1 ‖ sorted({public:P3, group:G2, personal:U1})),
    repositories = {public:P3, group:G2, personal:U1}
  }
```

- `[K-10]` 重复 Repo → `VIEW_GENERATION_INVALID`。
- generation_id 同输入同 id。
- `[代码]` `Catalog.resolveView`（store-agnostic）✅

## C.3 联邦读（不覆盖）

```text
catalog.readObject(gen, "policy/P-103")
→ [
    {repository:public, commit:P3, value:{statement:"production requires owned runbook"}},
    {repository:group,  commit:G2, value:{statement:"applies within production"}}
  ]
```

- `[K-12]` 保留 source repo/commit。
- `[K-13]` 多来源并存，不按 Scope 覆盖。
- `[代码]` `Catalog.readObject` ✅

## C.4 PROPOSAL（真实 git candidate branch）

```text
controlPlane.propose(
  proposalId="PR-42",
  repositoryId=group,
  targetRef="refs/heads/main",
  candidateRef="refs/heads/candidates/PR-42",
  baseCommit=G2,
  operations=[PUT procedure/refund-timeout])

→ git update-ref 创建 candidate branch
→ checkout candidate → git commit C1 → checkout main
→ main 仍 G2
```

- `[K-07]` Proposal 不改 main。
- `[代码]` `FileGitRepository.createRef/applyCommit` + `ControlPlane.propose` ✅

## C.5 PREVIEW + VALIDATE

```text
controlPlane.createPreview(baseGeneration=VG1, proposal)
→ Catalog CREATE_PREVIEW：以 group:C1 替换 VG1 中的 group:G2
→ PreviewGeneration{public:P3, group:C1, personal:U1}

controlPlane.validate(preview, "S7", "PASSED")
→ ValidationReport 绑定完整 preview_generation_id
```

- `[K-09]` Validation 绑定完整 PreviewGeneration。
- `[代码]` `ControlPlane.createPreview/validate` ✅

## C.6 candidate 前移使旧 Validation 失效

```text
同 candidateRef 再提交 C2
→ merge(旧 proposal C1, 旧 preview generation, 旧 validation)
→ CANDIDATE_MOVED
```

- `[ADR-013]` candidate 前移使旧 validation 失效。
- `[代码]` `ControlPlane.merge` 检查 candidate ref head ✅

## C.7 MERGE + PROMOTE

```text
controlPlane.merge(新 proposal C2, 完整 preview generation, 新 validation)
→ git update-ref refs/heads/main C2 G2（CAS）
→ 若 main 当前 checkout，git reset --hard 同步工作树

catalog.promote("stable", expectedGeneration, newGeneration)
→ 只移动 channel 指针，不改 Repo
```

- `[K-06]` Merge 是 CAS。
- `[K-22]` Promote 不改 Repo。
- `[代码]` `FileGitRepository.merge` + `Catalog.promote` ✅

## C.8 上游升级

```text
public main P3 → P4
catalog.resolveView(def)
→ 新 generation {public:P4, group:C2, personal:U2}
```

- `[K-14]` 普通引用升级不修改 group/personal Repo，不做跨 Repo merge。
- `[代码]` `Catalog.resolveView` ✅

## C.9 Embedded Access

```text
SqliteProjection.build(groupRepo, C2)
→ FTS5 只索引 object_id + value_text
→ SEARCH 命中后 read() 回 Git Canonical
→ describeIndex() 返回 basis/lag
```

- `[K-19]` Projection 非权威、可重建。
- `[代码]` `SqliteProjection` ✅

## C.10 Semantic Refinement

```text
SemanticRefine.run(spec, fixedCandidates, judge/scorer)
→ SEM_FILTER：MATCH/NO_MATCH/UNKNOWN/UNJUDGED
→ SEM_RERANK：RankGroup + unjudged
```

- 输出 Ref ⊆ 输入 Ref；不 SEARCH、不调工具、无副作用。
- `[代码]` `SemanticRefine` + `SemanticOperatorSpec` ✅

## C.11 回滚分层

```text
Projection 错 → rebuild
Serving 组合错 → Catalog.rollback（不动 Repo）
权威内容错 → Repository REVERT（保留历史）
```

三层不可混用。

**Phase C 结论**：同一 Catalog 协议在三 Repo 联邦上完整展开，并在同一个 Git store adapter 上通过。

---

# Phase D　推演总评

## D.1 参考实现语义闭环

- Catalog 语义只有一套；单 source 只是 ViewGeneration 的自然退化。
- 协议层只依赖 `Repository` 接口。
- FileGit 是当前唯一 store 实现；Snapshot=真实 git，Append=JSONL side stream。
- Catalog 注册不可变 Generation；Preview 保留完整成员；Promote 只能指向已注册 Generation。
- 联邦读只忽略明确的对象缺失，版本、完整性和 Backend 故障继续向调用方报告。

## D.2 推演曾暴露、现已补齐的裂缝

| 原裂缝 | 修复 |
|---|---|
| FileGit 无 candidate branch | 实现 `getRef/createRef/merge`，使用真实 git `update-ref` CAS |
| ControlPlane/Catalog 绑定 MemoryStore | 抽统一 `Repository` 接口；Store 变为 `Map<id, Repository>` |
| FileGit 无 APPEND 顺序约束 | JSONL Append 实现 Event ID 幂等与 expected cursor CAS |
| Preview 只含 Candidate Repo | 从已注册 Base Generation 替换一个成员，其余成员保持不变 |
| Channel 可指向未知 Generation | 增加不可变 Generation Registry，Promote/Rollback 校验目标存在 |
| 联邦读吞掉所有异常 | 只跳过 KNOWLEDGE_REF_UNRESOLVED，其余错误传播 |
| Pinned HEAD 误读工作区 | 所有 pinned read 固定读取 Git tree |
| Adapter 测试绑定 FileGit | 增加由 Repository Factory 驱动的共享 T12 Contract Test Kit |

## D.3 用户主线暴露、尚未补齐的产品能力

| 缺口 | 为什么是从零接入所必需 | 建议归属 |
|---|---|---|
| Catalog 管理面与持久化 | 必须创建/恢复 namespace、Repository registry、View revision、Generation 和 Channel | Catalog Service / Metadata Store |
| 稳定的 source key → object_id 映射 | Connector 必须正确识别源端重命名，而不是制造新对象 | Collector / Connector State |
| Generation 级索引编排 | 发布前要为每个 repo@commit 建好投影，并按 basis/lag 选择可用索引 | Access / Projection Controller |
| Agent serving gateway | 需要把 channel pinning、跨 Repo SEARCH、READ、ORIGIN 和 Citation 包成 HTTP/MCP/SDK 工具 | Application / Access Gateway |
| 身份认证与 Repository ACL | Agent 只能读取当前主体在该 Generation 中获准访问的成员 | AuthN/AuthZ / Policy |

这些不是新增一套 Catalog 语义，而是把现有协议变成可部署产品所需的管理、编排和安全层。实现顺序应优先覆盖一条最小竖切：单 namespace 持久化 → 单 Repo 导入 → `stable` 发布 → 只读 Agent gateway → pinned citation；再扩展多租户和多 Repo 检索。

## D.4 最终判断

> **被 Conformance 覆盖的参考实现语义已经闭环，Store 依赖倒置成立；从知识导入到 Agent 引用的产品契约已经明确，但 Agent 服务本身尚未端到端实现。**

Conformance T1–T12 共 40 个 case 通过；依赖 Repository 的用例运行在真实 FileGit 上，T10 是无 Store 的纯语义测试。Catalog 持久化、Agent gateway、索引编排、身份认证、进程间并发、性能和灾难恢复仍需按部署边界补足。
