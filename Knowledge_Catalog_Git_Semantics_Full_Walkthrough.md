---
title: "Knowledge Catalog 完整系统全流程推演"
subtitle: "Catalog、Ingress、Repository、Access 与维护控制平面的端到端状态演化"
author: "Architecture Walkthrough"
date: "2026-08-16"
version: "v4.0"
lang: "zh-CN"
---

# Knowledge Catalog 完整系统全流程推演

**版本：v4.0 · 对应白皮书 v5.0**

> 本文用一个连续案例推演完整系统，而不是只解释多 Repo 组合：从 Repository 创建和 Catalog 注册开始，依次覆盖 Artifact、Ingress 三种 Write Surface、Git Branch/Merge/Release、十二个 Access 操作、Candidate/Preview 测试、View Promotion、观察与派生、回滚、Fork/Vendor、归档和失败恢复。

---

# 0. 案例与不变量

| Repository | Scope / Owner | 内容 |
|---|---|---|
| `kr://acme/public/core` | PUBLIC / Core Council | Service Schema、Policy P-103、Team |
| `kr://acme/groups/payments` | GROUP / Payments | Payment Service、运行断言、Runbook |
| `kr://acme/personals/alice` | PERSONAL / Alice | 私有笔记、提醒、显示偏好 |

Catalog：`kc://acme/knowledge`。

贯穿全文的不变量：

1. 三个 Repository 有独立 ACL、Commit 图、Branch、Release 和生命周期。
2. `KnowledgeRef = repository_id + knowledge_object_id`，不含 Path。
3. `PinnedKnowledgeRef` 再增加 Commit；`FileRef` 再增加 Path 与 digest。
4. `ViewDefinition` 可以写 Branch/Release；读取前解析为 `ViewGeneration = repo→commit`。
5. 联合结果保留 Repository、Commit、Object、Scope 和 Provenance，不按 Scope 覆盖。
6. View 不可写；一次 ChangeSet 只修改一个 target Repository。
7. group/personal 的不同意见是自己的 Assertion，不是 public Overlay。
8. ValidationReport 绑定完整 PreviewGeneration。
9. 普通 public 升级只产生新 Generation；只有 Fork/Vendor 同步才有显式上游更新语义。
10. ViewGeneration 锁定数据 Commit，不冻结 ACL；Access 每次读取重新授权。
11. Snapshot 复用 Git；Append、Derived、Artifact 保留各自的非 Git 演化语义。
12. Repository Commit、Projection Ready 与 Catalog Promote 是三个独立观察点。

```text
P*    public Commit          G*    group Commit
U*    personal Commit        C*    group Candidate Commit
PC*   public Candidate       VG*   stable ViewGeneration
PVG*  PreviewGeneration      R*    immutable Release
O-*   immutable Object ID      S*    Append Cursor
VD*   ViewDefinition Revision  AR*   Authorization Decision
```

---

# 1. 创建三个独立 Repository，并登记到 Catalog

## 1.1 `CREATE_REPOSITORY`

Core Council、Payments 与 Alice 分别创建 Repo：

```yaml
operation: CREATE_REPOSITORY
repository_id: kr://acme/public/core
scope_kind: PUBLIC
owner_ref: group://core-council
default_ref: refs/heads/main
profile: portable-file-git/v1
acl_policy_ref: policy://public/core/v1
```

group 与 personal 使用同一操作，分别设置 `GROUP/group://payments` 与 `PERSONAL/principal://alice`。结果是三个独立初始 Commit `P0/G0/U0`，以及互不共享的 Object Store、Commit Graph、Ref Namespace、ACL、Release 与保留策略。

创建 Repo 不会自动产生联合 View。若三个 Scope 只是同一 Repo 的三个目录，就无法独立 Branch、Release、Clone 和控制历史可见性，因此本例不采用目录模型。

## 1.2 `REGISTER_REPOSITORY`

Catalog 已作为系统服务存在，只登记 Repository 的身份与连接元数据：

```yaml
operation: REGISTER_REPOSITORY
repository_id: kr://acme/public/core
endpoint_ref: repo-endpoint://git/public-core
owner_ref: group://core-council
visibility_metadata: PUBLIC
capability_digest: sha256:cap-public-v1
expected: ABSENT
```

group 与 personal 同样登记，得到 Registration Revision `RR3`。Registration 不复制内容、不修改 P0/G0/U0，也不授予调用者额外 Repo 权限。以后端点或 Capability 变化时使用 `UPDATE_REGISTRATION(expected_revision=RR3)`；不能借此改变 Repo ACL 或 Git 历史。

Alice 若无权发现某个 Repo，Catalog 返回防旁路泄漏的受控错误；不能通过 Registration Count 推断隐藏来源。

---

# 2. Public 创建、修改与发布权威知识

## 2.1 Repository `PutArtifact`

```yaml
operation: PutArtifact
repository_id: kr://acme/public/core
media_type: application/pdf
expected_digest: sha256:handbook-v1
bytes: <refund-handbook.pdf>
```

返回 `artifact:sha256:handbook-v1`。Artifact 上传不是第四个 Ingress Surface；在 Snapshot/Append/Derived 引用它之前，只是不可变内容，不表示知识已经发布。

## 2.2 Ingress `COMMIT`

```yaml
surface: COMMIT
binding_ref: binding://public/core/editor
command_id: cmd-public-bootstrap-1
repository_id: kr://acme/public/core
target_ref: refs/heads/main
base_commit: P0
expected_target_commit: P0
message: "publish service schema v2 and policy P-103"
operations:
  - op: PUT
    address: {kind: EntityAddress, object_id: schema/service-v2}
    value: {contract: {...}}
    precondition: IF_ABSENT
  - op: PUT
    address: {kind: EntityAddress, object_id: policy/P-103}
    value:
      statement: "production services require an owned runbook"
      handbook_ref: artifact:sha256:handbook-v1
      path_hint: policies/P-103.yaml
    precondition: IF_ABSENT
  - op: PUT
    address: {kind: EntityAddress, object_id: team/payments}
    value: {display_name: "Payments", path_hint: teams/payments.yaml}
    precondition: IF_ABSENT
```

Ingress 校验 Binding/Schema/Scope/幂等后，Repository 创建 Objects/Tree/Commit `P1(parent=P0)` 并把 main 从 P0 CAS 到 P1。Durable Receipt 返回 P1；三种引用为：

```text
KnowledgeRef:       kc://public/core/policy/P-103
PinnedKnowledgeRef: kc://public/core@P1/policy/P-103
FileRef:            file://public/core@P1/policies/P-103.yaml#sha256:...
```

## 2.3 修改内容与移动文件

修改 Policy 时保留 KnowledgeRef，Object ID 改变：

```yaml
op: PUT
address: {kind: EntityAddress, object_id: policy/P-103}
value: {statement: "production services require a tested, owned runbook", ...}
precondition: {type: IF_OBJECT_EQUALS, object_id: O-P103-1}
```

得到 P2。Repo-native 编辑器之后只调整 `path_hint`，把文件移动到 `policies/production/P-103.yaml`，生成 P3；ObjectIdentity 保持，内容 Object 若未变化可复用，只有 Tree Path 改变。普通 KnowledgeRef 仍可解析；旧 FileRef 仍精确指向 P1 的旧 Path。

## 2.4 Repository `PUBLISH_RELEASE`

```yaml
operation: PUBLISH_RELEASE
repository_id: kr://acme/public/core
release_id: release/2026.08-r17
commit_id: P3
manifest_digest: sha256:manifest-r17
publisher: group:core-council
```

得到不可变 R17。Release 名不能复用，也不能改指；发布 Repo Release 不会自动创建或 Promote Catalog ViewGeneration。

---

# 3. Group 引用 public，并维护自己的信息

## 3.1 Service、Relation 与局部 Assertion

```yaml
surface: COMMIT
binding_ref: binding://group/payments/editor
command_id: cmd-group-bootstrap-1
repository_id: kr://acme/groups/payments
target_ref: refs/heads/main
base_commit: G0
expected_target_commit: G0
dependency_basis:
  kr://acme/public/core: P3
operations:
  - op: PUT
    address: {kind: EntityAddress, object_id: service/payment}
    value:
      schema_ref: kc://public/core/schema/service-v2
      display_name: Payment Service
  - op: PUT
    address: {kind: RelationAddress, object_id: relation/payments-owns-service}
    value:
      from: kc://public/core/team/payments
      predicate: owns
      to: kc://groups/payments/service/payment
  - op: PUT
    address: {kind: EntityAddress, object_id: assertion/A-27}
    value:
      about: kc://public/core/policy/P-103
      relation: qualifies
      claim: {applies_within: payments-production}
```

得到 G1。它只改变 group Repo；P3 未变化。KnowledgeRef 不绑定 P3，但 G1 metadata 记录 P3 是提交时的验证 basis。

## 3.2 Runbook 与 `APPEND`

先调用 Repository `PutArtifact` 得到 `artifact:sha256:runbook-1`，再通过 COMMIT 提交 Artifact Manifest 与 Service 引用，得到 G2。运行证据使用 APPEND Surface：

```yaml
surface: APPEND
binding_ref: binding://group/payments/evidence-writer
command_id: append-evidence-9001
repository_id: kr://acme/groups/payments
stream_ref: evidence
expected_cursor: S0
entries:
  - event_id: evt-9001
    schema: runbook-test/v1
    payload: {service: kc://groups/payments/service/payment, outcome: PASSED}
```

得到 S1。重复相同 event_id/payload 幂等；同 ID 不同 payload 返回 `EVENT_ID_CONFLICT`。

---

# 4. Personal 维护自己的信息

Alice 在 U0 上创建私有 Note 与显示偏好：

```yaml
surface: COMMIT
binding_ref: binding://personal/alice/editor
command_id: cmd-alice-note-1
repository_id: kr://acme/personals/alice
target_ref: refs/heads/main
base_commit: U0
expected_target_commit: U0
dependency_basis: {kr://acme/groups/payments: G2}
operations:
  - op: PUT
    address: {kind: EntityAddress, object_id: note/payment-oncall}
    value:
      about: kc://groups/payments/service/payment
      text: "切换流量前先检查冻结窗口"
  - op: PUT
    address: {kind: EntityAddress, object_id: preference/payment-service}
    value:
      target: kc://groups/payments/service/payment
      display_name: "我的支付值班服务"
      highlight: true
      presentation_only: true
```

得到 U1。Preference 只能用于 Alice 的 UI；Canonical 查询仍返回 group 的正式名称。它不能改变 Assertion、Relation、Policy 或 Search 真值。

---

# 5. ViewDefinition、ViewGeneration 与授权投影

## 5.1 `DEFINE_VIEW` 与 `RESOLVE_VIEW`

```yaml
operation: DEFINE_VIEW
view_id: alice-default
expected_definition_revision: ABSENT
sources:
  - repository_id: kr://acme/public/core
    selector: {release: releases/2026.08-r17}
  - repository_id: kr://acme/groups/payments
    selector: {branch: refs/heads/main}
  - repository_id: kr://acme/personals/alice
    selector: {branch: refs/heads/main}
```

得到 `ViewDefinition Revision VD1`。Definition 是意图，不是稳定读取坐标；Catalog 执行 `RESOLVE_VIEW(VD1, principal=Alice)`，一次解析得到：

```yaml
view_generation: VG1
definition_revision: VD1
registration_basis: RR3
definition_digest: sha256:alice-default-v1
repository_commits:
  kr://acme/public/core: P3
  kr://acme/groups/payments: G2
  kr://acme/personals/alice: U1
resolution_evidence: {selectors: {...}, resolver_version: view-resolver/v4}
```

随后 `VALIDATE_GENERATION(VG1)` 通过，`PROMOTE_GENERATION(channel=alice/stable, expected=null, new=VG1)` 以 CAS 发布。Promote 只移动 Catalog Channel，不修改 P3/G2/U1。同一 RepositoryIdentity 若出现两次，返回 `VIEW_GENERATION_INVALID`。

## 5.2 Authorized Projection

Alice 的读取记录：

```yaml
principal_id: principal:alice
view_generation: VG1
authorization_policy_versions: {public: AP1, group: AG7, personal: AU3}
authorization_decision_ref: AR-901
```

Bob 使用同一 VG1，但无 personal 权限，因此 Authorized Projection 不含 U1。VG1 不因用户或 ACL 不同而变化；Generation 锁数据但不冻结 ACL。审计记录 `AR-901` 与 Policy versions；权限撤销后，相同 VG1 也不能继续泄漏已失权对象。

---

# 6. 查询：覆盖全部 Access 操作

| 操作 | 本例中的请求与结果 |
|---|---|
| `CAPABILITIES` | 返回 Profile、Search modes、Consistency、limit 与候选索引能力 |
| `DESCRIBE_SCHEMA` | 在 VG1 解析 service-v2，返回 public@P3、Schema Object 与 Lifecycle Contract |
| `DESCRIBE_INDEX` | 返回 `basis_view_generation=VG1`、AR-901、channels、projection lag 与 coverage |
| `RESOLVE` | P-103 + VG1 → public@P3、当前 Path 与 Object ID；Branch 只解析一次 |
| `READ_OBJECT` | 按 VG1+KnowledgeRef 或 Commit+Object ID 读，返回完整来源与授权证据 |
| `LIST_TREE` | 对 group@G2 完整枚举；分页 token 固定 Tree/filter digest |
| `LOG` | P-103 历史显示 P1 ADD、P2 REPLACE、P3 MOVE |
| `DIFF` | P1↔P3 区分内容修改与 Path 移动；VG1↔VG2 区分成员 Commit 变化 |
| `ORIGIN` | P-103.statement → P2、Action、principal、evidence；必要时降级为 Object 级 |
| `SEARCH` | 在 VG1/AR-901 上运行 EXACT_REF/PATH/STRUCTURED/LEXICAL/REGEX/SEMANTIC/HYBRID |
| `EXPAND_RELATIONS` | group Service 反向展开 owns，返回 public Team 与 Canonical Relation 来源 |
| `WATCH_UPDATES` | 通知 Ref/Stream/View/Catalog/Repository/Projection 前移；事实再用 Diff/Log/Read 获取 |

Search 的关键规则：

| Mode | 零结果语义 |
|---|---|
| EXACT_REF / PATH | 完整 Tree 上可确定 NOT_FOUND |
| STRUCTURED / LEXICAL / REGEX | 仅覆盖完整且 basis 匹配时可证明范围内无结果 |
| SEMANTIC / HYBRID | 只能说当前近似检索未发现 |

每个 Hit 保留 Repository/Commit/KnowledgeRef/Scope/Provenance。检索 P-103 可以同时命中 public P-103 与 group A-27；Catalog 不把 A-27 覆盖到 P-103 上。Watch 至少一次投递并可重复，Cursor 过期时从当前精确版本重建基线。

---

# 7. Group Proposal、Candidate Branch 与直接访问

## 7.1 Ingress `PROPOSAL` 创建 Candidate

```yaml
surface: PROPOSAL
binding_ref: binding://group/payments/proposer
command_id: proposal-42-create
repository_id: kr://acme/groups/payments
target_ref: refs/heads/main
candidate_ref: refs/heads/candidates/PR-42
base_commit: G2
rationale: 补充退款超时处置步骤
operations:
  - op: PUT
    address: {kind: EntityAddress, object_id: procedure/refund-timeout}
    value: {...}
```

Repository 创建 C1；若 CandidateRef 不存在则执行受保护的 `CREATE_REF`，后续修订均以 `UPDATE_REF(expected_old,new_commit)` CAS 前移。Proposal Metadata 记录 target/candidate/base/rationale。随后通过同一 Candidate Ref 再提交一次修订得到 C2：

```text
G2 <- C1 <- C2
main -> G2
candidate/PR-42 -> C2
```

每次写都指定 target Repository/Ref/base/expected target。Proposal Receipt 不移动 main，也不改变 VG1。向 View 直接写返回 `WRITE_TARGET_REQUIRED`。

## 7.2 直接访问 Branch

```yaml
operation: RESOLVE
repository_id: kr://acme/groups/payments
ref: refs/heads/candidates/PR-42
```

一次解析为 C2。`READ_OBJECT`、`LIST_TREE`、`LOG`、`DIFF`、`ORIGIN`、`SEARCH`、`EXPAND_RELATIONS` 都能访问 C2；分页和测试中不得再次解析 Branch。Branch 是协作指针，不是可复现测试坐标。

---

# 8. Candidate/Test View：完整 PreviewGeneration

## 8.1 生成 PVG1 并测试

Catalog 执行 `CREATE_PREVIEW`，以 VG1 为基线，只把 group G2 替换为 C2：

```yaml
preview_generation: PVG1
base_generation: VG1
repository_commits:
  kr://acme/public/core: P3
  kr://acme/groups/payments: C2
  kr://acme/personals/alice: U1
candidate: {repository_id: kr://acme/groups/payments, commit_id: C2}
```

控制平面通过 Access 在 PVG1 上执行 `DESCRIBE_INDEX / RESOLVE / READ_OBJECT / SEARCH / EXPAND_RELATIONS / DIFF / ORIGIN`。测试发现缺字段，保存 `VAL-1(candidate=C2,target=G2,preview=PVG1,suite=S7,outcome=FAILED)`。VAL-1 保留审计，但不能作为 Merge Gate 依据。

Candidate 修复得到 C3；旧 PVG1/VAL-1 不证明 C3。Catalog 创建 PVG2，测试通过，保存 `VAL-2(PVG2,PASSED)`。

## 8.2 Review、MergeGate 与稳定 View

Approval 明确批准 C3。Gate 验证 Candidate 仍为 C3、main 仍为 G2、VAL-2 绑定完整 PVG2、suite/Policy/Stream Cut 合格，然后调用 Repository `MERGE`：

```yaml
operation: MERGE
repository_id: kr://acme/groups/payments
target_ref: refs/heads/main
candidate_commit: C3
expected_target_commit: G2
strategy: FAST_FORWARD_OR_MERGE
validation_report: VAL-2
```

若 main/Candidate 前移则失败。若并发写先把 main 从 G2 移到 G2b，维护者必须执行：

```yaml
operation: REBASE_CANDIDATE
repository_id: kr://acme/groups/payments
candidate_ref: refs/heads/candidates/PR-42
expected_candidate_commit: C3
old_base: G2
new_base: G2b
```

它生成新 Candidate C4；C3、PVG2、VAL-2 和旧 Approval 均不能证明 C4。必须以 C4 创建 PVG3 重测。正常路径若 main 仍为 G2，则 MERGE 可 Fast-forward；若 Merge 生成新 Commit，同样必须按 Gate Policy 判断是否重建 Preview。合并后可以 CAS 删除 CandidateRef，Proposal/Validation/Approval 审计对象保留。

原 VG1 仍固定 G2。Catalog `RESOLVE_VIEW(VD1)` 生成 `VG2={public:P3,group:C3,personal:U1}`，`VALIDATE_GENERATION` 后把 alice/stable 从 VG1 CAS Promote 到 VG2。旧查询仍可用 VG1 重放；Repository Merge 和 Catalog Promote 是两个动作。

---

# 9. Public 未合并 Branch 的跨 Repo 测试

Core Council 通过 public Repo 的 PROPOSAL Binding 从 P3 创建 CandidateRef 并提交 PC1。Alice 无权 COMMIT public；只有 ACL 明确允许时才可读取该候选。Catalog 以 VG2 为基线只替换 public：

```yaml
preview_generation: PVG-PUBLIC-1
repository_commits:
  kr://acme/public/core: PC1
  kr://acme/groups/payments: C3
  kr://acme/personals/alice: U1
candidate: {repository_id: kr://acme/public/core, commit_id: PC1}
```

这就是“已提交、未合并、需要测试”的 Branch View。报告绑定完整 PVG-PUBLIC-1，而不只绑定 PC1。

如果 public main fast-forward 到 PC1，且其他成员、Policy、suite 均未变，Gate Policy 可以继承同一完整 Map 的报告。若 rebase/merge 产生 P4，则重新生成 PreviewGeneration 并测试。完成后 Repository 单独 `PUBLISH_RELEASE(P4)`；Catalog 再解析和 Promote，不由 MERGE 自动连带执行。

---

# 10. Public 正式升级：新 Generation，不做跨 Repo merge

public main 到 P4 并创建 R18。ViewDefinition 改用 R18，Resolver 构造：

```yaml
candidate_generation: VG3-CANDIDATE
repository_commits:
  kr://acme/public/core: P4
  kr://acme/groups/payments: C3
  kr://acme/personals/alice: U1
```

Resolver 重新解析 group Service 的 SchemaRef、Relation 的 public Team、A-27 的 `about:P-103` 和 personal Note 的 group 引用；检查缺失、Removed、Schema 与约束。P-103 移动 Path 不影响 KnowledgeRef；对象删除或无权时返回受控错误。

验证绑定完整 VG3-CANDIDATE；通过后 Promote 为 VG3。group 仍是 C3，personal 仍是 U1：没有跨 Repo Git merge，也没有改写引用文件。A-27 与新版 P-103 继续并存。

`WATCH_UPDATES` 依次观察到 public RefUpdate、R18 Published、VG3 Created/Promoted 和 Projection Ready。消费者按 Event ID/Cursor 幂等处理，再调用 `DESCRIBE_INDEX(VG3)` 与 `DIFF(VG2,VG3)` 获取确定事实；Watch Event 本身不携带完整知识 Payload，也不替代读取。

---

# 11. Fork 与 Vendor：两种维护流程

## 11.1 Fork

Payments 要独立演化 P-103，创建：

```yaml
knowledge_ref: kc://groups/payments/policy/GP-9
was_derived_from: kc://public/core@P4/policy/P-103
fork_base: {repository_id: kr://acme/public/core, commit_id: P4, object_id: O-P103-4}
```

得到 group G4。public 到 P6 后，只有主动同步 Fork 才比较：

```text
BASE     = public@P4/P-103
UPSTREAM = public@P6/P-103
LOCAL    = group@G4/GP-9
```

同字段双改返回 `FORK_SYNC_CONFLICT`；维护者在 group Candidate 解决、测试、合并。这是跨权威三方内容同步语义。

## 11.2 Vendor

Payments 为离线校验 Vendor 精确 Contract：

```yaml
source: file://public/core@P4/contracts/service-v3.json#sha256:abc
source_object: kc://public/core@P4/schema/service-v3
local_copy_digest: sha256:def
mode: VENDOR_READ_ONLY
```

刷新到 P6 时验证新 digest 并替换 pin/copy。若发现本地语义编辑，刷新失败：必须丢弃编辑或显式转为 Fork，不能保留 public 身份却声明本地权威。

---

# 12. Append 修正、Derived 维护与重放

## 12.1 Stream Correction / Retraction

对 S1 追加观测 `evt-9002` 得 S2。发现错误时不覆盖旧 Record，而追加：

```yaml
record_id: evt-9002-correction
type: CORRECTION
corrects: evt-9002
payload: {...}
```

得到 S3；撤回再追加 RETRACTION 得 S4。带 Stream 的请求使用：

```yaml
view_read_version:
  view_generation: VG3
  stream_cuts: {group/evidence: S4}
```

仅有 Git 快照时直接使用 VG3；旧 `VG2+S1` 仍可按保留策略重放。

## 12.2 固定输入上的 Derived Revision

评估器读取固定 `ViewReadVersion(VG3 + group/evidence@S4)`，计算退款 Runbook 健康度；它不能直接把推断写成 public Accepted Policy，而是使用受限 COMMIT Binding 写入 group Derived Collection：

```yaml
surface: COMMIT
binding_ref: binding://group/payments/derived-health-writer
repository_id: kr://acme/groups/payments
address: {kind: DerivedOutputAddress, object_id: health/refund-runbook}
input_view_read_version: {view_generation: VG3, stream_cuts: {group/evidence: S4}}
derivation_spec: derivation://runbook-health/v2
algorithm: {code_hash: sha256:health-code-v2}
value: {status: HEALTHY, score: 0.93}
expected_derived_head: D0
```

Repository 创建不可变 Derived Revision D1，并 CAS 更新 Derived Head；旧 D0 保留。以后 VG、Stream Cut 或算法变化时产生 D2，而不是覆盖 D1。`ORIGIN(D1)` 可以回到输入 Generation、S4、算法和 Activity；Projection 只索引 D1，不成为其权威来源。

---

# 13. 删除对象、移除来源与归档

## 13.1 `REMOVE` 保留 Git 历史

```yaml
surface: COMMIT
repository_id: kr://acme/groups/payments
target_ref: refs/heads/main
op: REMOVE
address: {kind: RelationAddress, object_id: relation/payments-owns-service}
precondition: {type: IF_OBJECT_EQUALS, object_id: O-REL-1}
reason: replaced-by-owner-claim-v2
replacement: kc://groups/payments/relation/payments-owns-service-v2
```

得到 G5。对象在 G5 Tree 中不再可见，但 Object、Commit、删除理由和替代引用仍在 Git/Provenance 历史中；普通 `KnowledgeRef` 在新 Generation 返回 `REMOVED`，PinnedKnowledgeRef 仍可按权限读取旧版本。不做无痕删除；隐私 Erasure 是另一套治理流程。

## 13.2 更新 View、归档 Repo 与退役 Definition

组织共享 View 不应包含 personal 时，创建新的 ViewDefinition Revision VD2，删除 personal Source，解析为不含 U1 的 VG4，验证后 Promote；personal Repo 和旧 VG3 都未被修改。

```yaml
operation: DEFINE_VIEW
view_id: org-shared
expected_definition_revision: VD1-shared
sources:
  - kr://acme/public/core@release/2026.08-r18
  - kr://acme/groups/payments@refs/heads/main
```

Definition 更新不等于 Repo 归档，也不破坏旧 Definition/Generation。

```yaml
operation: ARCHIVE_REPOSITORY
repository_id: kr://acme/personals/alice
reason: migrated
retention_deadline: 2027-08-16
```

普通写入被拒绝；精确历史读取按当前 ACL 与保留 Policy 决定。Catalog 更新 Registration 状态，使新 Resolve 默认不再选入该 Repo。完成 View 迁移和依赖检查后，可执行 `RETIRE_DEFINITION(org-shared)` 禁止新解析；它不遍历删除任何 Repository、Generation 或审计记录。

---

# 14. Promotion 回滚、Repository Revert 与灾难恢复

假设 VG3 发布后发现 Access 回归。最快恢复路径只操作 Catalog：

```yaml
operation: ROLLBACK_PROMOTION
channel: alice/stable
expected_current_generation: VG3
prior_generation: VG2
reason: search-regression
```

Channel CAS 回 VG2；P4/C3/U1 和所有 Git 历史不变。若问题是 Projection，可重建以 VG3 为 basis 的索引后再 Promote，不应修改 Canonical Knowledge。

若 public P4 的内容本身错误，Core Council 在 public Repo 创建反向 Commit：

```yaml
operation: REVERT
repository_id: kr://acme/public/core
target_ref: refs/heads/main
expected_head: P4
revert_commit: P4
reason: policy-regression
```

得到 P5；不删除 P4 或 R18。随后仍要 `PUBLISH_RELEASE(P5) → RESOLVE_VIEW → VALIDATE_GENERATION → PROMOTE_GENERATION`。Catalog Rollback 与 Repository Revert 可以独立使用：前者恢复 Serving 组合，后者修正权威历史。

备份恢复必须同时验证 Repository Object/Commit/Ref、Catalog Definition/Generation/Promotion、Append Cursor、Derived Head、Artifact Digest 和 Receipt/Audit；仅恢复 Git 目录并不能重建完整 `ViewReadVersion`。

---

# 15. 并发、失败与恢复

| 失败 | 场景 | 恢复 |
|---|---|---|
| `PRECONDITION_FAILED` | expected Object/Commit 不匹配 | READ_OBJECT/DIFF 后重建 ChangeSet |
| `IDEMPOTENCY_CONFLICT` | 相同 command_id 对应不同请求 | 换新 command_id 提交修正后的逻辑请求 |
| `NON_FAST_FORWARD` | target Ref 前移 | LOG/DIFF，显式 rebase 或新 Candidate |
| `CANDIDATE_MOVED` | 测试后 Branch 前移 | 解析新 PreviewGeneration 并重测 |
| `VALIDATION_BASIS_MISMATCH` | 报告只匹配旧 PVG | 对当前完整 PVG 重测 |
| `KNOWLEDGE_REF_UNRESOLVED` | 对象缺失/Removed/无权 | 改成员、迁移引用或接受 Removed 状态 |
| `WRITE_TARGET_REQUIRED` | 向 View 写且未指定 Repo | 选择唯一 target_repository |
| `FORK_SYNC_CONFLICT` | Fork 与上游同处双改 | 在 Fork 所属 Repo 人工三方解决 |
| `INDEX_BEHIND` | Projection 未追到 Generation | 等待、Canonical 降级或明确失败 |
| `AUTHORIZATION_CHANGED` | 相同 Generation 下 ACL 已撤销 | 重新授权；不得使用旧结果缓存泄漏对象 |
| `PROMOTION_CAS_FAILED` | Channel 已被其他 Generation 前移 | DESCRIBE_INDEX 后决定保留、重验或重新 Promote |

任何 Ref CAS 失败都不能报告成功。已写入但不可达的 Object/Commit 按保留策略回收；审计说明是否移动 Ref。

---

# 16. 全操作覆盖矩阵

## 16.1 Catalog 写操作

| 操作 | 推演位置 |
|---|---|
| `REGISTER_REPOSITORY` | §1.2 |
| `UPDATE_REGISTRATION` | §1.2、§13.2 |
| `DEFINE_VIEW` | §5.1、§10、§13.2 |
| `RESOLVE_VIEW` | §5.1、§8、§10、§14 |
| `CREATE_PREVIEW` | §8、§9 |
| `VALIDATE_GENERATION` | §5、§8、§10、§14 |
| `PROMOTE_GENERATION` | §5、§8、§10、§14 |
| `ROLLBACK_PROMOTION` | §14 |
| `RETIRE_DEFINITION` | §13.2 |

## 16.2 Repository 与 Ingress 写操作

| 操作 | 推演位置 |
|---|---|
| `CREATE_REPOSITORY` | §1.1 |
| Repository `PutArtifact` | §2.1、§3.2、§11.2 |
| Ingress `COMMIT {PUT/REMOVE}` | §2.2–2.3、§3、§4、§12.2、§13.1 |
| Ingress `PROPOSAL` | §7、§9 |
| Ingress `APPEND` | §3.2、§12.1 |
| `CREATE_COMMIT` | 由各次 COMMIT/PROPOSAL 内部执行 |
| `CREATE_REF / UPDATE_REF(CAS)` | Repo 初始化、§2、§7、§8 |
| `MERGE` | §8.2、§9 |
| `REBASE_CANDIDATE` | §8.2 的并发分支路径 |
| `PUBLISH_RELEASE` | §2.4、§10、§14 |
| `REVERT` | §14 |
| `ARCHIVE_REPOSITORY` | §13.2 |

Ingress COMMIT 的公开变更代数只有 PUT/REMOVE；Object/Tree/Commit/RefUpdate 是 Repository 的 Git 状态转移。Artifact 上传属于 Repository 辅助协议，Proposal Metadata 不属于 Accepted Knowledge。

## 16.3 Access 读操作

| 操作 | 推演位置 |
|---|---|
| `CAPABILITIES` | §6 |
| `DESCRIBE_SCHEMA` | §6 |
| `DESCRIBE_INDEX` | §6 |
| `RESOLVE` | §5、§6、§7、§8 |
| `READ_OBJECT` | §6 |
| `LIST_TREE` | §6 |
| `LOG` | §6 |
| `DIFF` | §6 |
| `ORIGIN` | §6 |
| `SEARCH` | §6 |
| `EXPAND_RELATIONS` | §6 |
| `WATCH_UPDATES` | §6 |

## 16.4 组合与治理

| 语义 | 推演位置 |
|---|---|
| ViewDefinition → ViewGeneration | §5 |
| Authorized Projection | §5.2 |
| KnowledgeRef / PinnedKnowledgeRef / FileRef | §2.2、§6 |
| 明确 Write Routing | §3、§4、§7 |
| Local Assertion，不覆盖 public | §3.1、§10 |
| Candidate Branch 直接访问 | §7.2 |
| 完整 PreviewGeneration | §8、§9 |
| ValidationReport 绑定完整组合 | §8、§9 |
| public 升级不做跨 Repo merge | §10 |
| Fork 三方同步 / Vendor 刷新 | §11 |
| Append Correction/Retraction 与 Derived Revision | §12 |
| Definition 更新、Repo Archive、Promotion Rollback | §13–14 |

## 16.5 关键时点状态账本

| 时点 | public/main | group/main | personal/main | Candidate | alice/stable | 说明 |
|---|---|---|---|---|---|---|
| Repo 创建后 | P0 | G0 | U0 | — | — | 只有独立 Repo，无联合 View |
| public 首次发布 | P3/R17 | G0 | U0 | — | — | Release 不自动 Promote |
| group/personal 写入后 | P3 | G2 | U1 | — | — | 下游引用上游，但没有复制 |
| VG1 发布 | P3 | G2 | U1 | — | VG1 | `VG1={P3,G2,U1}` |
| Proposal C2 | P3 | G2 | U1 | group:C2 | VG1 | main 与稳定 View 均未变化 |
| PVG1 测试失败 | P3 | G2 | U1 | group:C2 | VG1 | VAL-1 仅供审计 |
| C3/PVG2 通过 | P3 | G2 | U1 | group:C3 | VG1 | Approval 仍未改变 Accepted State |
| group Merge+Promote | P3 | C3 | U1 | closed | VG2 | Merge 与 Promote 分两步 |
| public P4/R18 | P4 | C3 | U1 | — | VG2 | 新 Release 尚未服务 |
| VG3 Promote | P4 | C3 | U1 | — | VG3 | 上游升级未改 group/personal Repo |
| Catalog Rollback | P4 | C3 | U1 | — | VG2 | 只恢复 Serving 组合 |
| public Revert P5 | P5 | C3 | U1 | — | VG2 | 权威内容修正后仍需新 Generation |
| personal Archive | P5 | C3 | U1 archived | — | 依 Channel | 新 View 默认不再选择 personal；历史受 ACL/保留策略约束 |

---

# 17. 最终判断题

1. **权威和 ACL 边界是什么？** 不同则拆 Repository，而不是建目录层级。
2. **引用知识、版本还是文件？** 分别使用 KnowledgeRef、PinnedKnowledgeRef、FileRef。
3. **读取组合了哪些 Commit？** 必须给出完整 ViewGeneration。
4. **权限是否可审计？** 保存 principal、Policy versions 和 AuthorizationDecisionRef。
5. **写到哪里？** 必须有唯一 target Repository/Ref/base Commit。
6. **这是补充/分歧还是修改来源？** 前者建本地 Assertion；后者进入来源 Repo Proposal。
7. **这是 Fork 还是 Vendor？** Fork 有新身份和三方同步；Vendor 只刷新只读副本。
8. **测试证明了什么？** 必须是完整 PreviewGeneration，不只是 Branch 或候选 Commit。
9. **上游升级会改本地 Repo 吗？** 普通引用不会；只生成新 Generation。
10. **查询能否回到规范来源？** 每个 Hit/Edge 都要有 Repo、Commit、KnowledgeRef、Object、Scope、Provenance。
11. **Generation 是否等于权限快照？** 不是；它锁定数据 Commit，每次访问仍重新授权。
12. **恢复动作改变哪一层？** Projection Rebuild、Catalog Rollback、Repository Revert 分别处理索引、Serving 组合和权威内容，不能混用。

只要这十二个问题都能得到确定答案，创建、修改、查询和维护才真正闭环。
