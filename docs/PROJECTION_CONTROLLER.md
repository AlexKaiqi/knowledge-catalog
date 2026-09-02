# 动态 State 投影控制

日期：2026-08-27
定位：运行设计。当前完成度和缺口只在 `MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 维护。

本文细化 `LIVE_MATERIALIZATION.md` 已有的动态 State 投影方向，回答两个问题：

1. Snapshot 或外部 Observation 发生变化时，哪些变化会影响索引；
2. 怎样使用固定 Repository commit 和 Binding observation 构建符合 Knowledge 读取语义的索引。

本文不新增一套 Knowledge 层对象。使用的协议词汇仍是：Repository commit、Address、Schema、
Binding、ResourceDescriptor、`ObservationBasis`、`UnitObservation`、`SearchView`、`AccessSpec`、
`CandidateRef`、`Retriever` 与 `ProjectionMaintainer`。

`CompiledDoc` 是 `index/` 已有的内部 Go 类型，表示一个 `object_id` 的 provider-neutral 投影文档；
它不是 Knowledge 对象，也不是公开协议术语。本文正文统一称“投影文档”，仅在解释现有控制
算法时引用具体类型名。

---

## 1. 结论

现有 Snapshot 索引链应扩展成一个统一的投影控制链：

```text
Snapshot advance ───────────────────────┐
                                       ├── 投影控制器 ── ProjectionMaintainer
external source changed ─ change notice┘                       ↓
                                 │                    OpenSearch projection
                                 └── pinned Binding lookup
                                           ↓
                                 value + ObservationBasis
```

- Snapshot advance 和外部 observation change 都由同一个控制器决定 no-op、增量更新、失效或重建。
- Schema、Binding 声明和 ResourceDescriptor 都属于 Snapshot；它们不是第三种变更通道。
- 动态值不进入 Snapshot，不生成 Repository commit，也不出现在 VFS/checkout。
- Collector 发布稳定知识时仍走 ChangeSet → Writer → Snapshot；感知动态值变化时只发 change
  notice。Collector、runtime 都不直接写 OpenSearch。
- 控制器在固定 commit 上读取声明，按 Binding 得到动态值，拼装绑定后的 `KnowledgeValue`，再调用
  现有投影编译与维护能力。
- Snapshot projection 与动态 State projection 分开维护。动态更新不能静默改写固定 commit 的
  Snapshot projection。
- 索引仍只定位 `CandidateRef`；公开 SEARCH hit 必须按声明 commit 和 observation basis 回读完整值。

---

## 2. 边界

### 2.1 Snapshot 保存什么

Snapshot 保存：

- `object_id` 与 Address；
- Entity、Aspect、Member、Record、Relation 的 Snapshot value；
- `schema_ref` 和 `schema/*`；
- `value_source` 与稳定 Binding 声明；
- ResourceDescriptor；
- provenance。

Snapshot 不保存：

- Binding 实际返回的 State value；
- runtime generation、cursor、watermark、健康与连接状态；
- Serving State 或 OpenSearch 文档。

### 2.2 外部运行时负责什么

外部 runtime 按固定 Binding 提供：

```text
value + ObservationBasis
```

其中 `ObservationBasis` 使用已经存在的字段：

- `bindingGeneration`
- `consistency`
- `sourceRevision`（源支持时）
- `watermark`（源支持时）
- `observedAt`

runtime 不决定 `object_id`、Schema、Workspace 或投影物理结构。

### 2.3 投影控制器负责什么

控制器是投影的唯一写入者，负责：

- 接收 Snapshot advance；
- 接收动态 source change notice，并按当前固定 Binding 拉取；
- 判断受影响 Address、object 和间接依赖；
- 拼装绑定后的完整 `KnowledgeValue`；
- 编译完整投影文档；
- 调用 `ProjectionMaintainer.Rebuild/Apply`；
- 管理动态投影使用的 declaration commit、observation basis 与 provider projection revision；
- 在无法证明完整时使 SEARCH 失败或返回 partial，而不是返回假空结果。

它不拥有 Repository、Writer、Catalog、凭证或具体源客户端。

---

## 3. 两类输入

### 3.1 Snapshot advance

沿用现有：

```text
repository + fromCommit + toCommit
```

控制器在固定 `toCommit` 上读取 Knowledge，并根据 from/to diff 确定受影响 object。事件本身不携带
知识正文，也不要求 Catalog 认识 Address、Schema 或 Binding。

### 3.2 Dynamic change notice

默认采用现有动态物化文档已经选择的 notify-and-pull：

```text
source observer
  → change notice(binding/address/source revision hint)
  → controller 在目标 commit 重新解析 Binding
  → StateLookup
  → value + ObservationBasis
```

notice 只用于定位刷新范围和降低延迟。它携带的 source revision 只是 hint，不能替代 runtime 返回
并经校验的 `ObservationBasis`，也不能携带任意正文直接进入索引。

同一个外部进程可以同时承担 Collector 和 observer，但必须使用不同合同：

```text
稳定知识变化  → Knowledge ChangeSet → Writer
动态值变化    → change notice       → 投影控制器
```

### 3.3 Snapshot live 恢复

Snapshot live 投影有三份坐标，不能合成一把锁：

| 坐标 | 谁拥有 | 角色 |
|---|---|---|
| 仓 published HEAD（空 ref = `snapshot.DefaultRef`） | Snapshot | live 投影**应该**在哪 |
| OpenSearch control basis + READY | provider | live 投影**实际**在哪 |
| `controller.db` desired / applied / status | Controller | 工作队列，**不是**真相 |

正确性靠两条闭环，而不是把通知做可靠：

```text
快路径（可丢）：COMMIT / Merge → AfterSnapshot → Desire(to) → wake worker
慢路径（必须有）：published HEAD ≟ 投影 basis ≟ controller.desired
                 不等 → Desire(HEAD) → Ensure → READY
```

约束：

- Writer receipt 只允许 Desire；不得同步写 OpenSearch，也不得因 Hook / Desire 失败回滚仓。
- `PROPOSAL` 不发 AfterSnapshot。
- `controller.db` 里 `READY && Applied == Desired` 不能当作 CatchUp 的 skip 条件，除非 Desired 已是 published HEAD，且 live basis 也是该 HEAD。
- 丢失全部 AfterSnapshot、Desire 未落盘、CatchUp / Publish 中途中断后，长寿命 `kc serve` 的 `Controller.Start` 必须只靠 HEAD 对账把 live 投影追到 READY。周期 tick 覆盖 Start 之后又丢的通知。
- `Start` 只挂在 serve 的长寿命 Home。一次性 `Open()`（包括本机 CLI `kc knowledge search`）不得 CatchUp，否则消费路径会维护投影。
- 显式 `kc operations projection sync` 仍用于历史 commit 的 EnsureAt、强制重建和排障；它不再是 live 正确性的唯一入口。
- 消费 SEARCH 在 basis 未 READY 或不匹配时失败关闭，不得偷偷 Rebuild。
- git watch / webhook 不是正确性来源。外部直推 published ref 时，对账会追上 live 投影；这不表示直推等于 Writer。
- 动态 State 仍走 notice + Binding lookup；不要和 Snapshot HEAD 合成一个 key。

---

## 4. 从 Knowledge 读取语义构建索引

### 4.1 一个 Address 的有效值

对固定 commit 上的 Address：

| 声明 | runtime 结果 | Knowledge 消费面 | 索引处理 |
|---|---|---|---|
| Address 不存在 | 任意 | 单元不存在 | 不贡献字段 |
| Snapshot value source | 不调用 runtime | 使用 commit 中的值 | 从 Snapshot 值提取字段 |
| State Binding | 成功返回完整 value+basis | 使用动态值并返回 `UnitObservation` | 从动态值提取字段 |
| State Binding | 成功返回 JSON null+basis | 使用已观察的 null | 该单元适用字段已检查，但没有 value cell |
| State Binding | 尚未观察或调用失败 | 无法得到绑定后的值 | 不把它当成字段缺失；动态 coverage 不完整 |
| State Binding | declaration/generation 不匹配 | 结果无效 | 拒绝，不进入索引 |
| Stream Binding | 任意 | 普通 READ 明确缺能力 | 不进入首版动态 State 投影 |

首版不新增动态状态枚举，也不新增另一种 observation envelope：

- runtime 成功返回现有 `StateObservation{Value, Basis}`，表示完成了一次完整 Address observation；
- 若业务上确认当前值为空，可以成功返回 `value:null` 与合法 basis；
- runtime error 表示无法观察，不等于字段缺失或业务空值。

### 4.2 Address 与 object

Address 是维护单元，`object_id` 是索引文档单元：

```text
Entity/Relation blob                   → object root
Aspect(aspectName)                     → root[aspectName]
Member(aspectName, memberKey)          → root[aspectName][memberKey]
```

任何 Address 变化后都在目标 basis 上重新拼装整个 object。删除一个 Member 不等于删除 object；只有
目标 commit 上 object 不再存在或不再产生投影文档时才删除物理文档。

### 4.3 Schema 字段

字段身份继续使用完整 `(schema, aspect, path)`。索引只解释 Schema 声明的 `text/filter/sort` 和
字段类型，不扫描任意 JSON。

对 MISSING/NEQ，必须区分两种情况：

1. 已成功取得完整 Snapshot/State unit，但 path 不存在：可以证明字段缺失；
2. Binding 没有成功 observation：不能证明字段缺失。

不需要为此新增另一组公开 coverage 字段。现有投影文档的 `EligibleFields` 可以表达首版语义：

- Snapshot unit 或成功观察的完整 State unit：加入适用字段；
- 尚未成功观察的 Binding unit：不加入动态适用字段；
- 有 eligible field 但没有对应 cell：MISSING；
- 没有 eligible field：该 provider 对此 object/field 不声称覆盖。

同时，动态投影的 control metadata 与 `Retriever.Probe` 必须把尚未成功观察的适用 Binding 计入
coverage 缺口。省略 `EligibleFields` 只防止“尚未成功观察”被误判为 MISSING，不能单独证明整次查询
complete。

首版因此要求 StateLookup 返回一个 Address 的完整值，不支持字段级部分 observation。

### 4.4 Schema 对象自身

Schema 是可 READ/RESOLVE/GET_PROVENANCE 的知识。当前索引实现不把 `schema/*` 本身作为普通搜索
文档；它只使用 Schema 编译 AccessSpec。若以后需要搜索 Schema 对象，应为它建立明确的检索合同，
不能把 Schema 的字段定义顺带当成业务正文。

---

## 5. 两种投影

### 5.1 Snapshot projection

现有投影保持不变：

```text
(repository, commit, provider, physicalDigest)
```

- 只使用固定 commit 中的 Snapshot value；
- Binding 占位值不作为动态知识；
- 可为历史 commit 重建；
- Candidate 在同一 commit 回读。

### 5.2 动态 State projection

动态投影使用：

```text
fixed repository commit
+ observations obtained through Bindings at that commit
→ complete object projection documents
```

一篇 object 文档同时包含 Snapshot 字段和已经成功观察的 State 字段，从而支持静态条件与动态条件
的 AND，不需要首版先实现跨两个 provider 候选集求交。

动态投影与 Snapshot projection 使用不同的物理 generation/control metadata。Observation 更新只更新
动态投影。provider 可以在一个 generation 内增量 Apply，不要求每次 observation 都创建新索引。

动态投影的控制元数据至少能证明：

- declaration commit；
- AccessDigest；
- 使用了哪些 Binding generation/observation basis；
- provider revision 与 PhysicalDigest；
- 当前 projection revision/state/coverage。

这些是投影运行元数据，不进入 Schema、Workspace 或 Catalog pin。具体内部结构由实现确定；公开
结果继续通过 `SearchView` 与每个 hit 的 `UnitObservation` 表达 basis。

### 5.3 Serving State 与索引

为了从候选的同一 observation basis hydrate，动态运行面需要保存完整 observation value：

```text
Serving State：完整 value + UnitObservation
Index：AccessSpec 字段 + object identity + 内部 basis reference
```

Serving State 不是 Knowledge Repository；索引 `_source` 也不能作为公开 Knowledge value 返回。

---

## 6. Snapshot 变化如何影响索引

### 6.1 投影拓扑

| 情况 | 判断 | 动作 |
|---|---|---|
| cold | 没有投影 | Rebuild |
| continuous | stored basis 等于 fromCommit | 允许 Apply |
| already ready | stored basis 等于 toCommit 且相关 digest 一致 | no-op |
| diverged | stored basis 与 fromCommit 不连续 | Rebuild |
| physical changed | provider revision/PhysicalDigest 变化 | Rebuild |

### 6.2 Knowledge 变化

| Snapshot 变化 | 影响 | 动作 |
|---|---|---|
| Snapshot Entity/Aspect/Member/Record PUT/REMOVE | object value | 在 toCommit 重拼 object，upsert/delete/no-op |
| Relation type/direction/endpoints 变化 | Relation 保留字段 | 重编译 Relation object |
| Address 新增/删除 | object 组成 | 重拼整个 object，不能按 Address 直接删除文档 |
| object_id 改名 | 两个身份 | 删除旧 object，建立新 object |
| schemaRef 改变 | 字段归属和类型 | 移除旧 FieldRef 贡献，按新 Schema 重编译 |
| AccessDigest 改变 | 全部投影字段合同 | 首版重建该 Repository 的两种投影 |
| Schema 只改非 access 内容 | 通常不影响投影 | 若投影文档不变，只推进 commit basis |
| Binding 声明改变 | 动态值对应关系 | 旧 observation 不再兼容；清理相关动态字段并重新 lookup |
| ResourceDescriptor 内容改变 | 所有引用 Binding | 以 DescriptorDigest 识别并刷新引用 Address |
| Snapshot → Binding | value source | 移除 Snapshot 字段，成功 observation 后再加入动态字段 |
| Binding → Snapshot | value source | 停用旧 observation，使用 toCommit Snapshot value |
| State → Stream | 访问形态 | 移出首版动态 State 投影 |
| provenance/非索引字段改变 | 无候选字段变化 | 不重写文档，只推进 commit basis |

事件分类只用于找出需要重算的 object。最终是否写物理索引，以重新编译后的投影文档是否变化为准。

### 6.3 间接依赖

至少需要处理：

```text
schema/*             → 引用它的 Address/object
ResourceDescriptor   → 引用它的 Binding Address
Binding Address      → 所属 object
```

首版可以在 AccessDigest 变化时全 Repository 重建，但 ResourceDescriptor 变化不能只更新 Descriptor
自身，否则旧 runtime generation 会继续污染动态索引。

---

## 7. Observation 变化如何影响索引

| 变化 | 动作 |
|---|---|
| 动态索引字段值改变 | 重新拼装所属 object，Apply upsert |
| 只改变非索引字段 | 更新 Serving State 和 observation basis；投影文档不变则不重写 |
| 值不变但 sourceRevision/basis 推进 | 不重写文档，只推进动态投影控制元数据 |
| 成功返回 null | 清除该单元旧 cells；适用字段已检查，因此可参与 MISSING |
| lookup 超时/失败 | 不发布新投影 revision，不解释为 null/MISSING |
| declarationDigest/DescriptorDigest 不匹配 | 拒绝结果 |
| 旧 bindingGeneration 的迟到结果 | 拒绝，不覆盖当前 generation |
| Binding generation 切换 | 旧 observation 失效；刷新固定 commit 上所有受影响 Address 后再恢复完整 coverage |
| 重复 notice | 允许重复刷新，最终结果与单次处理相同 |

首版不定义 TTL/freshness policy。一次瞬时刷新失败不改变已经发布的旧 projection revision，旧结果
仍只以它原来的 observation basis 可解释；如果 Binding 声明或 generation 已经改变，旧结果不再
兼容，动态投影必须降级或失效，不能回退到旧 generation。

首版也不要求 source delta/checkpoint 才能冷启动：控制器可以枚举固定 commit 中已知的 State
Binding Addresses，逐个 lookup 后建立动态投影。change notice 只负责后续刷新。新 object identity
仍必须先通过 Snapshot commit 出现。

---

## 8. 唯一构建路径

正确性应集中到一条构建路径：

```text
Build(repository, commit, available observations, AccessSpec)
  1. 在固定 commit 读取 UnitDeclaration 和 Snapshot values
  2. 解析 schemaRef、Binding 和 ResourceDescriptor
  3. 对 Snapshot unit 使用 commit 中的值
  4. 对 State Binding 只使用声明和 generation 匹配的成功 observation
  5. 按 Address 规则拼装绑定后的 KnowledgeValue
  6. 用现有 AccessSpec 编译完整 object 投影文档
  7. 规范排序并计算 object digest
```

增量只优化重算范围：

```text
event
  → affected object IDs
  → 对这些 object 运行同一 Build
  → 比较新旧投影文档
  → ProjectionMaintainer.Apply
```

Rebuild 与 Apply 必须复用相同的拼装和编译逻辑，满足：

```text
full Build(after changes) == incremental Apply(Build(before), changes)
```

不能为 observation 另写一套“直接拼 OpenSearch JSON”的路径。

---

## 9. 激活、SEARCH 与 hydrate

### 9.1 动态刷新顺序

```text
1. 固定并验证 declaration commit 上的 Binding
2. runtime lookup 得到 value + ObservationBasis
3. 保存可按该 basis 读取的完整 value
4. 编译并 Apply 动态投影文档
5. 发布新的 provider projection revision
```

查询只能使用已经发布的 revision。首版不要求跨服务分布式事务；任何一步失败都不发布新 revision。
若 provider 无法继续解释旧 revision，则显式标为 partial/failed，不能把混合 basis 报成 ready。

### 9.2 选择投影

- 请求只涉及 Snapshot 字段：使用与 commit 匹配的 Snapshot projection；
- 请求涉及 State Binding 字段：使用与声明 commit、AccessDigest 和 observation bases 匹配的动态
  State projection，或使用能如实 Probe 的 source-side Retriever；
- 静态和动态 clause 混合：首版使用动态 State projection 中的完整 object 文档；
- 必需动态 clause 没有可用投影/provider：默认 `CAPABILITY_UNSATISFIED`；调用方显式允许
  best-effort 时才返回 partial；
- Workspace 仍按本次 ResolvedWorkspace 的成员 commits 扇出，不按 Workspace 建索引。

### 9.3 SearchView 与 continuation

`SearchView` 是已有规范名。它在现有 `snapshots` 之外携带本次使用的 provider projection
revision；不新增另一种 `*View`，也不内联全库的 Binding observation basis。

一个 object 有多个 Binding 时，命中的 `KnowledgeVersion` 各自保留相关 `UnitObservation`，不能压成虚假的全局 watermark。全库 observations 由 revision digest 标识并保存在同 revision Serving State，避免 SearchView 变成 O(知识规模) 的响应。
continuation 继续绑定 query digest、SearchView、不可变 provider generation/revision 与当前位置。Observation 推进后，
旧 continuation 不能静默切换到新 basis。

### 9.4 Hydrate

- Snapshot units 从 SearchView 固定 commit 回读；
- State units 从 Candidate 对应的 observation basis 回读完整 value；
- 同 basis 的 Serving State 不可用时，查询以 `PRECONDITION_FAILED` 失败，不能改读 latest 冒充原候选或返回 partial；
- `latest-only` 不能承诺未来可重读，只能如实声明本次读取能力；
- 公开结果继续返回 `KnowledgeValue + KnowledgeVersion + UnitObservation[] + LaneEvidence[]`。

---

## 10. 完整性与失败

动态查询只有同时满足以下条件才可以声明 complete：

1. 每个 required clause 都有 Exact，或 Superset 已在完整候选集上完成 residual；
2. Snapshot 字段在 SearchView commit 上覆盖完整；
3. 涉及的 Binding Addresses 都有与固定声明匹配的成功 observation；
4. provider projection revision 与声明/observation bases 一致；
5. 每个公开 hit 都从同 basis hydrate 成功；
6. provider exhausted，或已证明 LIMIT 之后不影响本页。

| 情况 | 结果 |
|---|---|
| 请求字段不在 AccessSpec | `CAPABILITY_UNSATISFIED` |
| 必需动态 clause 没有 provider | `CAPABILITY_UNSATISFIED` |
| runtime 瞬时失败且请求要求刷新 | `TEMPORARY_UNAVAILABLE` |
| projection 与声明/observation basis 不匹配 | `PRECONDITION_FAILED`，不得空成功或降级 partial |
| Binding 未成功观察却执行 MISSING/NEQ | 不得声明 Exact |
| Candidate 无法同 basis hydrate | `PRECONDITION_FAILED`；不得消耗候选后继续 |

---

## 11. 分层、安全与部署

### 11.1 分层

- `knowledge/writer`、`knowledge/reader`、`catalog` 不依赖投影控制；
- `index` 可以扩展现有 `Index`、投影编译与 provider-neutral 端口，但不依赖 Catalog 或具体
  OpenSearch adapter；
- Catalog Hook 仍由应用装配层转交给 `index`；
- HTTP runtime、Serving State 和 OpenSearch adapter 都在应用装配层注入；
- `connector` 不依赖 Writer、Index 或 runtime。

### 11.2 安全

- 在调用 runtime 和 provider 前完成 Repository/Workspace 授权；
- change notice 使用可信服务身份，不能借 notice 越权探测 Address；
- principal、onBehalfOf、request/trace 继续走统一观测上下文；
- 凭证、实际 endpoint 和内部拓扑不进入 Snapshot、SearchView 或索引文档；
- 运行 health、lag、generation 和 last error 不 COMMIT。

### 11.3 Docker 首版

```text
source-mysql container
        │
collector/observer container
        ├── stable knowledge → Writer API
        └── change notice ─────────────────────┐
                                               ▼
gitea container                       KC/controller container
        ▲                                      ├── resource-access/v1
        └──────── Snapshot ─────────────────────┤
                                               └── OpenSearch API
resource-runtime container ◀────────────────────┤
opensearch container ◀──────────────────────────┘
```

每个逻辑服务一个容器，不要求多个副本。Collector 和 observer 可以暂时同容器，但必须使用两条不同
协议。验收不允许 KC 直接读 source fixture、runtime 与 KC 共用内存 fake、Collector 直写
OpenSearch，或 observation value 进入 Gitea Repository。

---

## 12. 验证边界

本文只拥有投影控制算法：Snapshot/Observation 输入、对象重拼、generation 发布、
`SearchView` 选择和同 basis hydrate。实施阶段、当前完成度和逐条测试清单不在这里维护。

控制器必须保持以下可证伪不变量：

1. Observation 更新不能推进 Repository commit，也不能改变 VFS/checkout bytes。
2. Snapshot 与 State 投影使用同一套 Address 拼装、Schema 解释和字段编译语义。
3. Binding 未成功观察、观察失败或 digest/generation 不匹配时，不得伪造 MISSING 或新值。
4. Candidate 不携带公开正文；公开命中必须在同一 Snapshot/Observation basis hydrate。
5. continuation 绑定 `SearchView`，不能跨 projection revision 静默续读。
6. 全量 rebuild 与连续 apply 在相同输入上必须得到相同文档集合和 digest。
7. Connector、runtime、Catalog、Writer 和 Snapshot Store 都不能绕过控制器写检索投影。
8. Provider 无法证明完整时必须返回 partial/capability 事实，不能把故障报告成零结果。
9. live Snapshot 投影以 published HEAD 和 provider READY basis 对账；`controller.db` 只是队列。
10. 消费 SEARCH/READ 不得 CatchUp / Ensure；长寿命 serve worker 与显式 `projection sync` 才维护投影。

当前实现证据和未完成场景统一登记在 `TEST_CATALOG.md` 的索引条目；产品可用性结论
统一登记在 `MVP_ACCEPTANCE.md`。多副本、worker lease、持久化 observation history、
Stream 与规模资格线属于后续运行/规模设计，不在本文追加 P0–P3 流水账。
