# Knowledge Catalog v5.1 全流程推演

**用同一套 Catalog 协议 + Git store adapter，重新推演单 source 退化形态与 public/group/personal 多 Repo 联邦形态。**

每步标注：`[K-xx]` = 不变量；`[代码]` = 当前实现对应。

目标：检验**协议是否只有一套、store 是否可替换、端到端是否闭环**。

---

# 0. 推演前提

## 0.1 一套语义，多个 store 实现

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

## 0.2 当前 Git store 的 Collection 映射

| Collection | 物理实现 | 语义 |
|---|---|---|
| Snapshot | 真实文件 + git commit/ref/update-ref CAS | 不可变版本状态 |
| Append | `streams/<ref>.jsonl`（gitignored side file） | append-only、event-id 幂等 |
| Projection | SQLite FTS5 | 可重建、非权威、记录 basis/lag |

---

# Phase A　单 source ViewGeneration 的自然退化

这不是“单人语义”，而是同一 Catalog 协议在 ViewDefinition 只有一个 source 时的退化形态。

## A.1 创建 Repository + COMMIT

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

## A.2 ViewDefinition → ViewGeneration（单 source）

```text
catalog.defineView("alice", 1, [personal@main])
catalog.resolveView(def)
→ ViewGeneration{ repositories:{personal:U1}, generationId:H(...) }
```

单 source 时 ViewGeneration 仍是 `repo→commit` Map，只是成员数量为一；没有切换另一套模式。

- `[K-10]` 同 Repo 只能出现一次。
- `[代码]` `Catalog.resolveView` ✅

## A.3 RESOLVE + READ

```text
Access.resolve({repository:personal, object:"note/payment-oncall"}, U1)
→ 扫描 frontmatter 找 object_id
→ {status:RESOLVED, pathHint, digest}

Access.read(同 ref, U1)
→ {repository, commit, address, value, provenance}
```

- address-map 是可重建 Projection，不是 Canonical。
- `[代码]` `FileGitRepository.resolve/read` ✅

## A.4 移动文件（身份不变）

```text
Ingress.commit(PUT note/payment-oncall, 同 value,
               pathHint="notes/oncall-v2.yaml")
→ 删除旧路径，写新路径，git commit → U2

RESOLVE(note/payment-oncall, U2)
→ 仍 RESOLVED，object_id 不变，pathHint 更新
```

- `[K-04]` 路径移动不破坏 KnowledgeRef。
- `[T1]` `.yaml` 移动场景在真实 git 上通过。✅

## A.5 ORIGIN

```text
Access.origin(note/payment-oncall, U2)
→ frontmatter provenance + git commit
→ ProvenanceTrace{value, repository, commit, chain}
```

- DERIVATION 必须记录 input version + algorithm。
- `[代码]` `FileGitRepository.origin` ✅

## A.6 APPEND

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

## A.7 INGEST / RECONCILE

```text
ingest(dir, repositoryId, baseCommit)
→ 一个文件 = 一个 object，provenance originKind=SOURCE
→ ChangeSet 预览 → Ingress.commit

reconcile(snapshot, currentDigests, repositoryId, baseCommit)
→ added/updated/removed → ChangeSet 预览
```

- `[K-21]` 是 COMMIT 之上的薄编排，不绕过 Ingress。
- `[代码]` `api/ingestion.ts` ✅

## A.8 GroundingCitation

```text
groundingCitation(KnowledgeValue)
→ {knowledgeRef, pinnedRef, provenanceSummary}
```

AI 事实断言必须有 PinnedKnowledgeRef 或显式标“推断”。

**Phase A 结论**：同一协议在单 source ViewGeneration 下完整闭环，store 是真实 git。

---

# Phase B　同一语义在 public/group/personal 三 Repo 上展开

## B.1 三 Repo 独立

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

## B.2 RESOLVE_VIEW（多 source）

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

## B.3 联邦读（不覆盖）

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

## B.4 PROPOSAL（真实 git candidate branch）

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

## B.5 PREVIEW + VALIDATE

```text
controlPlane.createPreview(proposal)
→ PreviewGeneration{previewId:"preview-C1", repositoryCommits:{group:C1}}

controlPlane.validate(preview, "S7", "PASSED")
→ ValidationReport 绑定 preview-C1
```

- `[K-09]` Validation 绑定完整 PreviewGeneration。
- `[代码]` `ControlPlane.createPreview/validate` ✅

## B.6 candidate 前移使旧 Validation 失效

```text
同 candidateRef 再提交 C2
→ merge(旧 proposal C1, 旧 validation)
→ CANDIDATE_MOVED
```

- `[ADR-013]` candidate 前移使旧 validation 失效。
- `[代码]` `ControlPlane.merge` 检查 candidate ref head ✅

## B.7 MERGE + PROMOTE

```text
controlPlane.merge(新 proposal C2, 新 validation)
→ git update-ref refs/heads/main C2 G2（CAS）
→ 若 main 当前 checkout，git reset --hard 同步工作树

catalog.promote("stable", expectedGeneration, newGeneration)
→ 只移动 channel 指针，不改 Repo
```

- `[K-06]` Merge 是 CAS。
- `[K-22]` Promote 不改 Repo。
- `[代码]` `FileGitRepository.merge` + `Catalog.promote` ✅

## B.8 上游升级

```text
public main P3 → P4
catalog.resolveView(def)
→ 新 generation {public:P4, group:C2, personal:U2}
```

- `[K-14]` 普通引用升级不修改 group/personal Repo，不做跨 Repo merge。
- `[代码]` `Catalog.resolveView` ✅

## B.9 Embedded Access

```text
SqliteProjection.build(groupRepo, C2)
→ FTS5 只索引 object_id + value_text
→ SEARCH 命中后 read() 回 Git Canonical
→ describeIndex() 返回 basis/lag
```

- `[K-19]` Projection 非权威、可重建。
- `[代码]` `SqliteProjection` ✅

## B.10 Semantic Refinement

```text
SemanticRefine.run(spec, fixedCandidates, judge/scorer)
→ SEM_FILTER：MATCH/NO_MATCH/UNKNOWN/UNJUDGED
→ SEM_RERANK：RankGroup + unjudged
```

- 输出 Ref ⊆ 输入 Ref；不 SEARCH、不调工具、无副作用。
- `[代码]` `SemanticRefine` + `SemanticOperatorSpec` ✅

## B.11 回滚分层

```text
Projection 错 → rebuild
Serving 组合错 → Catalog.rollback（不动 Repo）
权威内容错 → Repository REVERT（保留历史）
```

三层不可混用。

**Phase B 结论**：同一 Catalog 协议在三 Repo 联邦上完整展开，并在同一个 Git store adapter 上通过。

---

# Phase C　推演总评

## C.1 协议闭环

- Catalog 语义只有一套；单 source 只是 ViewGeneration 的自然退化。
- 协议层只依赖 `Repository` 接口。
- FileGit 是当前唯一 store 实现；Snapshot=真实 git，Append=JSONL side stream。
- ControlPlane 与 Catalog 不依赖具体 store。
- 未来新增 Dolt/PostgreSQL adapter 时，协议对象与不变量不变。

## C.2 推演曾暴露、现已补齐的裂缝

| 原裂缝 | 修复 |
|---|---|
| FileGit 无 candidate branch | 实现 `getRef/createRef/merge`，使用真实 git `update-ref` CAS |
| ControlPlane/Catalog 绑定 MemoryStore | 抽统一 `Repository` 接口；Store 变为 `Map<id, Repository>` |
| FileGit 无 APPEND | 实现 gitignored JSONL stream + event-id 幂等 |
| Memory 模拟重复实现 git | 删除 Memory，实现与测试全部迁移到真实 git |

## C.3 最终判断

> **v5.1 协议闭环，store 解耦成立。当前 Git adapter 从 COMMIT/APPEND/RESOLVE/ORIGIN 一直打通到 PROPOSAL/PREVIEW/MERGE/PROMOTE/Catalog 联邦；不再存在“Memory 一条线、FileGit 一条线”。**

Conformance T1–T11（32 个 case）全部运行在真实 git 上。Dolt 等后续实现只需实现 `Repository` 接口，并跑同一套测试。
