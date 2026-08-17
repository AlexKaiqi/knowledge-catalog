# Knowledge Catalog 系统设计白皮书

**版本：v5.1 · 面向「团队/组织共用」的 AI 知识底座**

---

# 0. 执行摘要

## 0.1 一句话主张

> **别把 git 已经会的东西重新发明成协议；只发明 git 不会的那三样——身份、来源、写边界——其余全部坍缩成 git 原生 + 一个薄 CLI。**

Knowledge Catalog 不是放大版 Repository，也不是跨 Repo 文件覆盖系统。它是多个独立权威 Knowledge Repository 的**注册、发现与版本化联合视图边界**，服务于一个具体目标：让 AI 能够**稳定引用、可复现版本化、来源保留、写者明确**地使用组织知识。

## 0.2 为什么是"地板"而非"可选项"

对"团队/组织共用"的场景（官方政策 + 组内补充 + 个人覆盖，且 AI 要能说清"这条来自哪、哪个版本"），这套架构**不是可选项，而是地板**。因为真正的约束只有一条：

> **AI 只能引用「身份稳定、版本已知、来源保留、写者明确」的知识。**

这一条性质机械地逼出了整套设计：身份与路径解耦、Commit/Ref/Release、Provenance、public/group/personal 独立权威。普通 RAG/向量库放弃了这些，所以只能"猜一段相似文本"，不能做"知识底座"。

## 0.3 单一语义与最小新增语义（本版的核心收敛）

Catalog 语义只有一套：身份、版本、来源、写边界、ViewGeneration、维护闭环与联邦读取都是同一协议。不同的只是 store adapter；当前实现是 Git，未来可按数据规模替换为 Dolt/PostgreSQL，而协议层不变。

对协议做第一性原理审查后，收敛出一个关键结论：

| 类别 | 含义 | 内容 |
|---|---|---|
| **协议必须定义** | 底层 store 不提供，但可信底座必须有 | 身份寻址（RESOLVE）、来源链（ORIGIN）、写边界（Binding） |
| **Store 原生承载** | 由 adapter 映射到成熟底层 | COMMIT/PROPOSAL、READ、LIST、LOG、DIFF、SEARCH |
| **同一协议的展开/退化** | source 数量或 store 能力改变，语义不变 | 单 source ViewGeneration 自然退化为单 commit；多 source 时完整联邦 |

**真正需要新增定义的只有三样**：身份、来源、写边界。其余由 store adapter 映射到成熟底层；Catalog/维护操作从来不是另一套语义。

---

# 1. 问题定义与第一性原理

## 1.1 真正的问题

通用知识底座面对的不是"知识存哪里"，而是一组相互耦合、不能用单一数据库抽象覆盖的问题：

- 同一主题由不同权威维护，变化频率与冲突粒度不同；
- 原始观察、正式定义、派生摘要、访问索引有不同认识论地位；
- 权威系统变更、用户编辑、维护建议、运行事件不是同一种写入；
- Agent 需要稳定引用、版本、来源，而不是"返回一段相似文本"。

因此目标不是选一个万能 Store，而是定义一套**与 Store 无关的逻辑语义**：

> **稳定身份 + 精确地址 + 类型化演化语义 + 薄写入协议 + 稳定读取协议。**

## 1.2 两条第一性原理

**原理一（可信强制）**：AI 只能引用「身份稳定、版本已知、来源保留、写者明确」的知识。这是底座成立的充要条件。

**原理二（repo-native 薄度）**：repo（文件 + git + grep + CLI）已经是 agent 生态的母语。使用 Git adapter 时，一个只懂 git + 文件 + grep 的现有 Coding Agent，应当**零新协议学习成本**完成 读 + 编辑 + 提交 + 追问"这条来自哪、哪个版本"；切换到 Dolt 等 store 时，Catalog 协议保持不变。

薄度是**可度量的约束**，不是事后优化：如果把"可信"的实现成本转嫁成了"使用"的学习成本，语义层就太厚了。

---

# 2. 系统边界

## 2.1 四个领域 + 两个上层职责

| 领域 | 承诺 | 明确不做 |
|---|---|---|
| **Knowledge Catalog** | 登记 Repo、ViewDefinition→ViewGeneration、Promotion | 不拥有成员知识、不写 Repo |
| **Knowledge Ingress** | 鉴权/Binding/Schema/前置条件/幂等/写路由/Receipt | 不解析内容、不做 LLM 抽取、不判语义冲突 |
| **Knowledge Repository** | 独立身份/ACL/Object/Tree/Commit/Ref/Release/Stream/保留 | 不判跨 Repo 真值、不做排序 |
| **Knowledge Access** | 在精确 Commit/Generation/ReadVersion 上读取与检索 | 不生成最终回答、不自动派生 |
| *Application*（上层） | Context Assembly、最终回答 | — |
| *Control Plane*（上层） | Watch/Diff/评估/提 Proposal/Merge/Promote | 必须仍经 Ingress，不绕写边界 |

## 2.2 四个根本区分（防陷阱）

1. **Ingress Surface ≠ Repository Primitive**：COMMIT/PROPOSAL/APPEND 是协议语义；Object/Tree/Commit/Ref 是状态语义。
2. **Catalog ≠ Repository ≠ Projection**：Catalog 组合，Repository 权威，Projection 可丢失可重建（非权威）。
3. **Release ≠ Ref ≠ View Promote**：三个独立 CAS 动作。
4. **Structure ≠ Epistemic Role ≠ Collection**：同一主题可同时是 Append / Derived / Snapshot / Projection。

## 2.3 逻辑与物理分离

逻辑层只有一套领域对象与协议；Git/Dolt/PostgreSQL/S3/OpenSearch/向量库是物理 store 或 Projection adapter。协议层只依赖统一 `Repository` 接口，不依赖具体 store。数据规模、查询形态或部署约束可以触发 store 迁移，但不得改变身份、版本和读写协议语义（K-23）。

---

# 3. 单一协议与 Store 映射（核心主线）

## 3.1 Catalog 语义只有一种

无论底层是 Git、Dolt 还是 PostgreSQL，协议对象与不变量完全相同：RepositoryIdentity、KnowledgeRef、Write Surface、ViewDefinition、ViewGeneration、Validation、Promotion。单 source 和多 source 不是两套协议；前者只是 `repo→commit` Map 只有一个成员的自然退化。

## 3.2 协议真正新增的三样

1. **身份寻址**：ObjectIdentity 与路径解耦，KnowledgeRef 稳定。
2. **来源链**：显式 Provenance/ORIGIN，超出 commit author/message。
3. **写边界**：COMMIT/PROPOSAL/APPEND + Binding，明确谁以什么语义写。

## 3.3 Store 原生映射

| 协议语义 | Git adapter | 其他 adapter 示例 |
|---|---|---|
| Snapshot COMMIT | commit + `update-ref` CAS | Dolt commit / 数据库 revision |
| PROPOSAL | candidate branch + commit | Dolt branch / candidate revision |
| LOG / DIFF / READ | git log/diff/show | Dolt/SQL 对应版本查询 |
| RESOLVE | 扫描 frontmatter/object index | 主键/索引查询 |
| APPEND | JSONL side stream（非 git tree） | SQLite WAL / event table |
| SEARCH | grep 或 SQLite FTS5 Projection | SQL/FTS/搜索服务 |

## 3.4 Catalog 操作始终属于同一协议

REGISTER / DEFINE_VIEW / RESOLVE_VIEW / CREATE_PREVIEW / VALIDATE_GENERATION / PROMOTE / ROLLBACK 等操作始终属于 Catalog 协议。source 数量为一时，ViewGeneration 自然退化为单 commit；source 增加时，同一语义完整展开为联邦，不需要切换"单人/多人模式"。

## 3.5 Repository 接口

协议层（Ingress/Access/ControlPlane/Catalog）只依赖统一 `Repository` 接口：`head/getRef/createRef/merge/applyCommit/append/resolve/read/origin/search/list`。当前实现为 FileGit；未来新增 Dolt 等 adapter 时，协议层一行不动。

## 3.6 结论

> **只定义底层没有的身份、来源、写边界；其余通过 Repository adapter 映射到成熟 store。Catalog 语义始终只有一套。**

---

# 4. 身份与引用

## 4.1 三种跨 Repo 引用

```text
KnowledgeRef       = RepositoryIdentity + ObjectIdentity            # 长期关系，路径无关
PinnedKnowledgeRef = + CommitIdentity                              # 可复现证据/审计/Derived 输入
FileRef            = RepositoryIdentity + Commit + Path + Digest   # 只定位原始文件
```

推论：**文件移动不破坏前两者**（Path 只是 `path_hint`）；跨 Repo API 不接受裸 StableRef。

## 4.2 身份载体决策（化解"硬骨头 1"）

身份与路径的解耦会逼出一个问题：逻辑层坚持 ObjectIdentity 与路径无关，物理层却是按路径寻址的 git tree，中间必然存在一张可变的 `object_id ↔ path` 映射。这张映射若作为独立状态，会变成需要额外一致性契约的"第四类状态"。

**决策**：

- **ObjectIdentity 的权威载体 = 文件内容里的稳定字段**（frontmatter `object_id`），不是路径，也不是独立映射文件。
- **address-map（object_id → path）是「可重建 Projection」**，非 Canonical，由"扫描每个文件的 object_id 字段"生成，可丢弃重建。

于是身份内嵌于 git 树，随同一次 commit 与内容**原子演化**，权威永远在 git 树。不存在需要额外一致性契约的第四类状态。

## 4.3 唯一性与移动

- repo 内 object_id 全局唯一；写入时校验，重复 → `OBJECT_ID_CONFLICT`。
- 移动文件 = 改 path_hint，object_id 不变；KnowledgeRef 不失效。
- 一个文件承载一个维护单元，避免"一文件多 object_id"的歧义。

---

# 5. 写入边界

## 5.1 两个最小 Surface

**COMMIT**（权威当前状态变化）= git commit + CAS ref。变更代数只有：

```text
PUT(address, full_value)      Create=PUT+IF_ABSENT；Update=PUT+IF_*_EQUALS；Upsert=PUT 无目标条件
REMOVE(address)               保留 Git 历史，不无痕删除
```

**APPEND**（记录发生的事件/观察）= append-only 流。不原地修订；修正/撤回用 `correction_of / retraction_of / supersedes` 新 Entry 表达。

## 5.2 不变量

- 一次 COMMIT 只写一个 target Repository；View 不可写。
- RefUpdate 必须带 expected-old；Change 带 expected 前置条件（**禁止 silent last-write-wins**）。
- 幂等：同 idempotency namespace + command_id + 规范化逻辑 digest → 返回原 Receipt（REPLAYED）；同 ID 异内容 → `IDEMPOTENCY_CONFLICT`。
- 不允许静默降级：强语义请求被弱语义实现时 → `CAPABILITY_UNSATISFIED`。

## 5.3 失败模型（摘录）

`PRECONDITION_FAILED` / `NON_FAST_FORWARD` / `EVENT_ID_CONFLICT` / `IDEMPOTENCY_CONFLICT` / `POSITION_REGRESSION` / `WRITE_TARGET_REQUIRED` / `TARGET_REPOSITORY_DENIED` / `CAPABILITY_UNSATISFIED` / `SCHEMA_REVISION_UNRESOLVED`。

---

# 6. 读取边界

## 6.1 操作

```text
RESOLVE · READ_OBJECT · LIST_TREE · LOG · DIFF · SEARCH · ORIGIN · DESCRIBE_SCHEMA
```

覆盖：身份解析 / 精确读 / 浏览 / 历史 / 变化 / 检索 / 来源链 / 结构内省。

## 6.2 结果强制字段

每个结果显式携带：`repository / commit / object provenance`、`complete / partial`、`coverage / projection lag`、`truncated / continuation`、`missing_capabilities`、`degradation`。

## 6.3 关键规则

- Branch/Release 符号名只在请求开始解析一次，结果返回 Commit（可复现）。
- 授权按读取时 Principal + 当前 Policy 重新判断（**Generation 锁数据，不冻结 ACL**）。
- 防旁路泄漏：无权 Repo 不得通过计数、错误差异、搜索片段、关系边、Provenance 泄漏。
- SEARCH 零结果分层：EXACT_REF/PATH 可确定 `NOT_FOUND`；LEXICAL 仅完整且 basis 匹配时可证明；SEMANTIC 只能说"近似检索未发现"。

---

# 7. 核心契约

## 7.1 RESOLVE（身份寻址）

```text
RESOLVE(refs[1..N], commit_id) → ResolutionSet
Resolution: { repository, commit, objectId, address, pathHint, digest, schemaRef,
              status: RESOLVED | REMOVED | UNRESOLVED | FORBIDDEN }
```

普通 KnowledgeRef 在给定 commit 解析；Pinned 只验证目标仍可访问，不重新跟随上游。删除 → `REMOVED`（非崩溃/非静默）。

## 7.2 ORIGIN（来源链）

```text
ORIGIN(target_address, commit_id) → ProvenanceTrace
链：Current Value ← Repo/Commit/Object ← Activity ← Source/Evidence ← Principal/Algorithm
```

最小 Provenance Envelope：`origin_kind / actor_ref / activity_ref / source_refs / evidence_refs / input_view_read_version_ref / algorithm{derivation_spec_ref, model_ref, code_hash}`。

DERIVATION 强制 `input version + algorithm`（否则来源链断）。按知识族最小义务：定义/断言需 actor+commit+evidence；派生需 input+spec+activity；Artifact 需 hash+media type+source。

## 7.3 INGEST / RECONCILE（低摩擦采集）

二者**都是 COMMIT 之上的薄编排，不是新 Surface**：

- `INGEST(dir)`：扫描目录，一个文件 = 一个 object，provenance originKind=SOURCE。
- `RECONCILE(snapshot)`：外部快照与 repo 现状做 set-diff（同 object_id 不同 digest = 更新；外部缺失 = REMOVE），产出 ChangeSet 预览，确认后 COMMIT。

不判断内容正确性（Ingress 语义薄），只做"外部状态 → 目标地址 → PUT/REMOVE"的机械翻译。

## 7.4 GroundingCitation（grounding 消费）

```text
GroundingCitation: { knowledgeRef, pinnedRef, digest, fragment, provenanceSummary }
```

**硬不变量**：AI 的每个事实性断言，要么携带 PinnedKnowledgeRef（+ fragment + provenance），要么显式标记"无依据/模型推断"。二者不可混淆——模型不能把推断伪装成已引用的知识。

---

# 8. 不变量（K-01..K-24）

| # | 不变量 |
|---|---|
| K-01 | 每个 Command 唯一 target_repository；View 不可写 |
| K-02 | 每个 Repository 独立身份、ACL、Commit 图、Ref、Release、生命周期 |
| K-03 | public/group/personal 是治理 Scope，不是目录优先级 |
| K-04 | KnowledgeRef 不依赖路径；Pinned 固定 Commit；FileRef 固定 Path/Digest |
| K-05 | Object/Tree/Commit/Release/已接受 Stream Record 不可变 |
| K-06 | RefUpdate 带 expected-old；Change 带 expected 前置条件（禁 silent LWW） |
| K-07 | Proposal 指向 Candidate Branch/Commit；Proposal Durable 不表示 main 已变 |
| K-08 | Review/Validation/Approval/MergeGate 绑定精确候选 Commit |
| K-09 | ValidationReport 绑定完整 PreviewGeneration，非只绑候选 Commit |
| K-10 | ViewGeneration 每 RepoIdentity 只出现一次 |
| K-11 | Branch/Release 只出现在 ViewDefinition 与解析证据；稳定读取不跟随 latest |
| K-12 | 联合结果保留 source Repository/Commit/Object/Scope/Provenance |
| K-13 | 同一主题多来源 Assertion 并存，不按 Scope 静默覆盖 |
| K-14 | 普通引用升级不修改引用方 Repo，不做跨 Repo merge |
| K-15 | Fork 创建新 KnowledgeRef；仅 Fork sync 使用 Base/Upstream/Local 三方比较 |
| K-16 | Vendor 保留来源 pin 与只读副本；本地编辑必须转为 Fork |
| K-17 | APPEND Entry 不原地修订；修正/撤回通过新 Entry |
| K-18 | 相同幂等键与相同 Digest 返回原 Receipt；不同 Digest 冲突 |
| K-19 | Projection 非 Canonical，须声明 basis 和 coverage |
| K-20 | ViewGeneration 锁数据不锁权限；授权审计另存 AuthorizationDecisionRef |
| K-21 | Application 与 Control Plane 不得绕过 Ingress 写 Backend/Ref |
| K-22 | 不构造跨 Repository 的虚假单一事务 |
| K-23 | Profile 迁移不改身份/版本/读写协议语义 |
| K-24 | 不存在 DELETE_REPOSITORY；生命周期终点是 ARCHIVE |

---

# 9. 维护闭环与 Catalog 联邦

## 9.1 维护闭环（统一协议）

```text
PROPOSAL（Candidate Branch/Commit，不改 main）
→ CREATE_PREVIEW（只替换一个 Repo Commit，其余不变 → 完整 PreviewGeneration）
→ VALIDATE_GENERATION（报告绑定完整 PreviewGeneration）
→ Review/Approval/MergeGate（绑定精确候选 Commit）
→ Repository MERGE（CAS 移动 main）
→ RESOLVE_VIEW → VALIDATE_GENERATION → PROMOTE_GENERATION（CAS 移动 Catalog 指针）
```

强不变量：**测试必须绑完整 PreviewGeneration；Repository Merge 与 Catalog Promote 两步分离；candidate 前移或任何参与 Repo 变化都使旧 Validation 失效。**

## 9.2 Catalog 联邦（同一语义在多 Repo 上展开）

- **ViewDefinition**（可变意图，含 Branch/Release 选择器）→ **ViewGeneration**（不可变 `repo→commit` Map）。
- **generation_id 是确定函数**：`H(definition_revision ‖ sorted({repo→commit}))`，同输入同 id（幂等/可重放/可缓存）。
- **联合读保留来源、不覆盖**：同一 object_id 在多个 Repo 存在时，返回所有来源（各自带 source commit），不按 Scope 覆盖。
- **Promote/Rollback 是 channel 的 CAS**，只移 Catalog 指针，不改任何 Repo。

## 9.3 三种跨 Repo 关系

| 语义 | 本地复制 | 对象身份 | 上游升级 |
|---|---|---|---|
| Reference | 否 | 上游 KnowledgeRef | 新 Generation 重解析验证 |
| Fork | 是 | 新本地身份 + wasDerivedFrom | 显式三方同步，可冲突 |
| Vendor | 是 | 本地只读副本 + 锁 pin | 显式 update，不假装自动跟随 |

普通引用升级无跨 Repo merge（下游 Repo 未被修改）。

## 9.4 回滚分层（不可混用）

| 层 | 动作 | 修什么 |
|---|---|---|
| Projection | Rebuild | 索引/访问状态 |
| Catalog | ROLLBACK_PROMOTION | Serving 组合（不动 Repo） |
| Repository | REVERT | 权威内容（保留历史） |

---

# 10. 附录：代码骨架验证

本白皮书的语义已由一份可运行的 TypeScript 代码骨架验证（`~/Desktop/knowledge-catalog`），运行时只使用 Node 内置能力（含 `node:sqlite`；vitest 仅为开发测试依赖）。T1–T11 共 32 个 conformance case，把协议不变量变成可执行断言：

| 测试 | 不变量 |
|---|---|
| T1 | 路径移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 | 过期 expected target commit 被拒绝（CAS） |
| T3 | 任一操作失败无部分提交（原子性） |
| T4 | 精确重试返回原 Receipt；异内容冲突（幂等） |
| T5 | 同 event id 同内容重放；异内容冲突（append 幂等） |
| T6 | 真实文件 + git：frontmatter 内嵌 object_id、移动、CAS、ORIGIN |
| T7 | ingest 扫描、reconcile 对账、groundingCitation 投影 |
| T8 | SQLite FTS5 投影可重建、非权威、记录 basis/lag |
| T9 | Proposal 隔离、candidate 前移失效、Merge CAS、Promote 分离 |
| T10 | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup + unjudged |
| T11 | 多 Repo 联合：确定性 generation、K-10 去重、联合读保留来源、Promote CAS |

**当前实现**：协议层只依赖统一 `Repository` 接口；FileGit 是当前 store（Snapshot=真实 git，Append=JSONL side stream），SQLite FTS5 是可重建 Projection。ControlPlane 与 Catalog 均在同一个 Git 实现上通过测试。未来新增 Dolt/PostgreSQL adapter 时，协议层与测试不变量保持不变。

---

## 术语表（摘录）

- **KnowledgeRef**：RepositoryIdentity + ObjectIdentity；在 ViewGeneration 内解析。
- **PinnedKnowledgeRef**：再含 CommitIdentity 的可复现引用。
- **ViewGeneration**：`RepositoryIdentity → CommitIdentity` 的不可变联合快照。
- **Projection**：从固定 Generation/ReadVersion 可重建的访问状态（非权威）。
- **GroundingCitation**：AI 事实断言携带 PinnedKnowledgeRef + provenance 的约定投影。
- **Semantic Refinement**：在固定 CandidateSet 上 Ref-preserving 的 Filter/Rerank。
