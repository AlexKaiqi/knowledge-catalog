# Knowledge Catalog v5.1 全流程推演

**用 v5.1 协议 + 三 Repo 案例（public/core、group/payments、personal/alice）重新推演端到端流程。**

每步标注：`[K-xx]` = 不变量；`[代码]` = 代码骨架对应；`⚠️ 缺口` = 推演暴露的、代码未打通之处。

目标不是复述设计，而是**检验协议是否闭环、代码是否撑得住**。

---

# Phase A　单人最小语义层（repo-native，FileGit Profile）

Alice（个人空间）用真实文件 + git 完成"写入 → 定位 → 读 → 移动 → 溯源 → 检索 → 采集"。

## A.1 创建 Repository + COMMIT 写入

```text
FileGitRepository(rootDir="~/alice-kc", repositoryId="kr://acme/personals/alice")
→ git init + 空 root commit
→ Ingress.commit(PUT note/payment-oncall, value={text:"切换流量前先检查冻结窗口"},
                 pathHint="notes/oncall.json")
→ 写文件 notes/oncall.json（frontmatter 内嵌 object_id: note/payment-oncall）
→ git commit → 返回 commit U1
```

- `[K-04]` object_id 内嵌文件内容，不在路径。
- `[K-05]` commit 不可变。
- `[代码]` `FileGitRepository.applyCommit` ✅

## A.2 RESOLVE + READ

```text
Access.resolve({repository:"kr://acme/personals/alice", object:"note/payment-oncall"}, U1)
→ scan 文件 frontmatter 找 object_id → 返回 pathHint + digest + status=RESOLVED

Access.read(同 ref, U1) → 返回 { repository, commit, address, value, provenance }
```

- `[代码]` `FileGitRepository.resolve / read` ✅
- address-map 是可重建 projection（扫描文件），无需独立映射。✅

## A.3 移动文件（身份不变）

```text
Ingress.commit(PUT note/payment-oncall, 同 value, pathHint="notes/oncall-v2.json")
→ 删除旧文件，写新文件，git commit → U2

Access.resolve(note/payment-oncall, U2) → 仍 RESOLVED，pathHint 变为新路径
```

- `[K-04]` 移动不改 object_id；`[T1]` 已验证。
- `[代码]` `FileGitRepository.applyCommit` 处理 move（delete old + write new）✅

## A.4 ORIGIN（溯源）

```text
Access.origin(note/payment-oncall, U2)
→ 读 frontmatter provenance + git log → ProvenanceTrace{ value, repo, commit, chain[] }
```

- `[代码]` `FileGitRepository.origin` ✅
- DERIVATION 类对象必须显式 input/algorithm（否则来源链断）。✅

## A.5 SEARCH

```text
Access.search("冻结窗口", repositoryId, U2) → 命中 note/payment-oncall
```

- `[代码]` `FileGitRepository.search`（JSON 文本匹配；多人 Profile 可用 SqliteProjection FTS5）✅

## A.6 APPEND（记录观察）

```text
Ingress.append(stream="evidence", entries=[{eventId:"evt-1", payload:{outcome:"PASSED"}}])
→ append-only 记录 + cursor 前移
```

- `[K-17]` 不原地修订；同 event id 同内容幂等，异内容 `EVENT_ID_CONFLICT`。
- `[代码]` `MemoryAppendStream` ✅
- **⚠️ 缺口 C**：`MemoryAppendStream` 是 Memory 专属；`FileGitRepository` 没有对应 append（JSONL 文件）实现。repo-native 单人要记观察，目前只能退回 Memory。

## A.7 INGEST（低摩擦采集）

```text
ingest(dir="~/docs", repositoryId, baseCommit=U2)
→ 扫描目录，一个文件 = 一个 object，provenance originKind=SOURCE
→ 产出 ChangeSet 预览，确认后 Ingress.commit
```

- `[K-21]` INGEST 是 COMMIT 之上的薄编排，不绕过 Ingress。
- `[代码]` `api/ingestion.ts` `ingest()` ✅（产出 CommitChangeSet，可喂给 FileGit 或 Memory）

## A.8 GroundingCitation

```text
groundingCitation(KnowledgeValue) → { knowledgeRef, pinnedRef, provenanceSummary }
```

- 硬不变量：AI 事实断言必须有 PinnedKnowledgeRef 或显式标"推断"。
- `[代码]` `api/ingestion.ts` `groundingCitation()` ✅

**Phase A 结论**：最小语义层（COMMIT/RESOLVE/READ/SEARCH/ORIGIN/INGEST/grounding）在 FileGit 上闭环，**唯一缺口是 APPEND 未落到 FileGit**。

---

# Phase B　多人多 Repo（Catalog + 维护闭环）

Core Council（public）、Payments（group）、Alice（personal）三个独立 Repo 联合。

## B.1 三 Repo 独立

```text
public:   policy/P-103 = {statement:"production requires owned runbook"}
group:    policy/P-103 = {statement:"applies within production"}   # 本地限定，同名不同值
          assertion/A-27 = {about:"policy/P-103"}
personal: note/payment-oncall = {...}
```

- `[K-02]` 三个独立 ACL/Commit 图/Ref；`[K-03]` 是治理 Scope，非目录优先级。
- `[代码]` `MemoryStore.addRepository` ✅

## B.2 DEFINE_VIEW + RESOLVE_VIEW

```text
catalog.defineView("alice-default", 1, sources=[public@main, group@main, personal@main])
catalog.resolveView(def) → ViewGeneration{
  generationId = H(1 ‖ sorted({public:P3, group:G2, personal:U1})),
  repositories = {public:P3, group:G2, personal:U1}
}
```

- `[K-10]` 每 Repo 只出现一次；重复 → `VIEW_GENERATION_INVALID`。
- `[G2]` generation_id 确定函数，同输入同 id（幂等）。
- `[代码]` `catalog/catalog.ts` `resolveView` ✅

## B.3 联合读（不覆盖）

```text
catalog.readObject(gen, "policy/P-103")
→ [ {repository:public, commit:P3, value:{statement:"production requires owned runbook"}},
    {repository:group,  commit:G2, value:{statement:"applies within production"}} ]
```

- `[K-12]` 保留 source repo/commit；`[K-13]` 两个来源并存，不覆盖。
- `[代码]` `catalog/catalog.ts` `readObject` ✅

## B.4 本地分歧（Assertion）

group 不修改 public，写本地 `assertion/A-27`（`about: policy/P-103`）。

- `[K-13]` 多来源 Assertion 并存；不静默覆盖。✅

## B.5 PROPOSAL → PREVIEW → VALIDATE

```text
controlPlane.propose(proposalId="PR-42", repositoryId=group, targetRef=main,
                     candidateRef="candidates/PR-42", baseCommit=G2,
                     operations=[PUT procedure/refund-timeout])
→ 创建 candidate branch（createRef）→ commit C1（不动 main）

controlPlane.createPreview(proposal) → PreviewGeneration{ previewId:"preview-C1",
  repositoryCommits:{group:C1}, candidate:{group, C1} }

controlPlane.validate(preview, "S7", "PASSED") → ValidationReport 绑定 preview-C1
```

- `[K-07]` Proposal 不改 main；`[K-09]` Validation 绑完整 PreviewGeneration。
- `[代码]` `control-plane/maintenance.ts` `propose/createPreview/validate` ✅（**基于 MemoryRepository**）

## B.6 candidate 前移 → 旧 validation 失效

```text
再次 propose 到同一 candidateRef（base=G2，candidate 已是 C1 → 新 commit C2）
→ merge(旧proposal C1, 旧val) → 抛 CANDIDATE_MOVED
```

- `[ADR-013]` candidate 前移使旧 validation 失效。
- `[代码]` `ControlPlane.merge` 检查 `getRef(candidateRef) !== candidateCommit` → `CANDIDATE_MOVED` ✅

## B.7 MERGE + PROMOTE

```text
controlPlane.merge(新proposal C2, 新val) → CAS fast-forward main G2→C2
controlPlane.promote("stable", expected, newCommit) → CAS channel（只移指针，不改 repo）
```

- `[K-06]` merge 是 CAS；`[K-22]` promote 不改 repo。
- `[代码]` `MemoryRepository.merge` + `ControlPlane.promote` ✅

## B.8 上游升级（不做跨 Repo merge）

```text
public 前移 P3 → P4 → 新 generation {public:P4, group:C2, personal:U1}
→ group/personal 的 Repo 未被修改（无跨 Repo merge）
```

- `[K-14]` 普通引用升级不改引用方 Repo。
- `[代码]` `catalog.resolveView` 重新解析即可 ✅

## B.9 回滚（分层）

```text
catalog.rollback("stable", expected, prior) → CAS 指回旧 generation（不动 repo）
repo REVERT（若权威内容本身错）→ 保留历史
```

- `[K-22]` + 回滚分层（Projection/Catalog/Repository 不可混用）。
- `[代码]` `catalog.rollback` ✅

**Phase B 结论**：多人多 Repo 的维护闭环 + Catalog 联邦在 **Memory** 上完整闭环。

---

# Phase C　推演结论：闭环的与缺口的

## C.1 协议闭环的部分（✅）

- 身份寻址（object_id 内嵌 frontmatter，移动不改身份）
- 来源链（ORIGIN 回链 provenance）
- 维护闭环（PROPOSAL→PREVIEW→VALIDATE→MERGE→PROMOTE，candidate 前移失效）
- Catalog 联邦（确定性 generation、联合读不覆盖、Promote 独立 CAS）
- 采集/grounding（INGEST 薄编排、GroundingCitation 硬不变量）

这些**纸面协议与代码一一对应，且被 29 个测试锁住**。

## C.2 推演暴露的缺口（⚠️）

| # | 缺口 | 本质 | 影响 |
|---|---|---|---|
| **C1** | `FileGitRepository` 无 `getRef / createRef / merge` | FileGit 无法表达 candidate branch | repo-native Profile 无法跑 PROPOSAL/维护闭环 |
| **C2** | `ControlPlane` / `Catalog` 硬绑 `MemoryStore`（`Map<id, MemoryRepository>`） | 缺统一的 Repository 接口 | 多人多 Repo 的维护闭环 + Catalog 联邦**只能跑 Memory，不能跑 FileGit** |
| **C3** | `FileGitRepository` 无 append stream | APPEND 只在 Memory 实现 | repo-native 单人要记观察/证据，只能退回 Memory |

**一句话**：v5.1 协议是对的，但代码里**"单人最小语义层"和"多人多 Repo"在两个不同的 Profile 上验证了，中间缺一个统一的 Repository 接口把它们接起来**——这正是 `minimal-core-contracts` 里 `ReadableRepository`（embedded/projection.ts 里那个局部接口）该升级成全 Repository 抽象的地方。

## C.3 补缺建议（按优先级）

1. **抽一个 `Repository` 接口**（`head / getRef / createRef / merge / applyCommit / resolve / read / origin / search / list`），让 `MemoryRepository` 和 `FileGitRepository` 都实现它；`MemoryStore` 泛化为 `Map<id, Repository>`。
2. **FileGit 补 `getRef / createRef / merge`**（candidate branch = 真实 git branch）。
3. **FileGit 补 append**（一个 JSONL 文件 + event-id 幂等，语义同 `MemoryAppendStream`）。

做完这三步，"团队/组织共用的 repo-native 知识底座"才是**从头到尾一条线打通的**，而不是 Memory 一条线、FileGit 一条线。

---

## 推演总评

> v5.1 的协议在逻辑上是闭环的，29 个测试证明了不变量。但**推演暴露了实现层的裂缝**：单人（FileGit）和多人（Memory）还没有被同一个 Repository 抽象连接起来。这不是设计错误，是"最小语义层"收敛后、实现按 Phase 落地时留下的最后一处待缝合的缝。
