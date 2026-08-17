# Knowledge Catalog 系统设计

> 由 WorkSurface（surface.md + blocks）模板组装生成。
> 权威文档是 WorkSurface 本身；本文件是可分发快照。

---

# Goal

把「Knowledge Catalog 系统设计」从两份源文档整理为结构化工作状态，并在本工作面上完成优化完善。本工作面即新的权威设计文档：surface.md 为骨架，blocks 为正文章节，模板组装即最终结果。已完成：P0→P1→P2 精炼、最小语义层收敛、核心契约（RESOLVE/ORIGIN）、一序缺口（采集/grounding）、Phase 0 代码骨架（TypeScript + conformance T1–T5）。

# Acceptance Criteria

- 工作面七节齐全。
- 领域 8 block + gap-analysis + refinements（p0/p1/p2）+ minimal-semantic-layer + minimal-core-contracts + ingestion-and-grounding。
- 最小语义层收敛（F/G/D 三档 + 逐操作矩阵）。
- RESOLVE/ORIGIN 最小契约冻结（身份载体决策化解硬骨头 1）。
- 低摩擦采集 + grounding 消费契约已补。
- Phase 0 代码骨架已交付：`~/Desktop/knowledge-catalog`（TypeScript），conformance T1–T5 通过。

# Known Facts and Constraints

- 历史输入材料两份（已降级，不再回写维护）：
  - `9c4f28b2-fbc1-4896-97c1-8b66ebbff0a1.md` — 白皮书 v5.0。
  - `Knowledge_Catalog_Git_Semantics_Full_Walkthrough.md` — 全流程推演 v4.0。
- 系统不变量：K-01..K-23（追加 K-24）；推演 §0 另有 12 条（映射见 refinements-p0 P0-2）。
- 已提交工作面 revision：上一版 `sha256:1c3b62de...`，本版固化「WorkSurface 即权威文档」决定。
- 代码骨架 git 已提交，typecheck + 5 测试通过。

# Assumptions

- 本工作面是「当前权威设计文档」；两份源 `.md` 是历史输入，不再维护或回写。
- 系统定位：面向「团队/组织共用」的 AI 知识底座；repo-native 是采用层第一性。
- 语义层 = 逻辑契约；适配器 = 物理 Profile；单人/团队是同一设计、两个 Profile。
- Phase 0 语言 = TypeScript。

# Open Questions

- O14 Phase 1（File+Git Profile）何时启动（当前 Phase 0 Memory 已完成）。
- O15 是否需要一个「模板组装」脚本，把 surface.md + blocks 渲染成单文件设计文档（待定）。

# Current Decisions

- D1 四领域边界 + 两上层职责；逻辑架构与物理 Profile 分离。
- D2 身份与路径分离：KnowledgeRef / PinnedKnowledgeRef / FileRef。
- D3 写入显式路由 + View 只读；一次 ChangeSet 只写一个 target Repo。
- D4 维护闭环：Candidate → PreviewGeneration → Validation → Merge → Promote。
- D5 回滚分层：Projection Rebuild / Catalog ROLLBACK_PROMOTION / Repository REVERT 不可混用。
- D6 工作面结构：surface.md 为索引，blocks/ 承载领域内容与优化产物。
- D7（P0）文档一致性：C1/C2/C3。
- D8（P1）MVP 契约冻结：G2/G3/G6/G7/G9。
- D9（P2）治理契约骨架：G1/G4/G5/G8。
- D10（最小语义层）：只发明 git 没有的三样——身份、来源、写边界；其余坍缩成 git 原生 + 薄 CLI。
- D11（Phase 0 冻结 + 核心契约）：RESOLVE 身份载体 = object_id 内嵌文件内容；ORIGIN = 最小 provenance 链。
- D12（一序缺口）：INGEST/RECONCILE 是 COMMIT 之上的薄编排；GroundingCitation 是 Access 结果的约定投影。
- D13（Phase 0 代码骨架）：TypeScript 实现最小语义层，conformance T1–T5 通过。
- D14（文档定位）：WorkSurface 即新的权威设计文档；源文档降级为历史输入；surface.md + blocks 模板组装即最终结果，不回写源文件。


---

# 正文

# Architecture（系统主张与四领域边界）

源：白皮书 §0–§3；推演 §0–§1。

## 系统主张
Knowledge Catalog 不是放大版 Repository，也不是跨 Repo 文件覆盖系统。它是多个独立权威 Knowledge Repository 的注册、发现与版本化联合视图边界。目标是与 Store 无关的逻辑语义：

> 稳定身份 + 精确地址 + 类型化演化语义 + 薄写入协议 + 稳定读取协议。

## 四个核心领域边界 + 两个上层职责
| 领域 | 承诺 | 明确不做 |
|---|---|---|
| Knowledge Catalog | 登记 Repo、ViewDefinition→ViewGeneration、Promotion | 不拥有成员知识、不写 Repo |
| Knowledge Ingress | 鉴权/Binding/Schema/前置条件/幂等/写路由/Receipt | 不解析内容、不做 LLM 抽取、不判语义冲突 |
| Knowledge Repository | 独立身份/ACL/Object/Tree/Commit/Ref/Release/Stream/保留 | 不判跨 Repo 真值、不做排序 |
| Knowledge Access | 在精确 Commit/Generation/ReadVersion 上读取与检索 | 不生成最终回答、不自动派生 |
| Application（上层） | Context Assembly、最终回答 | — |
| Active Control Plane（上层） | Watch/Diff/评估/提 Proposal/Merge/Promote | 必须仍经 Ingress，不绕写边界 |

## 四个根本区分（防陷阱）
1. **Ingress Surface ≠ Repository Primitive**：COMMIT/PROPOSAL/APPEND 是协议语义；Object/Tree/Commit/Ref 是状态语义。
2. **Catalog ≠ Repository ≠ Projection**：Catalog 组合，Repository 权威，Projection 可丢失可重建（非权威）。
3. **Release ≠ Ref ≠ View Promote**：三个独立 CAS 动作。
4. **Structure ≠ Epistemic Role ≠ Collection**：同一主题可同时是 Append/ Derived/ Snapshot/ Projection。

## MVP 语义压缩
Catalog 4 对象 / Ingress 3 Surface / Git 内核 / 4 Collection / 4 Pattern / 3 引用 / 2 View 对象 / 12 Access 操作 / 2 语义算子。

## 逻辑 vs 物理
逻辑层只有领域对象与协议；Files/Git/SQLite/PostgreSQL/S3/OpenSearch/向量库是物理 Profile。Portable 与 Managed Profile 共享协议，可迁移不改变语义（K-23）。


# Identity（身份、地址与版本）

源：白皮书 §14、§17A；推演 §0、§2.2。

## 三种跨 Repo 引用
```text
KnowledgeRef       = RepositoryIdentity + ObjectIdentity            # 长期关系，路径无关
PinnedKnowledgeRef = + CommitIdentity                              # 可复现证据/审计/Derived 输入
FileRef            = RepositoryIdentity + Commit + Path + Digest   # 只定位原始文件
```
推论：文件移动不破坏前两者（Path 只是 path_hint）；跨 Repo API 不接受裸 StableRef（ADR-008）。

## KnowledgeAddress（Repo 内精确定位）
```text
EntityAddress / AspectAddress / MemberAddress / RelationAddress / RecordAddress
DerivedOutputAddress / ArtifactAddress / FragmentAddress
```
RepositoryIdentity 来自请求上下文或 KnowledgeRef，不能靠 Path 推断。

## Repository 版本对象
CommitIdentity / BranchRef / Tag·Release / AppendCursor / DerivedRevision / ArtifactRevision / ProposalRef。
Branch 是可移动指针，不能充当可复现证据（Review/Validation/Approval/Release 必须落精确 Commit）。

## ViewGeneration vs ViewReadVersion
- ViewGeneration = `{RepositoryIdentity → CommitIdentity}` 不可变联合快照，锁每个权威 Snapshot 的 Git Commit。
- ViewReadVersion = Generation + Append Cuts + Derived Heads Manifests + Projection Generations + AuthorizationDecisionRef。
- 二者不能合并成虚假全局 Commit（ADR-019）；Generation 锁数据，不冻结 ACL（K-20）。


# Ingress（薄写入边界与三 Surface）

源：白皮书 §4–§9；推演 §2、§3、§7。

## 定义
Ingress = 能力协商 + 单 Surface Binding + 类型化写命令 + 契约执行 + 幂等/并发/顺序 + Durable Receipt。语义薄、执行强：不判内容权威，但严格机械执行。

## 三种 Write Surface
| Surface | 语义 | 状态效果 |
|---|---|---|
| COMMIT | 权威当前状态变化 | target Repo Commit + CAS target Ref |
| PROPOSAL | 建议状态变化 | Candidate Branch/Commit + Proposal Metadata |
| APPEND | 记录发生的事件/观察 | 不可变 Entry 追加到 Stream |

Artifact 上传（PutArtifact）不是第四种 Surface，是 Repository 辅助端点；被 COMMIT/PROPOSAL/APPEND 引用后才进入知识语义。

## 变更代数与前置条件
- 变更代数只有 `PUT(address, full_value)` / `REMOVE(address)`；无通用 PATCH（ADR-004）。
- 前置条件：`IF_ABSENT` / `IF_OBJECT_EQUALS` / `IF_DIGEST_EQUALS`。Create=PUT+IF_ABSENT；Update=PUT+IF_*_EQUALS；Upsert=PUT 无目标条件（需 Binding 允许）。

## WriteBinding 不变量
一个 Binding 只对应一个 Surface（最小权限、幂等命名空间稳定、监控/限流/错误分离）。Binding 声明 target_repository、allowed_target_refs、allowed_address_patterns、allowed_schema_refs、allowed_operations、precondition/ordering/idempotency/durability profile、limits。不允许静默降级（`CAPABILITY_UNSATISFIED`）。

## 执行流程与幂等
Transport Decode → Authenticate → Resolve Binding → Verify Surface → Canonicalize+Digest → Idempotency Check → Validate Scope/Schema/Op/Limits → Typed Executor → Atomic Preconditions+Write+Receipt → Return Durable Receipt。
精确重试：同 idempotency namespace + command_id + 规范化逻辑 Payload；成功返回原 Receipt（REPLAYED），同 ID 异内容 → IDEMPOTENCY_CONFLICT。

## 最小错误模型
PROTOCOL_UNSUPPORTED / BINDING_EXPIRED·REVOKED / SURFACE_MISMATCH / SCOPE_DENIED / SCHEMA_UNSUPPORTED / WRITE_TARGET_REQUIRED / TARGET_REPOSITORY_DENIED / PRECONDITION_FAILED / POSITION_REGRESSION / IDEMPOTENCY_CONFLICT / EVENT_ID_CONFLICT / CAPABILITY_UNSATISFIED / TEMPORARY_UNAVAILABLE。


# Repository（Git 内核、四 Collection、四 Pattern）

源：白皮书 §10–§18；推演 §2、§7、§12、§13。

## 核心抽象
RepositoryIdentity/Ownership/ACL + Git 状态内核（Object/Commit/Ref/Merge/Release）+ AddressSpace + 类型化 Collection + 结构契约 + Append/Derived/Artifact 侧集合 + Provenance/Time + CapabilityManifest。

## 受保护的 Git-like Control API
CREATE_REPOSITORY / CREATE_COMMIT / CREATE_REF / UPDATE_REF(CAS) / MERGE / REBASE_CANDIDATE / PUBLISH_RELEASE / REVERT / ARCHIVE_REPOSITORY。
禁止：强制覆盖受保护 Ref、删除已发布 Release、跨 Repo 原子 Merge。

## 四类 Canonical Collection
| Collection | 适用 | 演化语义 | 是否当前权威 |
|---|---|---|---|
| Snapshot | 定义/政策/流程/断言/关系/Schema | Git Commit/Ref/Release | 是 |
| Append | 观察/证据/反馈/轨迹 | append-only；修正用 correction/retraction/supersedes | 是真实记录，非快照 |
| Derived | 摘要/健康/热度/评估 | 不可变 Revision + Head CAS；invalidate | 仅声明 Derived Head |
| Artifact | PDF/大对象 | 内容 Hash 寻址；Manifest 引用 | 需引用才进语义 |

## 四个 Structural Pattern（只答「最小独立维护单位」）
Record / Keyed Collection / Ordered Artifact / Relation Set。
Append/Derived 是 Collection 演化语义，不是 Pattern（本版主动简化）。

## 状态转移要点
- COMMIT：验证 Address/Schema/Pattern → 解析 parent + Ref CAS → 建 object/tree → Git Commit → CAS 移 Ref → Change Event → Receipt。
- PROPOSAL：base_commit 建 Candidate Commit → CAS 更新 candidate ref → Proposal Metadata 记录（不移动 main/Release）。
- APPEND：校验 Stream/Schema/EventID → 幂等 + append → RecordRef/cursor → Receipt。
- Derived：读固定 ViewReadVersion → 外部计算 → COMMIT 到 DerivedOutputAddress + DerivationEnvelope → 不可变 Revision + CAS Head。

## 冲突语义
同 Repo Ref 前移 → PRECONDITION_FAILED；同 EventID 异内容 → EVENT_ID_CONFLICT；不同 Repo 断言矛盾 → 并存不覆盖；Fork/Vendor 同步 → Base/Upstream/Local 三方；普通 KnowledgeRef 随上游升级 → 重新解析验证，不 merge。


# Catalog（多 Repo 组合与版本化联合视图）

源：白皮书 §17A；推演 §5、§8–§11。

## public / group / personal
权威、权限、维护边界 → 默认独立 Repo；不是目录层级，没有覆盖优先级。联合结果保留 repository_id/commit_id/object_id/scope/provenance，多来源 Assertion 并存（K-13）。
拆分判断：所有权、ACL、Release 节奏、历史可见性一致时才合并进同一 Repo。

## ViewDefinition → ViewGeneration
- ViewDefinition 含可移动选择器（branch/release），是意图；每次稳定读取先 RESOLVE 成不可变 ViewGeneration。
- ViewGeneration 每 RepoIdentity 只出现一个 Commit（K-10）；同一 RepoIdentity 出现两次 → VIEW_GENERATION_INVALID。
- EffectiveView = union AuthorizedSnapshot(repo_i, commit_i, principal)；无覆盖栈（ADR-010）。

## 三种跨 Repo 关系
| 语义 | 本地复制 | 对象身份 | 上游升级 |
|---|---|---|---|
| Reference | 否 | 上游 KnowledgeRef | 新 Generation 重解析验证 |
| Fork | 是 | 新本地身份 + wasDerivedFrom | 显式三方同步，可冲突 |
| Vendor | 是 | 本地只读副本 + 锁 pin | 显式 update，不假装自动跟随 |

普通引用升级无跨 Repo merge（下游 Repo 未被修改）。

## Catalog 动作语义（只改登记/联合视图，不写 Repo）
REGISTER_REPOSITORY / UPDATE_REGISTRATION / DEFINE_VIEW / RESOLVE_VIEW / CREATE_PREVIEW / VALIDATE_GENERATION / PROMOTE_GENERATION / ROLLBACK_PROMOTION / RETIRE_DEFINITION。
PROMOTE 只移 Catalog 指针；失败继续服务旧 Generation。

## 本地分歧表达
group/personal 想补充/限定/反对 public → 写本地 Assertion/Relation（about: kc://public/...）；通用 OverlayPatch 不进 MVP。展示名/排序/高亮等非知识设置可用 Presentation Preference。


# Access（十二操作、结果模型、Projection、Refine）

源：白皮书 §19–§25；推演 §6、§7.2。

## 十二个 Core Operation
```text
CAPABILITIES · DESCRIBE_SCHEMA · DESCRIBE_INDEX
RESOLVE · READ_OBJECT · LIST_TREE
LOG · DIFF · ORIGIN
SEARCH · EXPAND_RELATIONS · WATCH_UPDATES
```
覆盖：能力发现 / 结构 / 视图组合 / 身份解析 / 精确读 / 浏览 / 历史 / 变化 / 来源 / 检索 / 一跳关系 / 维护通知。

## 结果强制字段
repository/commit/object provenance · view_generation/view_read_version · authorization_decision_ref · complete/partial · coverage/projection_lag · truncated/continuation · missing_capabilities · degradation。

## 零结果语义分层（SEARCH）
EXACT_REF/PATH → 可确定 NOT_FOUND；STRUCTURED/LEXICAL/REGEX → 仅完整且 basis 匹配时可证明；SEMANTIC/HYBRID → 只能说「近似检索未发现」。

## 关键规则
- Branch/Release 符号名只在请求开始解析一次，结果返回 Commit/Generation（可复现）。
- 授权按读取时 Principal + 当前 Policy 重新判断（Generation 锁数据不锁 ACL）。
- 防旁路泄漏：无权 Repo 不得通过计数、错误差异、搜索片段、关系边、Provenance 泄漏。
- EXPAND_RELATIONS 只保证一跳（depth=1）；跨 Repo 边两端独立授权裁剪。
- WATCH_UPDATES 至少一次投递；事件不携带完整 payload，消费者用 DESCRIBE_INDEX/DIFF/READ_OBJECT 取确定状态。

## 结果模型
RefSummary / KnowledgeValue / CandidateSet（多通道 ChannelEvidence，不伪造统一概率）/ GraphSlice / KnowledgeReadResult。

## Optional Semantic Refinement（Ref-preserving）
SEMANTIC_FILTER（子集）/ SEMANTIC_RERANK（RankGroup）。Filter 三值：MATCH/NO_MATCH/UNKNOWN，另有 UNJUDGED；不创造新 Ref、不调工具、无副作用（与 Agent 边界）。Base Profile 可声明 supported:false。

## Projection 归属 Access
Catalog/Text/Vector/Graph/Fragment 投影均可重建、记录 Source ViewReadVersion、不成为 KnowledgeRef 来源、失败不反写 Repo、切 Generation 显式报告。


# Maintenance（维护闭环、回滚分层、失败恢复、实施路线）

源：白皮书 §26–§31；推演 §7–§15。

## 维护闭环
```text
WATCH_UPDATES → DIFF/READ/SEARCH（发现陈旧/冲突/缺失）
→ PROPOSAL（Candidate Branch/Commit）
→ CREATE_PREVIEW（只替换一个 Repo Commit，其余不变 → 完整 PreviewGeneration）
→ VALIDATE_GENERATION（报告绑定完整 PreviewGeneration，非只绑 Candidate）
→ Review/Approval/MergeGate（绑定精确候选 Commit）
→ Repository MERGE（CAS 移动 main）
→ RESOLVE_VIEW → VALIDATE_GENERATION → PROMOTE_GENERATION（CAS 移动 Catalog 指针）
```
强不变量：测试必须绑完整 PreviewGeneration；Repository Merge 与 Catalog Promote 两步分离；Candidate 前移或任何参与 Repo 变化都使旧 Validation 失效。

## 回滚分层（不可混用）
| 层 | 动作 | 修什么 |
|---|---|---|
| Projection | Rebuild | 索引/访问状态 |
| Catalog | ROLLBACK_PROMOTION | Serving 组合（不动 Repo） |
| Repository | REVERT | 权威内容（保留历史） |

## 三个独立观察点
Repository Commit / Projection Ready / Catalog Promote 是三个独立观察点；CommitReceipt 返回不代表 Search Projection 或稳定 View 已同步。

## 失败恢复（示例）
PRECONDITION_FAILED → READ/DIFF 后重建；NON_FAST_FORWARD → LOG/DIFF + rebase/新 Candidate；CANDIDATE_MOVED → 解析新 Preview 重测；VALIDATION_BASIS_MISMATCH → 对当前完整 PVG 重测；PROMOTION_CAS_FAILED → DESCRIBE_INDEX 后决定保留/重验/再 Promote；任何 Ref CAS 失败不得报告成功。

## 实施路线
Phase 0 契约冻结 + Memory Adapter → Phase 1 Portable File Profile（多 Git Repo + SQLite + CAS + ripgrep CLI）→ Phase 2 Embedded Access（SQLite FTS5/邻接/覆盖率）→ Phase 3 主动维护试点（窄闭环）→ Phase 4 可选 Semantic Refinement。

## 结束到结束恢复
备份必须同时验证：Repository Object/Commit/Ref + Catalog Definition/Generation/Promotion + Append Cursor + Derived Head + Artifact Digest + Receipt/Audit；仅恢复 Git 目录不能重建完整 ViewReadVersion。


# Decisions（ADR 与不变量）

源：白皮书 §32（ADR-001..020）、§2.2（K-01..K-23）。

## ADR 要点（20 条）
- ADR-001 Catalog/Repository 两个公开领域边界
- ADR-002 Ingress 三 Surface，单 Binding 单 Surface
- ADR-003 Snapshot 复用 Git 状态语义
- ADR-004 Snapshot 只 PUT/REMOVE，无通用 PATCH
- ADR-005 四类 Canonical Collection
- ADR-006 四 Pattern 只描述结构
- ADR-007 Entity/Aspect 是维护内核
- ADR-008 KnowledgeRef 不用 Path 作身份
- ADR-009 ViewDefinition 与 ViewGeneration 分离
- ADR-010 联合 View 不是覆盖栈
- ADR-011 联合 View 不可写
- ADR-012 Proposal = Candidate Commit + 非权威 Metadata
- ADR-013 Candidate 测试绑定完整 PreviewGeneration
- ADR-014 Access 十二固定操作
- ADR-015 Projection 归属 Access
- ADR-016 Graph 只保证一跳
- ADR-017 Semantic Refinement 可选、Ref-preserving
- ADR-018 多通道候选保留 ChannelEvidence
- ADR-019 ViewGeneration 与 ViewReadVersion 分离
- ADR-020 MVP 先实现 Portable Multi-Repo Profile

## 核心不变量（K，摘录最关键的）
- K-01 每个 Command 唯一 target_repository；View 不可写
- K-03 public/group/personal 是治理 Scope，非目录优先级
- K-04 KnowledgeRef 不依赖路径；Pinned 固定 Commit；FileRef 固定 Path/Digest
- K-05 Object/Tree/Commit/Release/已接受 Stream Record 不可变
- K-06 RefUpdate 带 expected-old；Change 带 expected 前置条件（禁 silent LWW）
- K-09 ValidationReport 绑完整 PreviewGeneration
- K-10 ViewGeneration 每 Repo 只出现一次
- K-12 联合结果保留 source Repo/Commit/Object/Scope/Provenance
- K-13 多来源 Assertion 并存，不按 Scope 覆盖
- K-14 普通引用升级不修改引用方 Repo，不做跨 Repo merge
- K-19 Projection 非 Canonical，须声明 basis 和 coverage
- K-20 ViewGeneration 锁数据不锁权限；授权审计另存 AuthorizationDecisionRef
- K-22 不构造跨 Repo 虚假单一事务
- K-23 Profile 迁移不改身份/版本/读写协议语义

## 被明确拒绝的设计
Ingress=ETL+LLM；Catalog=Git Repo/PostgreSQL；write(payload) 无 Surface；全 JSON；统一复杂 status；Projection 作权威；通用 PATCH DSL；完整图查询语言；LLM_QUERY(whole_repo)；审批只绑 Branch 名；Knowledge OverlayPatch；View 跟随 latest。


# Minimal Semantic Layer（最小语义层：可信强制 vs 可坍缩 vs 可延后）

目标：以「repo-native 薄度」为尺子，逐个审查写侧 3 Surface、Catalog 9 操作、Access 12 操作，分出可信强制 / 可坍缩 / 可延后，收敛出最小语义层。

## 判据（第一性原理）

两条硬性质推导：
1. **可信强制**：AI 只能引用「身份稳定、版本已知、来源保留、写者明确」的知识；这是底座成立的充要条件。
2. **repo-native 薄度**：单人 Profile 下，一个只懂 git + 文件 + grep 的现有 Coding Agent，不学新协议即可完成 读 + 编辑 + 提交 + 追问「这条来自哪、哪个版本」。

四档：
- **F（Floor，可信强制）**：任何 Profile 都不能少的语义，否则底座不成立。
- **G（Git-native，可坍缩）**：语义必须，但 git/文件/grep 已原生提供，只需一层薄翻译，不新造协议。
- **D（Defer，可延后）**：多人/多 Repo/协作才产生的语义，单人 MVP 不需要。
- **E（Eliminate，MVP 消除）**：概念税，MVP 不展开，多人再展开。

## 一、写侧：3 个 Ingress Surface

| Surface | 档 | 理由 |
|---|---|---|
| COMMIT | G | 写边界是可信强制；但「权威写入」= git commit + ref 更新，git 原生。语义层只需把 PUT/REMOVE 翻译成 git commit，前置条件=CAS ref。 |
| PROPOSAL | D | 「候选 vs 权威」是可信区分；但它 = git branch + commit + review，git 原生。单人 MVP 用 branch 即可；多人再展开为正式 Proposal 治理（Receipt ≠ main 已变）。 |
| APPEND | F | append-only 记录（观察/事件/反馈）不是 git 快照语义，是「来源保留」里『发生过什么』的证据链，git 没有，必须一等公民。实现可先用 append 文件/SQLite，但语义层不可坍缩。 |

结论：写侧最小层 = **COMMIT（git 化）+ APPEND**；PROPOSAL 延后（单人用 git branch）。

## 二、Catalog：9 个操作

| 操作 | 档 | 理由 |
|---|---|---|
| REGISTER_REPOSITORY | D | 多 Repo 联合才需要；单 Repo 无需注册 |
| UPDATE_REGISTRATION | D | 同上 |
| DEFINE_VIEW | D | 多 Repo 联合视图；单人 = 单 Repo HEAD |
| RESOLVE_VIEW | D | 单人坍缩为「当前 commit 即版本坐标」；多人是核心 |
| CREATE_PREVIEW | D | 候选组合测试，团队协作 |
| VALIDATE_GENERATION | D | 绑定完整组合，团队协作 |
| PROMOTE_GENERATION | D | 单人 = HEAD 即 serving；多人是核心 |
| ROLLBACK_PROMOTION | D | 单人 = git revert/reset |
| RETIRE_DEFINITION | D | 团队 view 治理 |

结论：**9 个 Catalog 操作全部可延后（单人）或坍缩成 git（多人单 Repo 内）。Catalog 作为「领域」是『多 Repo 联合』的产物，不是可信底座的地板。** 单人 Profile 下 Catalog 坍缩为一个概念：`当前 commit = 版本坐标`。

## 三、Access：12 个操作

| 操作 | 档 | 理由 |
|---|---|---|
| CAPABILITIES | D | 能力协商是多人/多 Provider 产物；单人已知能力 |
| DESCRIBE_SCHEMA | G | AI 需知「能引用什么结构」；= 读 schema 文件，git 原生 |
| DESCRIBE_INDEX | D | 多 Repo 组合视图；单人 = 当前 commit |
| RESOLVE | F | 身份→地址解析是「稳定引用」核心；单人 = 一条寻址规则，但必须新发明 |
| READ_OBJECT | G | 读对象值；= 读文件 + 反查身份映射 |
| LIST_TREE | G | 导航；= 目录 / git ls-tree |
| LOG | G | 版本历史 = git log |
| DIFF | G | 变化 = git diff |
| ORIGIN | F | 来源链；git 有 commit author/message，但不等于「从哪来、由什么产生」，需显式 provenance 层 |
| SEARCH | G | 检索 = grep / ripgrep |
| EXPAND_RELATIONS | D | 一跳关系导航是「关系集/图」产物，单人 MVP 可延后 |
| WATCH_UPDATES | D | 维护通知是多人协作/控制平面产物 |

结论：12 个里，**可信强制 8 个**（DESCRIBE_SCHEMA / RESOLVE / READ_OBJECT / LIST_TREE / LOG / DIFF / ORIGIN / SEARCH），其中 6 个是 git+grep 原生（G 化）；**真正需要「新发明」的只有 RESOLVE（身份寻址）与 ORIGIN（来源链）两个**。可延后 4 个（CAPABILITIES / DESCRIBE_INDEX / EXPAND_RELATIONS / WATCH_UPDATES）。

## 四、核心结论：最小语义层

真正必须「新发明」的语义只有三样：
1. **身份寻址**（KnowledgeRef / ObjectIdentity ≠ path）——git 没有，是「稳定引用」的唯一来源。
2. **来源链**（Provenance / ORIGIN）——git 有 commit 元数据，但不等于「这条知识从哪来、由什么产生」，需显式。
3. **写边界**（Binding/Surface 的权限语义）——git 没有「谁能以什么语义写」。

其余（版本、历史、diff、读、检索、导航、权威写）**git + 文件 + grep 已经原生提供**，语义层的职责是「翻译 + 薄映射」，不是「新协议」。

> 一句话：**别把 git 已经会的东西重新发明成协议；只发明 git 不会的那三样——身份、来源、写边界——其余全部坍缩成 git 原生 + 一个薄 CLI。**

Catalog / Preview / Promote / Watch / Expand 是「多人多 Repo 协作」才展开的第二层，不是底座地板。

## 五、对先前讨论的回响

- **硬骨头 2（并发合并）**：repo-native 视角下，「git 原生 merge」是 feature 而非缺口（agent 已会 git merge，人类已会 review PR）。正确解是保留 git 原生合并，只加一道薄校验门（merge 后重跑 Schema/Pattern 校验，失败走人工/新 Candidate）。不要自造 pattern-aware merge 引擎，否则破坏 repo-native。
- **薄度是可度量约束**：验收标准 = 单人 Profile 下，只懂 git+文件+grep 的 Coding Agent，零新文档、零新协议，完成 读+编辑+提交+溯源。不满足即说明把「可信」的实现成本转嫁成了「使用」的学习成本。

## 六、最小语义层清单（单人 Profile 落地形态）

```text
身份层    KnowledgeRef / PinnedKnowledgeRef / FileRef + object_id↔path 映射（薄寻址）
写边界    COMMIT（= git commit + CAS ref） + APPEND（append 文件/SQLite）
读/检索   READ / LIST / LOG / DIFF / SEARCH（git + grep 原生，薄翻译）
来源      ORIGIN（显式 provenance 链）
结构      DESCRIBE_SCHEMA（读 schema 文件）
不变量    身份稳定、版本已知、来源保留、写者明确（K-04/K-05/K-06/K-12/K-20）
```
其余（PROPOSAL / Catalog 全 9 操作 / EXPAND_RELATIONS / WATCH_UPDATES / CAPABILITIES）→ 多人多 Repo 阶段再展开。

## 七、下一步建议

1. 把「最小语义层」作为 Phase 0 的**冻结范围**，多人能力按 D 档逐个解锁。
2. 先攻唯一两个「新发明」：RESOLVE（身份寻址）与 ORIGIN（来源链）的最小契约。
3. 其余按 G 档做「git 翻译」适配器，验证薄度标准。


# Minimal Core Contracts（RESOLVE 与 ORIGIN 最小契约）

最小语义层的两个「新发明」：RESOLVE（身份寻址）与 ORIGIN（来源链）。其余语义由 git+文件+grep 原生提供。本块是可进入 Phase 0 的冻结契约。

## A. RESOLVE（身份寻址）最小契约

### A.1 契约签名
```text
RESOLVE(refs[1..N], commit_id) → ResolutionSet

KnowledgeRef       = (repository_id, object_id)
PinnedKnowledgeRef = (repository_id, commit_id, object_id)
FileRef            = (repository_id, commit_id, path, digest?)

Resolution:
  repository_id, commit_id, object_id
  address: {kind: Entity|Aspect|Member|Relation|Record, object_id, aspect_name?, member_key?}
  path_hint
  digest
  schema_ref
  status: RESOLVED | REMOVED | UNRESOLVED | FORBIDDEN
```
- 普通 KnowledgeRef 在给定 commit 解析；Pinned 只验证目标仍可访问，不重新跟随上游。
- 对象在 commit 中不存在/已删除 → status=REMOVED；引用歧义 → UNRESOLVED；无权 → FORBIDDEN（外部呈现防旁路泄漏）。

### A.2 身份载体决策（化解「硬骨头 1」）
- **ObjectIdentity 的权威载体 = 文件内容里的稳定字段**（frontmatter `object_id` / `ref`），不是路径，也不是独立映射文件。
- `path_hint` 只是展示/便利位置，可移动。
- **address-map（object_id → path）是「可重建 Projection」**，非 Canonical，不进入 git 树，无需独立 CAS；由「扫描每个文件的 object_id 字段」生成，可丢弃重建。

这一决策直接化解第一性原理审查里的「硬骨头 1」（身份 vs 路径一致性）：
> 不存在需要额外一致性契约的「第四类状态」。身份内嵌于 git 树，随同一次 commit 与内容**原子演化**；address-map 降级为可重建 projection，权威永远在 git 树。

相比白皮书 §15.1 的 `.krepo/address-map.json`（独立映射、一致性契约未定义），本决策是对其的最小化修正。

### A.3 唯一性与移动
- repo 内 object_id 全局唯一；写入时校验，重复 → `OBJECT_ID_CONFLICT`。
- 移动文件 = 改 path_hint，object_id 不变；KnowledgeRef 不失效（K-04）。
- 约束：一个文件承载一个维护单元（Entity/Aspect/Member/Relation），避免「一文件多 object_id」的歧义。

### A.4 单人 Profile 落地
```text
index = scan(git tree, extract object_id field) → {object_id → path}   # 可重建
RESOLVE = 查 index → 读文件 → 校验 digest/schema → 返回 Resolution
```
无需 SQLite、无需独立 address-map；grep/文件读取即可。多人 Profile 再用 SQLite 物化该 index（仍是 Projection，非权威）。

## B. ORIGIN（来源链）最小契约

### B.1 契约签名
```text
ORIGIN(target_address, commit_id) → ProvenanceTrace

链（自近及远）:
  Current Value
  ← Repository / Commit / Object
  ← Commit / Append / Derivation Activity
  ← Source Record / Evidence / Artifact / Pinned Input
  ← Principal / System / Algorithm
```
跨 Repo 关系保留双方 KnowledgeRef；MVP 只返回可审计回链，不做完整 PROV 推理。

### B.2 最小 Provenance Envelope（字段冻结）
```yaml
provenance:
  origin_kind: SOURCE | OBSERVATION | EVIDENCE | ASSERTION | DEFINITION | DERIVATION | ...
  actor_ref:             # 谁
  activity_ref:          # 什么活动
  source_refs: []        # 来源
  evidence_refs: []      # 支撑证据
  input_view_read_version_ref:  # 仅 DERIVATION
  algorithm:             # 仅 DERIVATION
    derivation_spec_ref
    model_ref
    code_hash
  produced_at
```

### B.3 单人 Profile 落地
- **身份/版本**：git commit 的 author/message/hash —— git 原生，无需新造。
- **显式来源**：文件 frontmatter 的 `provenance` 块记录 source_refs/evidence_refs。
- **DERIVATION 强制**：必须显式 `input_view_read_version_ref` + `algorithm`；缺则来源链断，拒绝写入（呼应 K-08 / P2-1）。
- ORIGIN = 读 frontmatter provenance + git log + 沿 evidence_refs 回链。

### B.4 按知识族的最小义务
| 知识族 | 最小义务 |
|---|---|
| 定义/断言 | actor + repository + commit + evidence or rationale |
| 观察 | source + observed_at + record identity |
| 派生 | input version + derivation spec + activity |
| Artifact | content hash + media type + capture source |
| 关系 | source or derivation basis（若非纯结构引用） |

## C. 与既有决策的关系
- RESOLVE/ORIGIN 是 minimal-semantic-layer 里唯二的 F（Floor，可信强制）。
- RESOLVE 的身份载体决策是对 refinements-p0 P0-3（身份语法统一）的补充：不仅统一语法，还统一「身份的物理载体」。
- ORIGIN 与 refinements-p2 P2-1（授权）共用 actor/activity/evidence 字段形态，保持一致。
- 二者共同把「硬骨头 1」（身份 vs 路径一致性）从「延后」转为「已解」。

## D. 进入 Phase 0 的验收
1. 单人 Profile 下，只懂 git+文件+grep 的 Coding Agent 能完成：`RESOLVE kc://...` 定位到文件 → `READ` 读值 → `ORIGIN` 回链到 source → `COMMIT`（git commit）改值。
2. 移动文件后 RESOLVE 仍命中同一 object_id（Conformance T1 已验证语义）。
3. 删除对象后 RESOLVE 返回 REMOVED（非崩溃/非静默）。


# Ingestion & Grounding（低摩擦采集 与 grounding 消费）

两个「一序缺口」：底座再可信，(1) 没人往里放就死了；(2) 可信传不到用户眼前就没价值。本块以「不新增协议、只规范已有层」的方式补上，呼应最小语义层精神。

## A. 低摩擦采集（Ingestion）

第一性原理：repo-native 下，最自然的摄入单位是**文件/目录**，不是 Connector API。用户已经会 git、已经有文件，最低摩擦 = 把「一个目录/文件」声明为知识对象。

### A.1 两个薄工具（都在 COMMIT 之上，不是新 Surface）

```text
INGEST(dir, scope, schema_map) → 预览 + ChangeSet（确认后 COMMIT）
  扫描目录 → 识别维护单元（一个文件 = 一个 object）
  → 生成 object_id（稳定）、path_hint、schema_ref
  → provenance: {origin_kind: SOURCE, source_refs: [...], produced_at}
  → 产出 ChangeSet，预览后由调用方 COMMIT

RECONCILE(external_snapshot, scope, base_commit) → ChangeSet 预览
  外部快照 = 权威当前状态（结构化，如数据库全部表）
  → set-diff（新增→PUT+IF_ABSENT；更新→PUT+IF_OBJECT_EQUALS；删除→REMOVE）
  → 只产出 ChangeSet，不自动提交（确认后 COMMIT）
```

### A.2 关键约束
- INGEST / RECONCILE **都产出 COMMIT ChangeSet，不绕过 Ingress**（K-21）；它们属于 Application/Control Plane 在 Ingress 之上的编排，不是第四种 Surface。
- 二者**不判断内容正确性**（Ingress 语义薄）；只做「外部状态 → 目标地址 → PUT/REMOVE」的机械翻译。
- provenance 必须标注 source_refs（来源），否则 ORIGIN 链断。
- RECONCILE 的 diff 判定按「身份 + digest」：同 object_id 不同 digest = 更新；外部缺失且 repo 存在 = 删除（REMOVE，保留 Git 历史）。

### A.3 解决的问题
- 白皮书 §9.2 延后的「来源 Connector 协议、Scope Snapshot Reconciliation」→ 单人 Profile 下坍缩为 INGEST（目录扫描）+ RECONCILE（对账），多人/异构源再展开 Connector。
- 之前观察到的「缺少批量对账面」→ 用 RECONCILE 解决，但**不新增 Surface**，只是规范化「采集器如何构造 COMMIT」。

## B. grounding 消费路径（Grounding Citation）

第一性原理：用户感知价值 80% 在「AI 说 X，点击能看到：它用了哪个版本、哪段来源、为什么」。可信必须从 Access 一路传到 UI 不丢失。

### B.1 GroundingCitation（最小字段冻结）
```yaml
GroundingCitation:
  knowledge_ref        # 稳定身份
  pinned_ref           # 版本 + 对象（可复现，PinnedKnowledgeRef）
  digest
  fragment             # 可选，具体片段（段落/代码行）
  provenance_summary:  # 谁 + 什么活动 + 来源（来自 ORIGIN 摘要）
    actor_ref
    source_refs
    origin_kind
```

### B.2 硬不变量（可信传到眼前的判据）
> AI 的每个**事实性断言**，要么携带 PinnedKnowledgeRef（+ fragment + provenance），要么被**显式标记为「无依据 / 模型推断」**。二者不可混淆。

- 「无依据 / 推断」不是错误，但必须与「有依据」在 UI 上不可区分地分开呈现。
- 这等价于把「读/推断/提交」边界延伸到消费端：**模型不能把推断伪装成已引用的知识**。

### B.3 三层消费路径
| 层 | 职责 | 携带内容 |
|---|---|---|
| Access | 返回带来源的结果（K-12 已有） | repository/commit/object + provenance |
| Application | 组装上下文时，每个引用保留 GroundingCitation | 不丢 pinned_ref + provenance |
| UI | 引用可点击 → 展开「版本 + 来源 + 片段」 | 用户可复核 |

### B.4 与既有契约的关系
- GroundingCitation 复用 ORIGIN 的最小 provenance envelope（minimal-core-contracts B.2），是其「可展示摘要」。
- PinnedKnowledgeRef 是 citation 的版本锚（可复现）；普通 KnowledgeRef 只用于「当前视角」。
- 不新增 Access 操作；它是 READ_OBJECT / SEARCH / ORIGIN 结果的**约定投影**。

## C. 最小闭环验收（采集 → 消费）
1. 用户把目录/文件 INGEST 进来（标注 source_refs）→ 生成 Commit。
2. AI SEARCH/READ 命中 → 返回 PinnedKnowledgeRef + fragment + provenance。
3. UI 显示「该结论依据 X（版本 Y，来源 Z）」，点击可复核原文。
4. 模型没有依据的推断，被显式标为「推断」，不与引用混淆。

这条闭环是「真实用户可用」的一序命脉：它决定**底座是否被填满**（A）与**可信是否传到眼前**（B）。


# Gap Analysis（优化完善：一致性差异 + 设计缺口 + 精炼提案）

本块是「优化完善」主交付物。结论来自对两份源文档的逐节比对与契约闭环推演。

## A. 跨文档一致性差异（C1–C4）

| ID | 现象 | 位置 | 提案 |
|---|---|---|---|
| C1 | 版本标签不齐：推演标 v4.0，却自称「对应白皮书 v5.0」 | 推演标题/前言 | 推演版号独立于白皮书；标注「推演 v4.0 ↔ 白皮书 v5.0」映射，避免读者误读为同一版本线 |
| C2 | 不变量编号不一致：白皮书 K-01..K-23（23 条），推演 §0 只有 12 条，且无映射 | 白皮书 §2.2 vs 推演 §0 | 新增映射表（12 推演不变量 ↔ K 编号），或让推演直接引用 K 编号，消除两套口径 |
| C3 | 身份语法混用：白皮书示例里 `urn:knowledge:...`（§4.2、§12）与 `kc://...`（§14.1）并存，未声明等价 | 白皮书 §4.2/§12/§14 | 统一以 `kc://`（KnowledgeRef）与 `kr://`（Repository）为主；`urn:` 作为可映射别名并明示映射规则 |
| C4 | 术语 Projection 双关（字段投影 vs 可重建索引） | 白皮书 §20.2 | 已在内文澄清，无需改动；仅在本面记录，避免后续实现混淆 |

## B. 设计缺口与精炼提案（G1–G9）

### G1 授权模型欠形式化
- 现状：ACL（Repo 级）、WriteBinding（Ingress 级）、AuthorizationDecisionRef（Access 级）分散出现；K-20 说「锁数据不锁权限」，但无统一授权决策对象模型；AR-xxx 只出现为引用，未定义内容。
- 提案：新增最小「授权决策」契约：输入 {principal, policy_versions, resource, action}，输出 {decision, authorization_decision_ref, evidence}；明确 WriteBinding 是 Repo ACL 在 Ingress 的投影（不另设一套权限），Access 的 AR 是同一策略引擎的读取决策快照。
- MVP 取舍：只冻结字段契约，不实现策略引擎；策略引擎作为 Profile 组件。

### G2 ViewGeneration 解析确定性与幂等
- 现状：RESOLVE_VIEW 产生 generation_id，但未说明相同 (definition_revision, repo→commit) 是否得到同一 id，还是每次新建。
- 提案：规定 Resolve 是确定函数：`generation_id = H(definition_revision, {repo→commit} map)`；同输入同 id（幂等可重放、可缓存）；resolution_evidence 记录 resolver_version + selectors + registration_basis。
- MVP 取舍：内容寻址的 generation_id 直接获得审计与去重，成本为零。

### G3 保留与 GC 默认值
- 现状：「保留策略」「Grace Period」「孤儿 Artifact 回收」「Candidate TTL」只有引用，无默认值，实现方各自拍脑袋。
- 提案：在 MVP Profile 给出可覆盖默认值：孤儿 Artifact Grace Period=7d；已发布 Release/Generation 默认永久保留直至显式归档；Candidate Ref 未活动 TTL=30d 可归档（内容仍在 Git）；Append 保留按 Stream Policy。
- MVP 取舍：默认值写进 RepositoryProfile/StreamPolicy，Phase 1 落地。

### G4 隐私 Erasure 契约占位
- 现状：§11.2/§13.1 多次说「隐私 Erasure 是独立治理流程」，但无契约，属于隐性悬空承诺。
- 提案：设为显式开放问题，给占位契约 `ErasureRequest{target_ref, scope, reason, policy_ref}`；明确 Snapshot 不做无痕删除（保持 Git 不可变），Erasure 作用于 Append 记录/Artifact/Projection 副本；不进入 MVP 实现。
- MVP 取舍：只留契约骨架，不实现，避免被误当普通 REMOVE。

### G5 Derived invalidate 与 Head CAS 边界
- 现状：§11.3 有 `invalidate()`，§16.4 说「标记 stale/invalid 或 Head 被替换」，二者边界不清。
- 提案：区分 (a) Head CAS 前移（新 Revision 成为 Head，旧保留、可审计）与 (b) invalidation（显式标记某 Revision invalid + reason，不改历史；get_head 跳过 invalid，get_revision 仍可读）。
- MVP 取舍：invalidation 作为 DerivedRevision 可选状态字段，Phase 2 前不强制。

### G6 Repository 删除 vs 归档
- 现状：只有 ARCHIVE_REPOSITORY / RETIRE_DEFINITION，无 DELETE_REPOSITORY；对象级「不做无痕删除」与 Repo 级生命周期关系未说明。
- 提案：显式声明「不存在 DELETE_REPOSITORY」；Repo 生命周期终点是 ARCHIVE（禁写、保留可审计历史、Catalog 新 Resolve 默认不选入），物理删除是保留策略/合规的下游动作，不暴露为领域 API。
- MVP 取舍：一行不变量即可消除实现方自造 DELETE 的歧义。

### G7 Schema 自举（chicken-and-egg）
- 现状：Schema 存于 Repo（Snapshot Meta Knowledge），Ingress 校验依赖 Schema Contract；写时如何锁定 schema_ref 版本未定义。
- 提案：schema_ref 必须 pin 到已提交 Schema Revision；Ingress 校验用 Binding 声明的 allowed_schema_refs 白名单（预解析），Schema 变更与依赖它的写入共用 Precondition/CAS；不支持运行时动态解析未 pin 的 Schema。
- MVP 取舍：Binding 白名单最简，Phase 1 落地。

### G8 悬挂引用检测/通知契约
- 现状：REMOVE 有 replacement/reason 字段（好），但「谁负责发现并通知下游 about/from/to 引用断裂」未定义。
- 提案：悬挂引用检测归属 Control Plane（非 Repository 强制）：SEARCH/EXPAND 反查引用 → 生成 Proposal；Repository 只保证 REMOVE 的 replacement/reason 可审计。明确「引用断裂」在 Access 表现为 KNOWLEDGE_REF_UNRESOLVED（受控错误），不静默跳过。
- MVP 取舍：不进 Repository Core；Control Plane Phase 3 作为试点。

### G9 Producer Ordering 细节
- 现状：`MONOTONIC_PER_PARTITION` 未定义 partition key 与乱序拒绝；POSITION_REGRESSION 出现但无判定细节。
- 提案：partition key 来自 Binding 或命令（如 source_ref / 指定 partition）；单调性按 (partition, position) 判定，乱序/回退返回 POSITION_REGRESSION；NONE 时不做顺序保证。
- MVP 取舍：Phase 0 只实现 NONE，MONOTONIC 进 Phase 1。

## C. 优先级路线

| 优先级 | 项 | 性质 |
|---|---|---|
| P0（低成本高收益，文档一致性） | C1、C2、C3 | 消除口径分歧，直接改文档 |
| P1（MVP 契约补全，中成本） | G2、G3、G7、G9、G6 | Phase 0/1 契约字段，冻结语义 |
| P2（治理/合规，需更多设计） | G1、G4、G5、G8 | 契约骨架先行，实现延后 |

## D. 未决（需用户/团队决策）
- 优化最终形态：改源文档产出 v5.1？还是在本工作面维护提案集？还是起代码骨架（对应 Phase 0）？
- G1 授权是否要在 MVP 内形式化，还是显式标注为 Profile 实现细节。


# Refinements P0（文档一致性：C1 / C2 / C3）

解决 gap-analysis 的 C1–C3。本块内容可直接并入源文档（回写 v5.1）。

## P0-1（C1）版本标签对齐
- 决议：推演版号与白皮书版号相互独立，二者不联动。
- 推演前言改为：
  > **推演 v4.0 · 对应白皮书 v5.0（两条独立版本线）**
- 规则：白皮书 x.y 每次语义修订 +0.1；推演仅在案例/表述变化时 +0.1。读者不应把「推演 v4.0」读成「比白皮书 v5.0 旧」。

## P0-2（C2）不变量映射表
把推演 §0 的 12 条不变量映射到白皮书 K 编号，并列出推演未覆盖的 K。

| 推演 # | 措辞 | 映射 K | 备注 |
|---|---|---|---|
| 1 | 三 Repo 独立 ACL/Commit 图/Branch/Release/生命周期 | K-02 | K-01 的 target 约束见 #6 |
| 2 | KnowledgeRef 不含 Path | K-04 | — |
| 3 | Pinned +Commit；FileRef +Path/digest | K-04 | — |
| 4 | ViewDefinition 可写 Branch/Release；读前解析 VG | K-10, K-11 | K-09 见 #8 |
| 5 | 联合保留来源，不按 Scope 覆盖 | K-12, K-13 | — |
| 6 | View 不可写；一次 ChangeSet 一个 target Repo | K-01, K-22 | — |
| 7 | 本地分歧是 Assertion 非 Overlay | K-13 | — |
| 8 | ValidationReport 绑完整 PreviewGeneration | K-09 | — |
| 9 | 普通升级只新 Generation；Fork/Vendor 才有上游更新 | K-14, K-15, K-16 | — |
| 10 | VG 锁数据不冻结 ACL | K-20 | — |
| 11 | Snapshot 复用 Git；Append/Derived/Artifact 非 Git | K-05, K-17 | — |
| 12 | Commit / Projection Ready / Promote 三观察点 | K-19 | 呼应白皮书 §26.7 |

推演未覆盖的 K（应补为 #13–#19，或直接引用 K 编号）：
K-03（scope 非目录优先级）、K-06（expected-old/expected 前置条件）、K-07（Proposal 不改 main）、K-08（审批绑精确 Commit）、K-18（幂等键冲突）、K-21（不绕 Ingress）、K-23（Profile 迁移不变）。

决议：推演 §0 增补 7 条并显式标注 K 编号，消除两套口径。

## P0-3（C3）身份语法统一
规范语法：
```text
RepositoryIdentity  kr://<org>/<scope>/<name>
KnowledgeRef        kc://<repo-short>/<object-id>
PinnedKnowledgeRef  kc://<repo-short>@<commit>/<object-id>
FileRef             file://<repo-short>@<commit>/<path>#<digest>
```
urn 映射规则：
- `urn:knowledge:<object-id>` 等价于 `kc://<repo>/<object-id>`，仅在 RepositoryIdentity 由上下文确定时允许。
- 跨 Repo API 禁止裸 `urn:`（必须补齐 RepositoryIdentity，ADR-008）。
- 全库示例统一为 `kc://` 形式；`urn:` 只作「Repo 内紧凑 ObjectIdentity」别名。


# Refinements P1（MVP 契约冻结：G2 / G3 / G6 / G7 / G9）

解决 gap-analysis 的 G2、G3、G6、G7、G9。可进入 Phase 0/1 契约。

## P1-1（G2）ViewGeneration 确定性与幂等
冻结公式：
```text
generation_id = H( definition_revision ‖ sorted({ repo_id → commit_id }) )
```
- H 为内容寻址哈希（sha256）；sorted 使 Repo 顺序无关。
- 同 (definition_revision, map) → 同 generation_id（幂等、可重放、可缓存、可审计）。
- resolution_evidence 必须记录 {resolver_version, selectors, registration_basis}。
- 例外：Definition 含 Optional Source Policy 产生 Degradation 时，Generation 必须显式带 degradation 标记，不得静默。

## P1-2（G3）保留与 GC 默认值（可被 Profile/Policy 覆盖）
| 对象 | 默认 | 覆盖点 |
|---|---|---|
| 孤儿 Artifact | 7d 未引用回收 | RepositoryProfile |
| 已发布 Release / Generation | 永久，直至显式归档/退役 | Catalog Policy |
| Candidate Ref | 30d 未活动可归档（Commit 保留在 Git） | RepositoryProfile |
| Append Stream | 按 StreamPolicy（默认永久） | StreamPolicy |
| Receipt / Audit | 与幂等保留期一致（默认 P30D） | Binding.idempotency_retention |

## P1-3（G6）无 DELETE_REPOSITORY
新不变量（追加 K-24）：
> 不存在 DELETE_REPOSITORY 领域操作；Repository 生命周期终点是 ARCHIVE_REPOSITORY（禁写、保留可审计历史、Catalog 新 Resolve 默认不选入）。物理删除是保留策略/合规的下游动作，不暴露为领域 API。

## P1-4（G7）Schema 自举
- schema_ref 必须 pin 到已提交 Schema Revision（`urn:schema:...:vN` 或 `kc://<repo>/schema/...@<commit>`）。
- Ingress 校验用 WriteBinding.allowed_schema_refs 白名单（Binding 协商时预解析并冻结）。
- Schema 变更走普通 COMMIT/PROPOSAL，与依赖它的写入共用 Ref CAS / 前置条件；不支持运行时解析未 pin 的 Schema。
- 新增错误码：`SCHEMA_REVISION_UNRESOLVED`（白名单内但版本缺失）。

## P1-5（G9）Producer Ordering
- partition key = Binding 声明或命令字段（默认 source_ref）；单调性按 (partition, position) 判定。
- 乱序/回退 → POSITION_REGRESSION（NON_RETRYABLE）。
- ordering_profile=NONE 时不保证顺序、不做回归检查。
- 实现顺序：Phase 0 只 NONE；MONOTONIC_PER_PARTITION 进 Phase 1。


# Refinements P2（治理/合规契约骨架：G1 / G4 / G5 / G8）

解决 gap-analysis 的 G1、G4、G5、G8。契约骨架先行，实现延后。

## P2-1（G1）授权决策契约
```yaml
AuthorizationDecision:
  principal_ref: ...
  policy_versions: {repo_id: version}
  resource: KnowledgeRef | Address
  action: READ | WRITE | RESOLVE
  decision: ALLOW | DENY
  authorization_decision_ref: AR-xxx
  evidence: {...}   # 命中策略/规则引用
```
- WriteBinding = Repo ACL 在 Ingress 的投影（同一策略引擎，非第二套权限）。
- Access 的 AR = 读取决策快照（可审计，K-20）。
- MVP：只冻结字段契约，不实现策略引擎；策略引擎作为 Profile 组件。

## P2-2（G4）隐私 Erasure 契约占位
```yaml
ErasureRequest:
  target_ref        # Append Record / Artifact / Projection 副本
  scope
  reason
  policy_ref
```
- Snapshot 不做无痕删除（Git 不可变，K-05）。
- Erasure 作用于 Append 记录 / Artifact / Projection 副本；保留逻辑审计语义。
- MVP：只留骨架，不实现；实现前需独立 ADR。

## P2-3（G5）Derived invalidation 语义
- Head CAS 前移：新 Revision 成为 Head，旧 Revision 保留可审计。
- Invalidation：显式标记某 Revision `status=invalid` + reason，不改历史；get_head 跳过 invalid，get_revision 仍可读。
- DerivedRevision.status ∈ {valid, stale, invalid}：
  - valid = 当前算法 + 输入下有效
  - stale = 输入或算法已变但未重算
  - invalid = 显式撤回
- MVP：status 字段可选，Phase 2 前不强制。

## P2-4（G8）悬挂引用检测契约
- 归属：Control Plane（非 Repository 强制）。
- Repository 义务：REMOVE 必须可审计 replacement/reason（已有）。
- Access 表现：引用断裂 → KNOWLEDGE_REF_UNRESOLVED（受控错误），不静默跳过。
- Control Plane 流程：SEARCH/EXPAND 反查 about/from/to → 生成目标 Repo Proposal。
- MVP：不进 Repository Core；Phase 3 作为试点。

