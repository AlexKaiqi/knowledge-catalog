# Knowledge Catalog 系统设计

日期：2026-08-27
定位：**权威设计说明，不是第二份协议定义。**

本文解释问题、第一性原理、调研结论和架构决策。具体类型、字段、错误码和调用行为以 Go 接口、包 README、Conformance 测试和 CLI 命令表为准：

- 分层与依赖边界：`LAYERS.md`、`internal/arch/`
- 组合：`COMPOSITION.md`、`catalog/`
- 写：`knowledge/writer/README.md`、`knowledge/writer/`
- 读：`knowledge/reader/README.md`、`knowledge/reader/`
- 检索：`ASPECT_ACCESS.md`、`index/README.md`、`index/`
- Store：`STORE_ADAPTERS.md`、`snapshot/README.md`、`knowledge/README.md`、`snapshot/*/`、`retrieval/*/`
- 验证：`TEST_CATALOG.md`、`MVP_ACCEPTANCE.md`、各包 `_test.go`

文档不复制代码中可直接读出的结构，也不维护“已实现/未实现”流水账。实现状态看 README、测试目录和代码。

---

## 0. 设计摘要

Knowledge Catalog 是面向团队和组织的通用知识底座：保存或组合可被人和 Agent 共同寻址、复现、归因和维护的知识。它不是检索应用、个人笔记库，也不是某个元数据产品的 fork。

核心主张：

1. **知识身份不等于路径。** 文件可以移动，引用不能因此失效。
2. **一次读取必须有不可变坐标。** “刚才读到什么”必须能说明。
3. **来源是知识正文的义务。** Git author、日志和模型措辞不能替代来源信封。
4. **多个权威保持独立。** 组合不是覆盖，也不制造跨仓事务。
5. **写入意图必须显式。** 正式变更和候选建议具有不同治理承诺；动态观察不是 Repository 写面。
6. **检索只定位。** 索引可丢、可重建；命中后回权威读取。
7. **单 source 与多 source 使用同一语义。** 单 source 只是 Workspace 只有一个成员。
8. **采用成本属于架构约束。** 普通 Git 仓可以先挂载；需要知识解释时再要求 ② 能力。

### 0.15 协议分层

```text
③ 检索派生     AccessSpec / RetrievalPlan / CandidateRef / Hydrate
M 访问物化      外部 state / stream runtime（墙外上层产品）
② 知识内容     object_id / Address / Aspect / schema / provenance / Binding
① 组合平面     Catalog / Workspace / 一次命令内 pin
⓪ 操作语义     Snapshot / ref / commit / CAS
```

- ⓪ 回答“Snapshot 怎样演化和并发更新”。底座不再承载 Stream。
- ① 回答“这次任务组合哪些权威、固定在哪些坐标”。Catalog 不解释对象正文。
- ② 回答“一个版本上的内容为何是知识、怎样访问外部物化”。身份、Aspect、Schema、来源与 Binding 从这里开始。
- M 提供动态观察，但不是底座编号层；③ 编译声明并定位候选。Projection 不成为权威。

完整入侵检查见 `LAYERS.md`。物理介质的权威/索引/缓存/投影梯子见 `STORE_ADAPTERS.md`，不要与 ⓪–③ 混名。

---

## 1. 问题与第一性原理

### 1.1 最小可信知识值

一段文本不是足够的知识单元。可被可靠引用的知识至少是：

```text
KnowledgeClaim = Value
               @ StableIdentity
               @ ImmutableVersion
               <- Provenance
               under AuthorityBoundary
```

物理表示可以是 Markdown、JSON、数据库行或外部资源；进入可信消费路径后，必须能恢复这些坐标。

### 1.2 不可约事实

| 事实 | 直接后果 |
|---|---|
| F1 对象会移动 | 路径不能做长期身份 |
| F2 状态会演化 | 读取和引用必须固定版本 |
| F3 组织中有多个独立权威 | 组合必须保留来源，不能静默覆盖 |
| F4 正式状态、候选建议和动态观察的承诺不同 | 前两者走 Snapshot 写面；动态观察留在外部运行时 |
| F5 原始记录、正式定义、派生知识和索引的认识论地位不同 | 结构、角色、载体不能揉成一个枚举 |
| F6 模型输出是概率性的 | 推断不能伪装成 Repository 已接受事实 |
| F7 数据规模和部署条件会变化 | 语义不能绑定单一 Store |
| F8 采用成本决定系统能否落地 | 挂普通仓不应先强迫改造成知识仓 |

### 1.3 从审计问题反推机制

每条可信事实最终都要回答：

| 审计问题 | 最小机制 |
|---|---|
| 这是谁？ | `object_id` / Address，独立于路径 |
| 这是哪个状态？ | Snapshot commit；动态访问另带 observation basis |
| 谁主张它？ | Repository 权威边界与当前授权 |
| 它从哪里来？ | provenance envelope |
| 为什么没有覆盖别人？ | Workspace union 保留成员坐标 |
| 为什么没有丢写？ | expected-old / CAS / 幂等 |
| 搜索命中是否就是事实？ | typed candidate + 回权威 hydrate |

缺少其中任意一项，系统最多是内容存储或搜索工具，不是可审计知识底座。

### 1.4 非目标

通用本体、通用 PATCH DSL、完整图查询语言、跨 Repository 分布式事务、自动语义冲突裁决、全库 `LLM_QUERY` 和一次命令中途跟随 `latest` 都不属于核心协议。

这些功能并非永远不能存在，而是不能被偷偷塞进身份、组合或 Store 接口中。

---

## 2. 总体架构与领域边界

```text
外部权威 ── Collector ──→ Writer ──→ Snapshot

外部 state/stream ← Binding ← Aspect declaration
          │
          └────────→ Retrieval Projection（可重建）

Catalog ── ResolveWorkspace ──→ command-local pin
                                      │
                                      ├─→ Reader 精确读
                                      └─→ Retrieval 定位候选 → 回权威读
```

### 2.1 四个核心边界

| 边界 | 负责 | 不负责 |
|---|---|---|
| Repository | Snapshot 版本、Ref、CAS、权威内容 | 动态运行、检索排名、最终答案 |
| Writer | 显式写意图、前置条件、幂等、Receipt | 采集框架、内容真伪判断 |
| Catalog | 承认仓、Workspace 配方、命令内 pin | `object_id`、Aspect、索引、payload |
| Materialization / Retrieval | 外部观察、候选定位、typed hydrate | 改写 Repository、业务回答 |

ControlPlane、Gate、Hook、Collector 和 Agent Application 都是围绕核心边界的职责，不应反向污染核心接口。

### 2.2 四个必须正交的维度

```text
Structure       Entity / Aspect / Member / Relation
Epistemic role  SOURCE / OBSERVATION / ASSERTION / DERIVATION / DEFINITION
Collection      Snapshot / Artifact / Derived
Access          exact read / text / filter / sort / state binding / stream binding
```

同一主题可以同时有 Snapshot Definition、外部 Stream Observation、Derived Assertion 和检索 Projection。外部 Stream 是访问形态，不是 Repository collection。把这些维度压成一个“对象类型”会导致写冲突、权限、版本和检索语义互相绑死。

### 2.3 逻辑与物理分开

底座逻辑协议只冻结身份、Snapshot 版本、来源、写边界、组合与读取结果。Git、Dolt 与 OpenSearch 是实现选择；外部 State/Stream 的引擎由上层产品选择。本地未配置 OpenSearch 时不模拟 SEARCH，只提供精确 READ/VFS。

Repository-native 是采用策略：尽量复用 Git 已经提供的 commit/ref/CAS，不把 Git 的偶然细节提升成知识协议。

---

## 3. 身份、版本与来源

### 3.1 身份不依赖路径

`KnowledgeRef` 指向 Repository 中的长期对象；`PinnedKnowledgeRef` 再固定版本；文件路径只是某个版本里的位置提示。

一个对象可以由多个 Aspect/Member 单元组成，因此唯一写入地址是：

```text
Address = object_id + aspectName + memberKey
```

同一 `object_id` 的多个 Aspect 合法；同一 Address 重复不合法。不要把 Entity blob 与 Aspect 文件混在同一个对象上。

为什么把身份放在内容而不是独立 address map：身份与内容随同一次 Snapshot 原子演化；路径索引可以扫描重建，不产生第四份需要单独 CAS 的权威状态。

### 3.2 Snapshot 版本与动态观察分责

- Snapshot 用 commit/ref；读取时固定 commit。
- Workspace 把各成员 selector 在命令开始时解析一次。
- Binding 指向的 State/Stream 由上层产品返回 watermark/cursor/observedAt；这些不进入 Workspace pin。
- Projection 报告自己的 basis、coverage 和 lag，但 basis 不因此变成 Canonical。

### 3.3 Provenance 与历史分责

- `GET_PROVENANCE` 回答当前对象单元声明了哪些来源、活动和算法输入。
- `LOG` 回答对象在 Repository 版本图上何时引入不同 revision。
- `DIFF` 比较两个固定版本的对象值。

三者不能互相伪装。Git author 不是外部来源；`sourceRefs` 也不是 Reader 自动爬取的证据图。

DERIVATION 必须固定输入版本和算法信息，否则模型生成内容无法复现其依据。

---

## 4. 写入与权威边界

### 4.1 为什么必须分写面

```text
COMMIT    接受为 Snapshot 权威状态
PROPOSAL  写候选，不改变已发布状态
```

两者的发布承诺不同。动态 State/Stream 是 Binding 观察面，不是第三个 Writer Surface；需要沉淀时由 Collector 显式生成 Snapshot ChangeSet。

知识内容的变更代数只有 PUT / REMOVE。`PUT Aspect` 替换一个 Address 分区，不引入通用 PATCH。

### 4.2 Writer 薄而强

Writer 不判断业务内容是否正确，只机械保证：

- 唯一 target；
- expected-old commit CAS；
- 命令幂等；
- Schema 与来源的最低约束；
- 原子失败或明确 Receipt；
- 写入只经受保护接口，不直写 Backend。

采集、对账、翻译源消息和 LLM 生成都在 Writer 之外。Collector 只生成 Snapshot ChangeSet，不获得动态运行时写面。

### 4.3 Repository 是治理边界

Repository 同时划定独立身份空间、版本图、写权限和生命周期。按 public/group/personal 覆盖对象，或按表级 GRANT 拆知识仓，都会把治理边界和内容属性混在一起。

归档是领域生命周期终点；物理删除属于保留或合规流程，不伪装成普通知识操作。

---

## 5. 权威载体与认识论地位

### 5.1 Snapshot 与外部动态观察

Snapshot 适合保存组织接受、需要版本图的知识。高频当前态和事件流适合由专门产品承担 lookup、cursor、retention、回放和 Fold。

Aspect 可以保存 State/Stream Binding，Schema 分别描述当前值或单条记录。外部值不会因可访问而自动成为 Canonical；需要保留时由 Collector 在明确 provenance 下 COMMIT 一次 Snapshot。

### 5.2 Canonical、Derived 与 Projection

- Canonical 是某个权威载体在固定 basis 上接受的内容。
- Derived 必须保留输入 basis 和算法。
- Projection 只为访问加速，可删除重建。

任何索引、缓存或投影都不能反向成为 `object_id`、来源或事实的权威。

### 5.3 外部权威

外部系统保有实时状态时，知识侧保存稳定、可版本化、Agent 可理解的访问声明；实际访问复用统一身份、授权和 trace。访问默认不隐式写回知识。

ResourceDescriptor 可以打包共享声明，Aspect 也可以内嵌或引用 State/Stream Binding。具体规划见 `CONNECTORS.md` 和 `LIVE_MATERIALIZATION.md`。

---

## 6. Catalog 与 Workspace 组合

### 6.1 为什么组合而不是复制

组织知识天然分布在多个权威中。把 public 拷进 personal、或把多个仓覆盖成一份结果，会丢失来源、制造同步问题并模糊权限。

Workspace 是配方：选择哪些 Repository 和 selector。一次命令开始时解析成固定 `{repo → commit}`；命令内不漂移、默认不落登记表。

```text
WorkspaceDefinition --resolve once--> ResolvedWorkspace
                                          repo A → commit A1
                                          repo B → commit B7
```

联合读是来源保留的 union，不是 public/group/personal 覆盖栈。同标题、同 `object_id`、甚至相互矛盾的来源都可以并存。

### 6.2 组合层不解释知识

Catalog 只看 Repository identity、selector 和 commit 是否存在。它不读 frontmatter，不认识 Aspect/Binding，也不生成 AccessPlan 或动态 cut。

这条边界让普通 Git 仓可以停在 ⓪+①：先被挂载、组合和检出；只有进入知识 READ/SEARCH 时才要求 ②/③ 能力。

### 6.3 写回必须有唯一落点

Workspace 本身不可写。可写 mount 根据路径前缀确定唯一 Repository；跨 mount 修改拆成多次单仓提交，不制造跨仓事务。详细推导和业界对照见 `COMPOSITION.md`。

---

## 7. 读取与检索

### 7.1 精确读取优先

Repository Reader 在已经固定的 commit 上回答：对象是否存在、声明/快照值是什么、来自哪里、如何复核。消费方通过 Workspace 打开 Knowledge Serving；维护方才直接指定 Repository 与版本。

对象读可以拼装多个 Aspect，也可以按 Address 读取单元。拼装是读取策略，不是存储形状。消费侧逻辑 READ 按 `ValueSource` 分派：Snapshot 返回 commit 中的值；State Binding 经注入端口 hydrate，并返回 declaration/observation 双 basis；参考 Knowledge Server 通过 `resource-access/v1` 调用独立 runtime 容器。Workspace SEARCH 对纯 Snapshot 字段使用 Snapshot projection；涉及 State Binding 字段时使用固定声明 commit 上构建的独立动态 projection，并从同 revision Serving State hydrate。Stream 必须走独立 window/query，不能被普通 READ 隐式数组化。VFS、checkout 与维护读始终保留 Snapshot/声明视图。Aspect 具体取舍见 `ASPECT_ACCESS.md`，动态投影见 `PROJECTION_CONTROLLER.md`，代码契约见 `knowledge/reader/README.md` 与 `knowledge/serving/README.md`。

### 7.2 历史三问分开

```text
LOG             对象 revision 时间线
DIFF            两个固定版本的值差异
GET_PROVENANCE  当前版本单元声明的来源
```

拆开后，每个问题都有明确 basis、授权和性能边界；合并成一个“历史 API”反而容易误导。

### 7.3 SEARCH 只定位候选

Schema 不声明“建一个什么索引”，只声明字段允许怎样被发现：

```text
schema field access[] + type
  → AccessSpec（固定 repo + commit 的逻辑访问契约）
  → RetrievalPlan（按请求和运行时 provider capability 编译）
  → CandidateRef（仅 provider → hydrator 的内部引用）
  → READ/Hydrate authority @ 同一 basis
  → 完整 KnowledgeHit + KnowledgeVersion
```

第一版逻辑访问面只有 `text / filter / sort`。`stored`、`summary` 是物理引擎可能采用的内部优化，不是 Schema 能力，也不是结果形状；`key` 若表示对象身份应使用 `KnowledgeRef/Address`，若只表示快速等值查询则已被 `filter` 覆盖。Schema 不绑定 provider、analyzer 或物理表，也不枚举查询谓词。查询算子由访问面与类型推出；能力不满足时明确报错，不退化成整包 JSON contains。

字段身份必须是 `(schema, aspect, path)`，不能只用裸 `path`。裸 path 查询若命中多个字段，Planner 必须显式展开或报歧义，不能取第一项。

依赖方向是高层语义定义抽象、物理实现依赖该抽象，而不是 Schema 依赖某个索引器：

```text
reader: SearchRequest / SearchResult / basis / 高层 Searcher port
   ↑
index:  planner / executor / Retriever / ProjectionMaintainer / CandidateRef
   ↑
local, scale, upper-layer binding adapters

catalog: 只提供 ResolvedWorkspace，不 import index
cli:      负责组装
```

`Retriever.Probe` 针对具体 clause 声明 `exact / superset / approximate / unsupported` 与 coverage。`superset` 不漏候选，执行器 hydrate 后做 residual filter；`approximate` 可能漏项，因此结果只能声明 partial。投影维护另用 `ProjectionMaintainer`，不能要求所有可检索外部源都实现 rebuild/apply。

Workspace 只固定成员坐标，不拥有一个复制所有内容的联邦大索引。不同 lane/provider 的 score 不具备共同尺度；结果必须保留 provider、local rank/score 和 matched fields。

Workspace 同样不进入投影文档。它是一次请求的范围：先解析并授权得到固定
`{Repository → commit}`，再扇出到各 Repository basis 的共享投影，最后合并并回读。
`workspace_id/workspace_ids` 不属于知识正文、CompiledDoc 或 OpenSearch mapping；否则仅修改
配方就会重写知识投影，并把组合状态误当成权限或内容属性。多 index、`_msearch` 和绑定不可变
PinID 的短期 alias 可以作为执行优化，但不改变 ProjectionSpec、SearchView 或授权边界。

Candidate 不携带知识正文。Provider 可以在内部保存 `_source`、stored field 或 doc value，但协议不暴露这些载荷，最终命中必须回权威源读取完整知识。公开返回形状至少包含：

```text
SearchResult  = SearchView + Completeness + KnowledgeHit[]
KnowledgeHit = KnowledgeValue + KnowledgeVersion + LaneEvidence[]

KnowledgeVersion = repository + object_id + declarationCommit
                 + unit(Address, digest, schema_ref, valueBasis)[]

valueBasis = SnapshotCommit | ObservationBasis
```

查询视图、知识版本与 provenance 中的 source revision 是三个不同概念。完整 hydrate 或 residual filter 可能消耗多个 candidate page；Planner 必须继续翻页直到填满 limit、provider exhausted 或预算耗尽。预算耗尽但仍可能有结果时必须标 `partial`。稳定并列顺序至少使用 `(repository, object_id)`。

动态知识的 Snapshot/State/Stream RetrievalPlan、observation basis 与 typed candidate 见 `LIVE_MATERIALIZATION.md`。

### 7.4 Grounding 不能在最后一跳丢失

Application 组装模型上下文时，必须保留 pinned reference、digest、fragment 和 provenance 摘要。模型推断可以存在，但要与 Repository 中已接受的知识明确区分；推断若要成为知识，仍走 COMMIT 或 PROPOSAL。

---

## 8. 维护、治理与恢复

### 8.1 维护闭环

```text
PROPOSAL → Preview（完整 Workspace）→ Validation/Gate → MERGE
```

Validation 必须绑定完整 Preview，而不是浮动分支名或单一 Candidate。Merge 推知识仓已发布 Ref；新的消费命令重新解析 Workspace 后才看见变化。

Gate 是状态跃迁的证据清单；Hook 是动词前后的出站通知；Collector 是外部权威的入站更新。三者方向不同，不能互相替代。详见 `GATES.md`、`HOOKS.md`、`CONNECTORS.md`。

### 8.2 三种回滚

- Projection 错误：重建投影，不改知识。
- Workspace 配方错误：修正 selector，重新解析。
- Repository 内容错误：新提交或 REVERT，保留历史。

混用会把派生状态、组合坐标和权威事实揉成一套不可审计的“回滚”。

### 8.3 授权

知识可见性按 Repository 和 `kc` 动作求值；Workspace 配方不发权，命令内 pin 也不冻结未来授权。

外部系统的实时业务授权仍由外部系统强制。仓内 `permissions` Aspect 可以保存某次外部授权快照，但不能反过来放行 `kc knowledge read` 或外部 SELECT。推导和业界对照见 `PERMISSIONS.md`。

### 8.4 恢复边界

底座恢复要验证 Repository ref、Catalog 配方、派生 basis 和 Receipt/Audit。上层产品另行恢复 Binding generation、动态 cursor/watermark 与 projection checkpoint；两者不能伪装成一次统一 Store 恢复。

### 8.5 访问可观测性

消费访问不写回 Canonical，而是以 `principal`、可选 `onBehalfOf`、trace/span/session 和固定 `Repository + commit + object/Address` 形成版本化过程证据。显式反馈按 trace 关联，hitmap 从访问账派生；认证算法和委托证明可替换，授权仍按实际 principal 求值。完整边界见 `OBSERVABILITY.md`。

---

## 9. 调研结论与设计决策

### 9.1 调研地图

| 问题 | 主要参照 | 采用的结论 | 专题文档 |
|---|---|---|---|
| Aspect 写粒度与检索形态 | DataHub、Unity、Atlas/Ranger、OpenMetadata | 写单元、默认读形态和检索文档分开 | `ASPECT_ACCESS.md` |
| 多仓可写组合 | Android repo、josh、Egeria、Solid、Nix flakes | 显式 mount、命令内 pin、按路径唯一写回 | `COMPOSITION.md` |
| 权限边界 | Git/Gitea、Ranger、Unity、Solid | Repository ACL 与外部业务授权分开 | `PERMISSIONS.md` |
| Store 与投影 | Git、Dolt、OpenSearch | Snapshot 权威、索引、缓存、投影分层 | `STORE_ADAPTERS.md` |
| 外部资源 | integration runtime、resource access | 访问声明是知识；凭证和运行留墙外 | `CONNECTORS.md` |
| 动态物化 | Garlic、CQL、IVM、DBSP、联邦检索 | State/Stream 由外部 Binding 物化；Retrieval 统一规划但不统一权威 | `LIVE_MATERIALIZATION.md` |
| 访问可观测性 | tracing、审计账、反馈闭环 | 固定知识版本的访问证据横切各层，不成为 Canonical 或授权依据 | `OBSERVABILITY.md` |
| 治理扩展 | CI checks、webhooks、merge protection | Gate、Hook、Collector 分责 | `GATES.md`、`HOOKS.md` |

专题文档保留调研证据和取舍；本文件不重复论文摘要或产品命令。

### 9.2 ADR 索引

- ADR-001 Catalog 与 Repository 是两个公开领域边界。
- ADR-002 Writer 只使用 COMMIT / PROPOSAL 两种 Snapshot 写意图；动态观察不是写面。
- ADR-003 Snapshot 使用不可变 Version/Ref/CAS；Git Adapter 直接复用 Git。
- ADR-004 Snapshot 内容只用 PUT/REMOVE，不提供通用 PATCH。
- ADR-005 Structure、Epistemic Role、Collection、Access 正交。
- ADR-006 Entity/Aspect/Member 是维护粒度，不是检索文档形状。
- ADR-007 KnowledgeRef 不用 Path 作身份。
- ADR-008 WorkspaceDefinition 与命令内 ResolvedWorkspace 分离。
- ADR-009 联合 Workspace 保留来源，不做覆盖。
- ADR-010 Workspace 不可写；写回必须路由到唯一 Repository。
- ADR-011 Proposal 指向 Candidate，不能直接改变已发布 Ref。
- ADR-012 Validation/Gate 绑定完整 Preview。
- ADR-013 Projection 归属 Access，非 Canonical。
- ADR-014 图核心最多保证一跳；不建通用图语言。
- ADR-015 Semantic Refinement 可选且 Ref-preserving。
- ADR-016 多 lane 候选保留 LaneEvidence，不伪造统一概率。
- ADR-017 Repository Store 只承担 Snapshot；State/Stream 由版本化 Binding 指向墙外运行时。
- ADR-018 Store Adapter 可替换，并复用同一 Conformance。
- ADR-019 已废止：正式 authority 仅 Dolt/Gitea，具体 adapter import 收敛到唯一 composition root。
- ADR-020 Repository 生命周期终点是 ARCHIVE，不暴露领域 DELETE。
- ADR-021 外部资源访问与 Collector Snapshot 更新分开；访问不隐式采集。
- ADR-022 Aspect 可声明 State/Stream Binding；Catalog 不固定动态 cut，Retrieval 负责观察与路由。
- ADR-023 Schema 只声明 `text/filter/sort` 访问语义；不声明索引实例、provider、`stored/summary/key`。
- ADR-024 RetrievalPlan 按请求从 AccessSpec、provider capability 与 runtime policy 编译；逻辑声明依赖抽象 provider 端口。
- ADR-025 CandidateRef 只在 Retrieval 内部流转，不携带知识正文；公开 SEARCH 总是 hydrate 完整 KnowledgeValue 与精确 KnowledgeVersion。
- ADR-026 Retriever 与 ProjectionMaintainer 分离；只支持 source pushdown 的 Binding 不被迫实现投影维护。
- ADR-027 消费侧精确 READ 对 State Binding 返回逻辑值与双 basis；Repository Reader/VFS 保持声明视图，Stream 不进入普通 READ。

### 9.3 核心不变量（K-01..K-28）

本节保留设计推导层的语义结论；规范性的可证伪属性、禁止观察和自动化证据统一登记在
[`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md)。两处冲突时必须先修复冲突，不能选择
对当前实现更宽松的一份解释。

| # | 不变量 |
|---|---|
| K-01 | 每个 Writer 命令只有一个 Snapshot target；Workspace 和动态运行值都不是 target |
| K-02 | 每个 Repository 有独立身份、ACL、Version 图、Ref 和生命周期 |
| K-03 | public/group/personal 是治理 Scope，不是目录优先级 |
| K-04 | KnowledgeRef 不依赖路径；PinnedKnowledgeRef 固定 Version |
| K-05 | Version 内 Canonical 与已接受 Ref 不可原地修改 |
| K-06 | Ref/对象更新必须带前置条件，禁止静默 LWW |
| K-07 | Proposal Durable 不表示已发布状态改变 |
| K-08 | Review、Validation、Approval、Gate 绑定精确 Candidate/Preview |
| K-09 | ValidationReport 绑定完整 Preview，而非单仓候选 |
| K-10 | ResolvedWorkspace 是 Repository→Commit Map；命令内不可变 |
| K-11 | 跨命令可跟已发布 selector；命令内不得跟随 latest |
| K-12 | 联合结果保留 Repository、Version、Object、Scope 和 Provenance |
| K-13 | 多来源并存，不按 Scope 静默覆盖 |
| K-14 | 普通知识引用升级不修改引用方 Repository，也不跨 Repository merge |
| K-15 | Fork 创建新 KnowledgeRef；只有 Fork sync 做三方比较 |
| K-16 | Vendor 保留精确来源 pin；本地编辑必须转 Fork |
| K-17 | 动态 State/Stream 不因可访问而成为 Canonical；沉淀必须显式 COMMIT |
| K-18 | 同幂等键同 digest 返回原 Receipt；异 digest 冲突 |
| K-19 | Projection 非 Canonical，必须声明 basis、coverage 和 lag |
| K-20 | Workspace pin 锁数据不锁未来权限；授权按请求求值 |
| K-21 | 内容写入经 Writer；治理动作经受保护 Control API，不直写 Backend/Ref |
| K-22 | 不构造跨 Repository 的虚假单一事务 |
| K-23 | Adapter 迁移不得改变身份、版本和读写协议语义 |
| K-24 | Repository 领域生命周期终点是 ARCHIVE；物理删除由保留/合规流程处理 |
| K-25 | Candidate 不作为知识结果；SEARCH 命中必须在计划固定的 SearchView/basis 上 hydrate 完整知识及版本 |
| K-26 | Schema 字段访问声明不包含 provider、物理存储载荷或对象身份的替代定义 |
| K-27 | 不能证明无漏项的 provider/plan 必须返回 partial；不得用 score、缓存命中或 invalidation 推断完整性 |
| K-28 | Bound State 消费结果必须同时标识声明与 observation basis；VFS/commit 不得冒充冻结动态值，Stream 不得隐式数组化 |

### 9.4 明确拒绝

Writer=ETL/LLM、Catalog=文件仓、Stream=Repository/Writer Surface、路径=对象身份、Workspace 覆盖栈、Projection 作权威、通用 PATCH、跨仓事务、审批只绑分支名、一次命令中途跟随 latest，均与上述推导冲突。

---

## 10. 代码是具体协议说明

设计决策落到代码后，不在本文重复维护字段表：

| 主题 | 规范入口 |
|---|---|
| Identity / Address / provenance；基础 errors | `knowledge/`；`kernel/` |
| Snapshot capabilities / Knowledge Reader-Writer | `snapshot/`、`knowledge/` 及各自 README |
| Workspace / Registry / pin / mount | `catalog/`、`catalog/README.md` |
| COMMIT / PROPOSAL / ChangeSet | `knowledge/writer/`、`knowledge/writer/README.md` |
| 声明 READ / LOG / DIFF / PROVENANCE | `knowledge/reader/`、`knowledge/reader/README.md` |
| 消费侧逻辑 READ / State Binding hydrate | `knowledge/serving/`、`knowledge/serving/README.md` |
| Access declaration / RetrievalPlan / physical projection | `ASPECT_ACCESS.md`、`LIVE_MATERIALIZATION.md`、`retrieval/`、`index/` |
| Gate / Hook / Collector helper | `gate/`、`hook/`、`connector/` |
| CLI/HTTP surface | `cli/command.go`、`cli/command_test.go` |
| Adapter guarantees | `internal/testkit/`、各 adapter contract tests |

协议变化先判断归属：改变为什么和跨包边界时更新设计；改变类型或行为时更新代码和测试；改变使用方法时更新包 README 或 Walkthrough。不要再把三者复制进同一份文档。
